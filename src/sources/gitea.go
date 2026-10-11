package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"main/src/configuration"
	"main/src/utils"
	"net/http"
	"net/url"
)

// Gitea reads repositories from a Gitea or Forgejo instance (self-hosted, gitea.com, Codeberg,
// LavaForge, ...) through the /api/v1 API they share.
type Gitea struct {
	URL   url.URL
	Token string

	client *http.Client
}

// giteaMetadata remembers which endpoint lists the owner's repositories once the first page resolved it.
type giteaMetadata struct {
	OwnerKind string // "orgs" or "users"
}

type GiteaRepository struct {
	Name        string `json:"name"`
	CloneURL    string `json:"clone_url"`
	Description string `json:"description"`
	Avatar      string `json:"avatar_url"`

	Private  bool `json:"private"`
	Archived bool `json:"archived"`
	Empty    bool `json:"empty"`
	Fork     bool `json:"fork"`

	OpenIssues *int `json:"open_issues_count"`
	OpenPRs    *int `json:"open_pr_counter"`
	Releases   *int `json:"release_counter"`
}

type GiteaRelease struct {
	Name        string       `json:"name"`
	TagName     string       `json:"tag_name"`
	Description string       `json:"body"`
	CreatedAt   string       `json:"created_at"`
	Assets      []GiteaAsset `json:"assets"`
}

type GiteaAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// giteaPageSize is Gitea's default maximum page size.
const giteaPageSize = 50

func NewGitea(config configuration.ConfigSource) *Gitea {
	u, _ := url.Parse(config.BaseURL)

	return &Gitea{
		URL:   *u,
		Token: config.Auth.Token,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

func (g *Gitea) Paginate(username string, groupCfg configuration.ConfigGroup, prev *PaginationResponse) (*PaginationResponse, error) {
	if prev == nil {
		prev = &PaginationResponse{NextPage: 1}
	}

	meta, _ := prev.Metadata.(*giteaMetadata)
	if meta == nil {
		meta = &giteaMetadata{}
	}

	// Organisations list their repositories (including the private ones the token may see) under
	// /orgs, while plain users only exist under /users. The first page probes /orgs and falls back;
	// later pages reuse whichever endpoint answered.
	kinds := []string{"orgs", "users"}
	if meta.OwnerKind != "" {
		kinds = []string{meta.OwnerKind}
	}

	for _, kind := range kinds {
		status, body, _, err := g.get([]string{"api", "v1", kind, username, "repos"}, url.Values{
			"page":  {fmt.Sprint(prev.NextPage)},
			"limit": {fmt.Sprint(giteaPageSize)},
		})
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			continue
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("failed to fetch repositories: received status code %d", status)
		}

		var fetched []GiteaRepository
		if err := json.Unmarshal(body, &fetched); err != nil {
			return nil, err
		}

		repos := make([]SourceRepository, len(fetched))
		for i, repo := range fetched {
			repos[i] = SourceRepository{
				Name:        repo.Name,
				URL:         repo.CloneURL,
				Description: optionalString(repo.Description),
				Avatar:      optionalString(repo.Avatar),
				Private:     repo.Private,
				Archived:    repo.Archived,
				Empty:       repo.Empty,
				Forked:      repo.Fork,
				Stats: RepositoryStats{
					Releases:     repo.Releases,
					Issues:       StateCounts{Open: repo.OpenIssues},
					PullRequests: StateCounts{Open: repo.OpenPRs},
				},
			}
		}

		meta.OwnerKind = kind
		return &PaginationResponse{
			Repositories: repos,
			NextPage:     prev.NextPage + 1,
			Metadata:     meta,
		}, nil
	}

	return nil, fmt.Errorf("owner %q was not found as an organisation or a user", username)
}

func (g *Gitea) GetWikiURL(username, repoName string) string {
	u := g.URL.JoinPath(username, repoName+".wiki.git")
	if g.Token != "" {
		u.User = url.User(g.Token)
	}
	return u.String()
}

func (g *Gitea) FetchReleases(username string, repo SourceRepository) ([]SourceRelease, error) {
	status, body, _, err := g.get([]string{"api", "v1", "repos", username, repo.Name, "releases"}, url.Values{
		"page":  {"1"},
		"limit": {fmt.Sprint(giteaPageSize)},
	})
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		// The repository has its releases unit disabled.
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch releases: received status code %d", status)
	}

	var fetched []GiteaRelease
	if err := json.Unmarshal(body, &fetched); err != nil {
		return nil, err
	}

	releases := make([]SourceRelease, len(fetched))
	for i, rel := range fetched {
		assets := make([]SourceAsset, len(rel.Assets))
		for j, asset := range rel.Assets {
			assets[j] = SourceAsset{
				Name: asset.Name,
				URL:  asset.BrowserDownloadURL,
			}
		}

		releases[i] = SourceRelease{
			Name:        rel.Name,
			TagName:     rel.TagName,
			Description: rel.Description,
			CreatedAt:   rel.CreatedAt,
			Assets:      assets,
		}
	}

	return utils.Reverse(releases), nil
}

// FetchStats completes the counters the listing provides (releases, open issues, open pull requests)
// with one tiny listing per missing count, read from the X-Total-Count header.
func (g *Gitea) FetchStats(username string, repo SourceRepository, needs StatsNeeds) (RepositoryStats, error) {
	stats := repo.Stats
	repoPath := []string{"api", "v1", "repos", username, repo.Name}

	count := func(segment string, query url.Values) (*int, error) {
		query.Set("limit", "1")
		status, _, header, err := g.get(append(repoPath, segment), query)
		if err != nil {
			return nil, err
		}
		if status == http.StatusConflict {
			return utils.Pointer(0), nil // an empty repository has no commits
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("failed to count %s: received status code %d", segment, status)
		}
		return headerCount(header, "X-Total-Count"), nil
	}

	// Issues and pull requests share the issues endpoint, told apart by type.
	fill := func(counts *StateCounts, need StateNeeds, kind string) error {
		var err error
		if (need.Open || need.All) && counts.Open == nil {
			if counts.Open, err = count("issues", url.Values{"type": {kind}, "state": {"open"}}); err != nil {
				return err
			}
		}
		if (need.Closed || need.All) && counts.Closed == nil {
			if counts.Closed, err = count("issues", url.Values{"type": {kind}, "state": {"closed"}}); err != nil {
				return err
			}
		}
		if need.All && counts.All == nil && counts.Open != nil && counts.Closed != nil {
			counts.All = utils.Pointer(*counts.Open + *counts.Closed)
		}
		return nil
	}

	if err := fill(&stats.Issues, needs.Issues, "issues"); err != nil {
		return stats, err
	}
	if err := fill(&stats.PullRequests, needs.PullRequests, "pulls"); err != nil {
		return stats, err
	}

	var err error
	if needs.Releases && stats.Releases == nil {
		if stats.Releases, err = count("releases", url.Values{}); err != nil {
			return stats, err
		}
	}
	if needs.Commits && stats.Commits == nil {
		query := url.Values{"stat": {"false"}, "verification": {"false"}, "files": {"false"}}
		if stats.Commits, err = count("commits", query); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

// AddTokenToCloneUrl embeds the token as the basic-auth username, which Gitea and Forgejo accept.
func (g *Gitea) AddTokenToCloneUrl(u string) string {
	if g.Token == "" {
		return u
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	parsed.User = url.User(g.Token)
	return parsed.String()
}

func (g *Gitea) FetchUsernamePassword() (string, string) {
	return g.Token, ""
}

// get performs a GET against the instance with the token header and returns the status, body and headers.
func (g *Gitea) get(segments []string, query url.Values) (int, []byte, http.Header, error) {
	u := g.URL.JoinPath(segments...)
	u.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("error creating request: %v", err)
	}
	if g.Token != "" {
		req.Header.Set("Authorization", "token "+g.Token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("error reading response body: %v", err)
	}

	return resp.StatusCode, body, resp.Header, nil
}

// optionalString maps the API's empty strings to nil, like the other sources report missing values.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

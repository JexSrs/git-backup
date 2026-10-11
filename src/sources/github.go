package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"main/src/configuration"
	"main/src/utils"
	"net/http"
	"net/url"
)

type Github struct {
	Token string

	client *http.Client
}

type GithubResponse struct {
	Items []GithubRepository `json:"items"`
}

type GithubRepository struct {
	Name        string  `json:"name"`
	URL         string  `json:"clone_url"`
	Description *string `json:"description"`
	License     struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"license"`
	Topics   []string `json:"topics"`
	Language string   `json:"language"`

	Private        bool `json:"private"`
	Archived       bool `json:"archived"`
	HasPages       bool `json:"has_pages"`
	HasDiscussions bool `json:"has_discussions"`
	IsFork         bool `json:"fork"`

	IssuesEnabled bool `json:"has_issues"`
	OpenIssues    int  `json:"open_issues_count"`

	Stars    int `json:"stargazers_count"`
	Watchers int `json:"watchers_count"`
	Forks    int `json:"forks_count"`
	Size     int `json:"size"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type GithubRelease struct {
	Name        string        `json:"name"`
	TagName     string        `json:"tag_name"`
	Description string        `json:"body"`
	CreatedAt   string        `json:"created_at"`
	Assets      []SourceAsset `json:"assets"`
}

type GithubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadUrl string `json:"browser_download_url"`
}

func NewGithub(config configuration.ConfigSource) *Github {
	return &Github{
		Token: config.Auth.Token,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

func (g *Github) Paginate(username string, groupCfg configuration.ConfigGroup, prev *PaginationResponse) (*PaginationResponse, error) {
	if prev == nil {
		prev = &PaginationResponse{NextPage: 1}
	}

	urlPath := fmt.Sprintf("https://api.github.com/search/repositories?q=user:%s+fork:true&per_page=100&page=%d", username, prev.NextPage)

	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	if len(g.Token) > 0 {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	var res GithubResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	repos := make([]SourceRepository, 0)
	for _, repo := range res.Items {
		repos = append(repos, SourceRepository{
			Name:        repo.Name,
			Description: repo.Description,
			URL:         repo.URL,
			Private:     repo.Private,
			Archived:    repo.Archived,
			Forked:      repo.IsFork,
			Empty:       repo.Size == 0,
		})
	}

	return &PaginationResponse{
		Repositories: repos,
		NextPage:     prev.NextPage + 1,
	}, nil
}

func (g *Github) GetWikiURL(username, repoName string) string {
	return fmt.Sprintf("https://%s:x-oauth-basic@github.com/%s/%s.wiki.git", g.Token, username, repoName)
}

func (g *Github) FetchReleases(username string, repo SourceRepository) ([]SourceRelease, error) {
	urlPath := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=10", username, repo.Name)

	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.Token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	fetchedReleases := make([]GithubRelease, 0)
	if err := json.Unmarshal(body, &fetchedReleases); err != nil {
		return nil, fmt.Errorf("error decoding JSON to map: %v", err)
	}

	releases := make([]SourceRelease, len(fetchedReleases))
	for i, rel := range fetchedReleases {
		assets := make([]SourceAsset, len(rel.Assets))
		for i, asset := range rel.Assets {
			assets[i] = SourceAsset{
				Name: asset.Name,
				URL:  asset.URL,
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

// githubStatsQuery reads every count the filters can ask for in one request. GitHub's REST API no
// longer reports issue totals (its issue listing is cursor paginated), so GraphQL is the only exact source.
const githubStatsQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    releases { totalCount }
    defaultBranchRef { target { ... on Commit { history { totalCount } } } }
    issuesAll: issues { totalCount }
    issuesOpen: issues(states: [OPEN]) { totalCount }
    issuesClosed: issues(states: [CLOSED]) { totalCount }
    prsAll: pullRequests { totalCount }
    prsOpen: pullRequests(states: [OPEN]) { totalCount }
    prsClosed: pullRequests(states: [CLOSED, MERGED]) { totalCount }
  }
}`

type githubCount struct {
	TotalCount int `json:"totalCount"`
}

type githubStatsResponse struct {
	Data struct {
		Repository *struct {
			Releases         githubCount `json:"releases"`
			DefaultBranchRef *struct {
				Target struct {
					History githubCount `json:"history"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
			IssuesAll    githubCount `json:"issuesAll"`
			IssuesOpen   githubCount `json:"issuesOpen"`
			IssuesClosed githubCount `json:"issuesClosed"`
			PrsAll       githubCount `json:"prsAll"`
			PrsOpen      githubCount `json:"prsOpen"`
			PrsClosed    githubCount `json:"prsClosed"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// FetchStats fills every count with a single GraphQL query, which needs a token.
func (g *Github) FetchStats(username string, repo SourceRepository, needs StatsNeeds) (RepositoryStats, error) {
	stats := repo.Stats
	if !needs.Any() {
		return stats, nil
	}
	if len(g.Token) == 0 {
		return stats, fmt.Errorf("%w: GitHub needs auth.token to read repository statistics", ErrStatsUnavailable)
	}

	payload, err := json.Marshal(map[string]any{
		"query":     githubStatsQuery,
		"variables": map[string]string{"owner": username, "name": repo.Name},
	})
	if err != nil {
		return stats, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(payload))
	if err != nil {
		return stats, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return stats, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return stats, fmt.Errorf("failed to fetch repository statistics: received status code %d", resp.StatusCode)
	}

	var res githubStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return stats, err
	}
	if len(res.Errors) > 0 {
		return stats, fmt.Errorf("failed to fetch repository statistics: %s", res.Errors[0].Message)
	}
	if res.Data.Repository == nil {
		return stats, fmt.Errorf("failed to fetch repository statistics: repository %s/%s not found", username, repo.Name)
	}

	r := res.Data.Repository
	commits := 0
	if r.DefaultBranchRef != nil {
		commits = r.DefaultBranchRef.Target.History.TotalCount
	}
	stats.Releases = utils.Pointer(r.Releases.TotalCount)
	stats.Commits = utils.Pointer(commits)
	stats.Issues = StateCounts{
		All:    utils.Pointer(r.IssuesAll.TotalCount),
		Open:   utils.Pointer(r.IssuesOpen.TotalCount),
		Closed: utils.Pointer(r.IssuesClosed.TotalCount),
	}
	stats.PullRequests = StateCounts{
		All:    utils.Pointer(r.PrsAll.TotalCount),
		Open:   utils.Pointer(r.PrsOpen.TotalCount),
		Closed: utils.Pointer(r.PrsClosed.TotalCount),
	}
	return stats, nil
}

func (g *Github) AddTokenToCloneUrl(u string) string {
	parsedURL, _ := url.Parse(u)
	parsedURL.User = url.User(g.Token)
	return parsedURL.String()
}

func (g *Github) FetchUsernamePassword() (string, string) {
	return g.Token, ""
}

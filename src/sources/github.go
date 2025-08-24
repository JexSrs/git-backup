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
		Token: config.Token,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

func (g *Github) Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error) {
	if prev == nil {
		prev = &PaginationResponse{NextPage: 1}
	}

	urlPath := fmt.Sprintf("https://api.github.com/search/repositories?q=user:%s&per_page=100&page=%d", username, prev.NextPage)

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

func (g *Github) AddTokenToCloneUrl(u string) string {
	parsedURL, _ := url.Parse(u)
	parsedURL.User = url.User(g.Token)
	return parsedURL.String()
}

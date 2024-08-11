package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"main/src/utils"
	"net/http"
)

type Github struct {
	Token string
}

type GithubRepository struct {
	Name        string  `json:"name"`
	URL         string  `json:"clone_url"`
	Description *string `json:"description"`
}

func NewGithub(token string) *Github {
	return &Github{Token: token}
}

func (g *Github) Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error) {
	page := 1
	if prev != nil {
		page = prev.NextPage
	}

	urlPath := fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100&page=%d", username, page)

	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	if len(g.Token) > 0 {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

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

	githubRepos := make([]GithubRepository, 0)
	if err := json.Unmarshal(body, &githubRepos); err != nil {
		return nil, fmt.Errorf("error decoding JSON to map: %v", err)
	}

	repos := make([]SourceRepository, 0)
	for _, repo := range githubRepos {
		repos = append(repos, SourceRepository{
			Name:        repo.Name,
			Description: repo.Description,
			URL:         repo.URL,
		})
	}

	return &PaginationResponse{
		Repositories: repos,
		NextPage:     page + 1,
	}, nil
}

func (g *Github) GetWikiURL(username, repoName string) string {
	return fmt.Sprintf("https://%s:x-oauth-basic@github.com/%s/%s.wiki.git", g.Token, username, repoName)
}

func (g *Github) FetchReleases(username, repoName string) ([]SourceRelease, error) {
	urlPath := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=10", username, repoName)

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

	repositories := make([]SourceRelease, 0)
	if err := json.Unmarshal(body, &repositories); err != nil {
		return nil, fmt.Errorf("error decoding JSON to map: %v", err)
	}

	return utils.Reverse(repositories), nil
}

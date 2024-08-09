package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Github struct {
	Token string
}

func NewGithub(token string) *Github {
	return &Github{Token: token}
}

func (g *Github) GetID() string {
	return "github"
}

func (g *Github) Paginate(username string, page int) ([]SourceRepository, error) {
	urlPath := fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100&page=%d", username, page)

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

	repositories := make([]SourceRepository, 0)
	if err := json.Unmarshal(body, &repositories); err != nil {
		return nil, fmt.Errorf("error decoding JSON to map: %v", err)
	}

	return repositories, nil
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

	return reverse(repositories), nil
}

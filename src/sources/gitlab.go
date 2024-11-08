package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"main/src/utils"
	"net/http"
)

type Gitlab struct {
	Token string
}

type GitlabRepository struct {
	Name        string  `json:"name"`
	URL         string  `json:"clone_url"`
	Description *string `json:"description"`
}

func NewGitlab(token string) *Gitlab {
	return &Gitlab{Token: token}
}

type GitlabMetadata struct {
	What string
}

func (g *Gitlab) Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error) {
	return prev, nil
}

func (g *Gitlab) GetWikiURL(username, repoName string) string {
	return fmt.Sprintf("https://%s:x-oauth-basic@github.com/%s/%s.wiki.git", g.Token, username, repoName)
}

func (g *Gitlab) FetchReleases(username, repoName string) ([]SourceRelease, error) {
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

func (g *Gitlab) AddTokenToCloneUrl(url string) string {
	// TODO
	return url
}

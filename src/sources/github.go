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

func (g *Github) Paginate(username string, page int) ([]SourceRepository, error) {
	url := fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100&page=%d", username, page)

	req, err := http.NewRequest("GET", url, nil)
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

	// Initially parse to a slice of maps
	var rawRepos []map[string]interface{}
	if err := json.Unmarshal(body, &rawRepos); err != nil {
		return nil, fmt.Errorf("error decoding JSON to map: %v", err)
	}

	// Convert maps to SourceRepository structs
	repositories := make([]SourceRepository, 0)
	for _, rawRepo := range rawRepos {
		description := ""
		if str, ok := rawRepo["description"].(string); ok {
			description = str
		}

		repo := SourceRepository{
			Name:        rawRepo["name"].(string),
			URL:         rawRepo["clone_url"].(string),
			Description: description,
		}
		repositories = append(repositories, repo)
	}

	return repositories, nil
}

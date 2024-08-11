package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type HuggingFace struct {
	Token string
}

type HuggingFaceRepository struct {
	ID string `json:"id"`
}

type HuggingFaceMetadata struct {
	What string
}

func NewHuggingFace(token string) *HuggingFace {
	return &HuggingFace{Token: token}
}

func (g *HuggingFace) Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error) {
	if prev == nil {
		prev = &PaginationResponse{
			Metadata: HuggingFaceMetadata{
				What: "models",
			},
		}
	}

	meta := prev.Metadata.(HuggingFaceMetadata)

	urlPath := ""
	if meta.What == "models" {
		urlPath = fmt.Sprintf("https://huggingface.co/api/models?author=%s&limit=100", username)
	} else if meta.What == "datasets" {
		urlPath = fmt.Sprintf("https://huggingface.co/api/datasets?author=%s&limit=100", username)
	}

	if prev.NextCursor != nil {
		urlPath = *prev.NextCursor
	}

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

	nextCursor := extractLink(resp.Header.Get("Link"))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	githubRepos := make([]HuggingFaceRepository, 0)
	if err := json.Unmarshal(body, &githubRepos); err != nil {
		return nil, fmt.Errorf("error decoding JSON to map: %v", err)
	}

	repos := make([]SourceRepository, 0)
	for _, repo := range githubRepos {
		repos = append(repos, SourceRepository{
			Name:        strings.Split(repo.ID, "/")[1],
			Description: nil,
			URL:         fmt.Sprintf("https://huggingface.co/%s.git", repo.ID),
		})
	}

	// If finished models, go to datasets
	if len(repos) == 0 && meta.What == "models" {
		meta.What = "datasets"
		prev.Metadata = meta
		return g.Paginate(username, prev)
	}

	return &PaginationResponse{
		Repositories: repos,
		NextCursor:   &nextCursor,
		Metadata:     meta,
	}, nil
}

func (g *HuggingFace) GetWikiURL(username, repoName string) string {
	return ""
}

func (g *HuggingFace) FetchReleases(username, repoName string) ([]SourceRelease, error) {
	return nil, nil
}

func extractLink(h string) string {
	if len(h) == 0 {
		return ""
	}

	// Regular expression to match the URL
	re := regexp.MustCompile(`<([^>]+)>`)

	// Find the URL
	matches := re.FindStringSubmatch(h)
	return matches[1]
}

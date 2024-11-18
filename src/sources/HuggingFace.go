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

	client *http.Client
}

type HuggingFaceRepository struct {
	ID string `json:"id"`
}

type HuggingFaceMetadata struct {
	What string
}

func NewHuggingFace(token string) *HuggingFace {
	return &HuggingFace{
		Token:  token,
		client: &http.Client{},
	}
}

func (g *HuggingFace) Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error) {
	if prev == nil {
		prev = &PaginationResponse{
			NextCursor: nil,
			Metadata:   HuggingFaceMetadata{What: "models"},
		}
	}

	meta := prev.Metadata.(HuggingFaceMetadata)

	var res *PaginationResponse
	var err error
	if prev.NextCursor != nil {
		res, err = g.fetchRepositories(*prev.NextCursor)
	} else {
		if meta.What == "models" {
			res, err = g.fetchRepositories(fmt.Sprintf("https://huggingface.co/api/models?author=%s&limit=100", username))
			if err != nil {
				return nil, err
			}

			// If finished models, go to datasets
			if len(res.Repositories) == 0 {
				meta.What = "datasets"
			}
		}

		if meta.What == "datasets" {
			res, err = g.fetchRepositories(fmt.Sprintf("https://huggingface.co/api/datasets?author=%s&limit=100", username))
		}
	}

	if err != nil {
		return nil, err
	}

	res.Metadata = meta
	return res, nil
}

func (g *HuggingFace) fetchRepositories(cursor string) (*PaginationResponse, error) {
	req, err := http.NewRequest(http.MethodGet, cursor, nil)
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

	nextCursor := extractLink(resp.Header.Get("Link"))

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
			Private:     false,
		})
	}

	return &PaginationResponse{
		Repositories: repos,
		NextCursor:   &nextCursor,
		Metadata:     nil,
	}, nil
}

func (g *HuggingFace) GetWikiURL(username, repoName string) string {
	return ""
}

func (g *HuggingFace) FetchReleases(username string, repo SourceRepository) ([]SourceRelease, error) {
	return nil, nil
}

func (g *HuggingFace) AddTokenToCloneUrl(url string) string {
	// TODO
	return url
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

package main

import (
	"encoding/json"
	"fmt"
	"main/src/sources"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Project struct {
	Destination           *GitLab
	DestinationRepository *ProjectGitLab

	Source           sources.Source
	SourceUsername   string
	SourceRepository sources.SourceRepository
}

type ProjectGitLab struct {
	ID            *int
	HttpUrl       *string
	ParentGroupID int

	IsImported bool
	IsCloned   bool
}

func (g *Project) GetRepoPath() string {
	return fmt.Sprintf("/tmp/git-backup/%s/%s", g.SourceUsername, g.SourceRepository.Name)
}

func (g *Project) RetrieveExistingRepo() (int, error) {
	urlPath := fmt.Sprintf("/api/v4/groups/%d/projects?search=%s&per_page=100", g.DestinationRepository.ParentGroupID, g.SourceRepository.Name)
	body, err := g.Destination.Request(http.MethodGet, urlPath, nil)
	if err != nil {
		return -1, err
	}

	var projects []struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		HttpUrlToRepo string `json:"http_url_to_repo"`
	}
	err = json.Unmarshal(body, &projects)
	if err != nil {
		return -1, err
	}

	lowercaseRepoName := strings.ToLower(g.SourceRepository.Name)
	for _, project := range projects {
		if strings.ToLower(project.Name) == lowercaseRepoName {
			g.DestinationRepository.ID = &project.ID
			g.DestinationRepository.HttpUrl = &project.HttpUrlToRepo

			return project.ID, nil
		}
	}

	return -1, nil
}

func (g *Project) Import() (int, error) {
	data := url.Values{}
	data.Set("name", g.SourceRepository.Name)
	data.Set("namespace_id", strconv.Itoa(g.DestinationRepository.ParentGroupID))
	data.Set("import_url", g.SourceRepository.URL)
	data.Set("description", g.SourceRepository.Description)

	body, err := g.Destination.Request(http.MethodPost, "/api/v4/projects", []byte(data.Encode()))
	if err != nil {
		return -1, fmt.Errorf("creating request: %w", err)
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return -1, fmt.Errorf("parsing JSON response: %w", err)
	}

	g.DestinationRepository.ID = &result.ID
	g.DestinationRepository.IsImported = true
	return result.ID, nil
}

func (g *Project) LockUntilImport() error {
	urlPath := fmt.Sprintf("/api/v4/projects/%d", g.DestinationRepository.ID)

	for {
		body, err := g.Destination.Request(http.MethodGet, urlPath, nil)
		if err != nil {
			return err
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return err
		}

		importStatus := result["import_status"].(string)
		switch importStatus {
		case "finished":
			return nil
		case "failed":
			return fmt.Errorf("current import status: %s", importStatus)
		default:
			fmt.Printf("- Current import status: %s\n", importStatus)
			time.Sleep(5 * time.Second)
		}
	}
}

func (g *Project) SetOriginalURL() error {
	urlPath := fmt.Sprintf("/api/v4/projects/%d/variables", g.DestinationRepository.ID)
	data := "key=original_url&value=" + g.SourceRepository.URL

	_, err := g.Destination.Request(http.MethodPost, urlPath, []byte(data))
	return err
}

func (g *Project) GetProtectedBranches() ([]string, error) {
	urlPath := fmt.Sprintf("/api/v4/projects/%d/protected_branches", g.DestinationRepository.ID)
	body, err := g.Destination.Request(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, err
	}

	branches := make([]struct {
		Name string `json:"name"`
	}, 0)
	if err = json.Unmarshal(body, &branches); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(branches))
	for _, branch := range branches {
		names = append(names, branch.Name)
	}

	return names, nil
}

func (g *Project) UnprotectBranch(name string) error {
	encodedBranch := url.QueryEscape(name)

	urlPath := fmt.Sprintf("/api/v4/projects/%d/protected_branches/%s", g.DestinationRepository.ID, encodedBranch)
	_, err := g.Destination.Request(http.MethodDelete, urlPath, nil)
	return err
}

func (g *Project) LinkAsset(tagName, assetName, assetUrl string) error {
	return nil
}

func (g *Project) CloneFromSource() error {
	err := g.Source.Clone(g.SourceRepository.URL, g.GetRepoPath())
	if err != nil {
		return err
	}

	g.DestinationRepository.IsCloned = true
	return nil
}

func (g *Project) LinkDestinationUrlToRepo() error {
	if !g.DestinationRepository.IsCloned {
		return nil // just skip
	}

	// Create url
	parsedURL, _ := url.Parse(g.Destination.URL.String())
	parsedURL.User = url.UserPassword("oauth2", g.Destination.APIToken)

	// Link it
	path := g.GetRepoPath()

	return nil
}

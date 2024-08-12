package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"main/src/sources"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Project struct {
	Config ConfigRepo

	Destination           *GitLab
	DestinationRepository *ProjectGitLab
	DestinationStorage    *Dufs

	Source           sources.Source
	SourceUsername   string
	SourceRepository sources.SourceRepository

	Repo *git.Repository
}

type ProjectGitLab struct {
	ID                *int    `json:"id"`
	Name              string  `json:"name"`
	HttpUrl           *string `json:"http_url_to_repo"`
	PathWithNamespace *string `json:"path_with_namespace"`
	ParentGroupID     int
}

func NewProject(gitlab *GitLab, dufs *Dufs, groupId int, source sources.Source, username string, sourceRepository sources.SourceRepository, config ConfigRepo) *Project {
	return &Project{
		Destination: gitlab,
		DestinationRepository: &ProjectGitLab{
			ID:            nil,
			HttpUrl:       nil,
			ParentGroupID: groupId,
		},
		DestinationStorage: dufs,
		SourceUsername:     username,
		Source:             source,
		SourceRepository:   sourceRepository,
		Config:             config,
	}
}

func (g *Project) RetrieveExistingRepo() (int, error) {
	data := url.Values{}
	data.Add("search", g.SourceRepository.Name)
	data.Add("per_page", "100")

	urlPath := fmt.Sprintf("/api/v4/groups/%d/projects?%s", g.DestinationRepository.ParentGroupID, data.Encode())
	body, err := g.Destination.Request(http.MethodGet, urlPath, nil)
	if err != nil {
		return -1, err
	}

	if body.Status == http.StatusNotFound {
		return -1, nil
	}

	var projects []ProjectGitLab
	err = json.Unmarshal(body.Body, &projects)
	if err != nil {
		return -1, err
	}

	lowercaseRepoName := strings.ToLower(g.SourceRepository.Name)
	for _, project := range projects {
		if strings.ToLower(project.Name) == lowercaseRepoName {
			g.DestinationRepository.ID = project.ID
			g.DestinationRepository.HttpUrl = project.HttpUrl
			g.DestinationRepository.PathWithNamespace = project.PathWithNamespace

			return *project.ID, nil
		}
	}

	return -1, nil
}

func (g *Project) Import() (int, error) {
	data := url.Values{}
	data.Add("name", g.SourceRepository.Name)
	data.Add("namespace_id", strconv.Itoa(g.DestinationRepository.ParentGroupID))
	data.Add("import_url", g.SourceRepository.URL)

	if g.SourceRepository.Description != nil {
		data.Add("description", *g.SourceRepository.Description)
	}

	body, err := g.Destination.Request(http.MethodPost, "/api/v4/projects", []byte(data.Encode()))
	if err != nil {
		return -1, fmt.Errorf("creating request: %w", err)
	}

	if body.Status != http.StatusCreated {
		return -1, fmt.Errorf("invalid response: %s", body.Body)
	}

	var result ProjectGitLab
	if err := json.Unmarshal(body.Body, &result); err != nil {
		return -1, fmt.Errorf("parsing JSON response: %w", err)
	}

	g.DestinationRepository.ID = result.ID
	g.DestinationRepository.HttpUrl = result.HttpUrl
	g.DestinationRepository.PathWithNamespace = result.PathWithNamespace

	return *result.ID, nil
}

func (g *Project) SetOriginalURL() error {
	data := url.Values{}
	data.Add("key", "original_url")
	data.Add("value", g.SourceRepository.URL)

	urlPath := fmt.Sprintf("/api/v4/projects/%d/variables", *g.DestinationRepository.ID)
	_, err := g.Destination.Request(http.MethodPost, urlPath, []byte(data.Encode()))
	return err
}

func (g *Project) LockUntilImport() error {
	urlPath := fmt.Sprintf("/api/v4/projects/%d", *g.DestinationRepository.ID)

	for {
		body, err := g.Destination.Request(http.MethodGet, urlPath, nil)
		if err != nil {
			return err
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body.Body, &result); err != nil {
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

func (g *Project) GetProtectedBranches() ([]string, error) {
	urlPath := fmt.Sprintf("/api/v4/projects/%d/protected_branches", *g.DestinationRepository.ID)
	body, err := g.Destination.Request(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, err
	}

	branches := make([]struct {
		Name string `json:"name"`
	}, 0)
	if err = json.Unmarshal(body.Body, &branches); err != nil {
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

	urlPath := fmt.Sprintf("/api/v4/projects/%d/protected_branches/%s", *g.DestinationRepository.ID, encodedBranch)
	_, err := g.Destination.Request(http.MethodDelete, urlPath, nil)
	return err
}

func (g *Project) CloneFromSource() error {
	path := g.GetDir()
	os.RemoveAll(path)

	r, err := git.PlainClone(path, false, &git.CloneOptions{
		URL: g.SourceRepository.URL,
	})

	if err != nil {
		return err
	}

	g.Repo = r
	return nil
}

func (g *Project) AddRemoteToRepo() error {
	if g.Repo == nil {
		return fmt.Errorf("no repository found for project %d", *g.DestinationRepository.ID)
	}

	// Create url
	parsedURL, _ := url.Parse(*g.DestinationRepository.HttpUrl)
	parsedURL.User = url.UserPassword("oauth2", g.Destination.APIToken)

	// Add a new remote, named "gitlab"
	_, err := g.Repo.CreateRemote(&config.RemoteConfig{
		Name: "gitlab",
		URLs: []string{parsedURL.String()},
	})
	return err
}

func (g *Project) GetBranches() ([]string, error) {
	if g.Repo == nil {
		return nil, fmt.Errorf("no repository found for project %d", *g.DestinationRepository.ID)
	}

	branches, err := g.Repo.Branches()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	err = branches.ForEach(func(ref *plumbing.Reference) error {
		names = append(names, ref.Name().String())
		return nil
	})
	if err != nil {
		fmt.Printf("Error iterating branches: %v\n", err)
	}

	return names, nil
}

func (g *Project) PushBranch(name string) error {
	if g.Repo == nil {
		return fmt.Errorf("no repository found for project %d", *g.DestinationRepository.ID)
	}

	pushOptions := &git.PushOptions{
		RemoteName: "gitlab",
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/" + name + ":refs/heads/" + name),
		},
		Force: true,
	}

	// Perform the push
	if err := g.Repo.Push(pushOptions); err != nil && err.Error() != "already up-to-date" {
		return err
	}

	return nil
}

func (g *Project) PushAllTags() error {
	if g.Repo == nil {
		return fmt.Errorf("no repository found for project %d", *g.DestinationRepository.ID)
	}

	pushOptions := &git.PushOptions{
		RemoteName: "gitlab",
		RefSpecs:   []config.RefSpec{"refs/tags/*:refs/tags/*"},
		Force:      true,
	}

	// Perform the push
	if err := g.Repo.Push(pushOptions); err != nil && err.Error() != "already up-to-date" {
		return err
	}

	return nil
}

func (g *Project) ReleaseExists(tagName string) (bool, error) {
	tagNameEncoded := url.QueryEscape(tagName)
	urlPath := fmt.Sprintf("/api/v4/projects/%d/releases/%s", *g.DestinationRepository.ID, tagNameEncoded)

	body, err := g.Destination.Request(http.MethodGet, urlPath, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	return body.Status != http.StatusNotFound, nil
}

func (g *Project) CreateRelease(release sources.SourceRelease) error {
	data := url.Values{}
	data.Add("name", release.Name)
	data.Add("tag_name", release.TagName)
	data.Add("description", release.Description)
	data.Add("released_at", release.CreatedAt)

	urlPath := fmt.Sprintf("/api/v4/projects/%d/releases", *g.DestinationRepository.ID)

	body, err := g.Destination.Request(http.MethodPost, urlPath, []byte(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if body.Status != http.StatusCreated {
		return fmt.Errorf("create release: status %d", body.Status)
	}

	return nil
}

func (g *Project) GetDir() string {
	return filepath.Join("/tmp/git-backup/", g.SourceUsername, g.SourceRepository.Name)
}

func (g *Project) LinkAsset(tagName, assetName, assetUrl string) error {
	encodedTagName := url.QueryEscape(tagName)

	data := url.Values{}
	data.Add("name", assetName)
	data.Add("url", assetUrl)

	urlPath := fmt.Sprintf("/api/v4/projects/%d/releases/%s/assets/links?%s", *g.DestinationRepository.ID, encodedTagName, data.Encode())

	body, err := g.Destination.Request(http.MethodPost, urlPath, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if body.Status != http.StatusCreated {
		return fmt.Errorf("create release: status %d", body.Status)
	}

	return nil
}

func (g *Project) GetWikiProject() *Project {
	dstRepoUrl := fmt.Sprintf("%s/%s.wiki.git", g.Destination.URL.String(), *g.DestinationRepository.PathWithNamespace)

	return &Project{
		Destination: g.Destination,
		DestinationRepository: &ProjectGitLab{
			ID:            nil,
			HttpUrl:       &dstRepoUrl,
			ParentGroupID: -1,
		},
		SourceUsername: g.SourceUsername,
		Source:         g.Source,
		SourceRepository: sources.SourceRepository{
			Name:        fmt.Sprintf("%s.wiki", g.SourceRepository.Name),
			URL:         g.Source.GetWikiURL(g.SourceUsername, g.SourceRepository.Name),
			Description: nil,
		},
	}
}

func (g *Project) Prune() {
	os.RemoveAll(g.GetDir())
}

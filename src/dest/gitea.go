package dest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/go-git/go-git/v5/config"
	"io"
	"main/src/configuration"
	"main/src/sources"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Gitea struct {
	ID       string
	URL      url.URL
	APIToken string

	client *http.Client
}

func NewGitea(config configuration.ConfigDestination) *Gitea {
	giteaUrl, _ := url.Parse(config.URL)

	return &Gitea{
		ID:       config.ID,
		URL:      *giteaUrl,
		APIToken: config.Token,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

type giteaResponse struct {
	Status int
	Body   []byte
}

type giteaProject struct {
	ID                *int    `json:"id"`
	Name              string  `json:"name"`
	HttpUrl           *string `json:"html_url"`
	PathWithNamespace *string `json:"full_name"`
	Empty             bool    `json:"empty"`
}

type giteaRelease struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	TagName string `json:"tag_name"`
}

func (g *Gitea) request(method, path string, data *bytes.Buffer, contentType string) (*giteaResponse, error) {
	pathQuery := strings.Split(path, "?")

	_path := g.URL.JoinPath(pathQuery[0])
	if len(pathQuery) > 1 {
		_path.RawQuery = pathQuery[1]
	}

	var req *http.Request
	var err error

	if data != nil {
		req, err = http.NewRequest(method, _path.String(), data)
		if len(contentType) != 0 {
			req.Header.Set("Content-Type", contentType)
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequest(method, _path.String(), nil)
	}

	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", fmt.Sprintf("token %s", g.APIToken))
	req.Header.Add("Accept", "*/*")

	res, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return &giteaResponse{
		Status: res.StatusCode,
		Body:   body,
	}, nil
}

func (g *Gitea) GetIdentification() DestinationID {
	return DestinationID{
		ID: g.ID,
		Repository: DestinationIDRepository{
			Avatar:   true,
			Wiki:     true,
			Releases: true,
		},
	}
}

func (g *Gitea) RetrieveExistingRepo(gConfig configuration.ConfigGroup, remote sources.SourceRepository) (*Repository, error) {
	_path := fmt.Sprintf("/api/v1/repos/%s/%s", gConfig.GiteaUsername, strings.ReplaceAll(remote.Name, " ", "-"))
	body, err := g.request(http.MethodGet, _path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if body.Status == http.StatusNotFound {
		return nil, nil
	}

	var project giteaProject
	if err := json.Unmarshal(body.Body, &project); err != nil {
		return nil, err
	}

	return &Repository{
		ID:                fmt.Sprintf("%d", *project.ID),
		Name:              project.Name,
		HttpUrl:           *project.HttpUrl,
		PathWithNamespace: *project.PathWithNamespace,
		FinishedMiration:  !project.Empty,
		Remote:            remote,
		ConfigGroup:       gConfig,
	}, nil
}

func (g *Gitea) ImportRepository(gConfig configuration.ConfigGroup, config configuration.ConfigRepo, remote sources.SourceRepository, source sources.Source) (*Repository, error) {
	repoName := strings.ReplaceAll(remote.Name, " ", "-")
	data := map[string]any{
		"clone_addr":    source.AddTokenToCloneUrl(remote.URL),
		"issues":        false,
		"labels":        false,
		"lfs":           *config.LFS,
		"milestones":    false,
		"mirror":        false,
		"private":       false,
		"pull_requests": false,
		"releases":      false,
		"repo_name":     repoName,
		"repo_owner":    gConfig.GiteaUsername,
		"service":       "git",
		"wiki":          false,
	}

	if remote.Description != nil {
		data["description"] = *remote.Description
	}

	js, _ := json.Marshal(data)
	res, err := g.request(http.MethodPost, "/api/v1/repos/migrate", bytes.NewBuffer(js), "application/json")
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		// In case of timeout, wait for repo to finish importing
		return g.RetrieveExistingRepo(gConfig, remote)
	}

	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if res.Status == http.StatusRequestTimeout || res.Status == http.StatusGatewayTimeout {
		// In case of timeout, wait for repo to finish importing
		return g.RetrieveExistingRepo(gConfig, remote)
	}

	if res.Status != http.StatusCreated {
		if res.Status == http.StatusInternalServerError || res.Status == http.StatusUnprocessableEntity {
			// Delete repository if failed
			g.request(http.MethodDelete, fmt.Sprintf("/api/v1/repos/%s/%s", gConfig.GiteaUsername, repoName), nil, "")
		}

		return nil, fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	var result giteaProject
	if err := json.Unmarshal(res.Body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	return &Repository{
		ID:                fmt.Sprintf("%d", *result.ID),
		Name:              result.Name,
		HttpUrl:           *result.HttpUrl,
		PathWithNamespace: *result.PathWithNamespace,
		Remote:            remote,
		ConfigGroup:       gConfig,
	}, nil
}

func (g *Gitea) LockUntilImport(repo *Repository, ping func(string)) error {
	for {
		_path := fmt.Sprintf("/api/v1/repos/%s/%s", repo.ConfigGroup.GiteaUsername, repo.Name)
		body, err := g.request(http.MethodGet, _path, nil, "")
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		if body.Status == http.StatusNotFound {
			return fmt.Errorf("repository not found")
		}

		var project giteaProject
		if err := json.Unmarshal(body.Body, &project); err != nil {
			return err
		}

		switch project.Empty {
		case false:
			return nil
		case true:
			ping("waiting")
			time.Sleep(5 * time.Second)
		}
	}
}

func (g *Gitea) SetOriginalUrl(repo *Repository, originUrl string) error {
	data, _ := json.Marshal(map[string]any{
		"value":       originUrl,
		"description": "Original url of the migrated repository. Migration happened using git-backup application",
	})

	_path := fmt.Sprintf("/api/v1/repos/%s/%s/actions/variables/original_url", repo.ConfigGroup.GiteaUsername, repo.Name)
	res, err := g.request(http.MethodPost, _path, bytes.NewBuffer(data), "application/json")
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if res.Status == http.StatusConflict {
		return nil // Url already set
	}

	if res.Status != http.StatusNoContent {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return nil
}

func (g *Gitea) GetProtectedBranches(repo *Repository) ([]string, error) {
	names := make([]string, 0)

	page := 0
	for {
		_path := fmt.Sprintf("/api/v1/repos/%s/%s/branches?page=%d&limit=20", repo.ConfigGroup.GiteaUsername, repo.Name, page)
		body, err := g.request(http.MethodGet, _path, nil, "")
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		page++
		branches := make([]struct {
			Name      string `json:"name"`
			Protected bool   `json:"protected"`
		}, 0)
		if err = json.Unmarshal(body.Body, &branches); err != nil {
			return nil, err
		}

		if len(branches) == 0 {
			break
		}

		for _, branch := range branches {
			if branch.Protected {
				names = append(names, branch.Name)
			}
		}
	}

	return names, nil
}

func (g *Gitea) UnprotectBranch(repo *Repository, branch string) error {
	_path := fmt.Sprintf("/api/v1/repos/%s/%s/branche_protections/%s", repo.ConfigGroup.GiteaUsername, repo.Name, branch)
	res, err := g.request(http.MethodDelete, _path, nil, "")
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if res.Status != http.StatusNoContent {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return err
}

func (g *Gitea) AddRemoteToRepo(repo *Repository) error {
	if repo.LocalRepository == nil {
		return fmt.Errorf("no local repository found for gitlabProject %d", repo.ID)
	}

	// Create url
	parsedURL, _ := url.Parse(repo.HttpUrl)
	parsedURL.User = url.UserPassword("oauth2", g.APIToken)

	// Add a new remote, named "gitlab"
	_, err := repo.LocalRepository.CreateRemote(&config.RemoteConfig{
		Name: g.ID,
		URLs: []string{parsedURL.String()},
	})
	return err
}

func (g *Gitea) ChangeArchivedState(repo *Repository, isArchived bool) error {
	data, _ := json.Marshal(map[string]any{
		"archived": isArchived,
	})

	_path := fmt.Sprintf("/api/v1/repos/%s/%s", repo.ConfigGroup.GiteaUsername, repo.Name)
	res, err := g.request(http.MethodPatch, _path, bytes.NewBuffer(data), "application/json")
	if err != nil {
		return err
	}

	if res.Status != http.StatusOK {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return nil
}

func (g *Gitea) ChangeAvatar(repo *Repository, avatar *bytes.Buffer, ext string) error {
	data, _ := json.Marshal(map[string]any{
		"image": base64.StdEncoding.EncodeToString(avatar.Bytes()),
	})

	_path := fmt.Sprintf("/api/v1/repos/%s/%s/avatar", repo.ConfigGroup.GiteaUsername, repo.Name)
	res, err := g.request(http.MethodPost, _path, bytes.NewBuffer(data), "application/json")
	if err != nil {
		return err
	}

	if res.Status != http.StatusNoContent {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return nil
}

func (g *Gitea) ReleaseExists(repo *Repository, tagName string) (bool, error) {
	release, err := g.fetchReleaseFromTag(repo, tagName)
	if err != nil {
		return false, err
	}

	return release != nil, nil
}

func (g *Gitea) CreateRelease(repo *Repository, release sources.SourceRelease) error {
	data, _ := json.Marshal(map[string]any{
		"draft":      false,
		"name":       release.Name,
		"prerelease": false,
		"body":       release.Description,
		"tag_name":   release.TagName,
	})

	_path := fmt.Sprintf("/api/v1/repos/%s/%s/releases", repo.ConfigGroup.GiteaUsername, repo.Name)
	res, err := g.request(http.MethodPost, _path, bytes.NewBuffer(data), "application/json")
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if res.Status != http.StatusCreated {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return nil
}

func (g *Gitea) LinkAsset(repo *Repository, release sources.SourceRelease, assetName, assetUrl string) error {
	gRelease, err := g.fetchReleaseFromTag(repo, release.TagName)
	if err != nil {
		return err
	}

	if gRelease == nil {
		return fmt.Errorf("no gitea release found for tag %s", release.TagName)
	}

	// Create an in-memory file with assetUrl content
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("attachment", assetName+".txt")
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, strings.NewReader(assetUrl)); err != nil {
		return fmt.Errorf("write file content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}

	_path := fmt.Sprintf("/api/v1/repos/%s/%s/releases/%d/assets?name=%s.txt", repo.ConfigGroup.GiteaUsername, repo.Name, gRelease.ID, assetName)
	body, err := g.request(http.MethodPost, _path, &buf, writer.FormDataContentType())
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if body.Status != http.StatusCreated {
		return fmt.Errorf("create release: status %d", body.Status)
	}

	return nil
}

func (g *Gitea) GetWikiProject(repo *Repository, gConfig configuration.ConfigGroup, source sources.Source) *Repository {
	wikiUrl := source.GetWikiURL(gConfig.GiteaUsername, repo.Remote.Name)
	if len(wikiUrl) == 0 {
		return nil
	}

	return &Repository{
		ID:                "",
		Name:              fmt.Sprintf("%s.wiki", repo.Name),
		HttpUrl:           fmt.Sprintf("%s/%s.wiki.git", g.URL.String(), repo.PathWithNamespace),
		PathWithNamespace: "",
		Remote: sources.SourceRepository{
			ID:              0,
			Name:            fmt.Sprintf("%s.wiki", repo.Remote.Name),
			URL:             wikiUrl,
			Description:     nil,
			Avatar:          nil,
			Private:         false,
			Archived:        false,
			ParentGroupPath: nil,
		},
		LocalRepository:      nil,
		OverrideRemoteBranch: "main",
	}
}

func (g *Gitea) CreateWiki(repo *Repository, gConfig configuration.ConfigGroup) error {
	data, _ := json.Marshal(map[string]any{
		"content_base64": "",
		"message":        "Init wiki project",
		"title":          "_Init_",
	})

	_path := fmt.Sprintf("/api/v1/repos/%s/%s/wiki/new", gConfig.GiteaUsername, repo.Name)
	res, err := g.request(http.MethodPost, _path, bytes.NewBuffer(data), "application/json")
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if res.Status != http.StatusCreated && strings.Contains(string(res.Body), "wiki page already exists") {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return nil
}

func (g *Gitea) UploadFile(buffer *bytes.Buffer, dstPath string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (g *Gitea) fetchReleaseFromTag(repo *Repository, tagName string) (*giteaRelease, error) {
	eTag := url.QueryEscape(tagName)
	_path := fmt.Sprintf("/api/v1/repos/%s/%s/releases/tags/%s", repo.ConfigGroup.GiteaUsername, repo.Name, eTag)
	res, err := g.request(http.MethodGet, _path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if res.Status == http.StatusNotFound {
		return nil, nil
	}

	if res.Status != http.StatusOK {
		return nil, fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	var release giteaRelease
	err = json.Unmarshal(res.Body, &release)
	if err != nil {
		return nil, err
	}

	return &release, nil
}

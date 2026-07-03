package dest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"main/src/configuration"
	"main/src/sources"
	"main/src/utils"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/config"
)

type GitLab struct {
	ID       string
	URL      url.URL
	APIToken string

	client *http.Client
	config *configuration.ConfigDestination
}

func NewGitLab(config configuration.ConfigDestination) *GitLab {
	gitlabUrl, _ := url.Parse(config.URL)

	return &GitLab{
		ID:       config.ID,
		URL:      *gitlabUrl,
		APIToken: config.Token,
		client: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: config.TLSConfig(),
			},
		},
		config: &config,
	}
}

type gitlabResponse struct {
	Status int
	Body   []byte
}

type gitlabProject struct {
	ID                *int    `json:"id"`
	Name              string  `json:"name"`
	HttpUrl           *string `json:"http_url_to_repo"`
	PathWithNamespace *string `json:"path_with_namespace"`
	ImportStatus      string  `json:"import_status"`
	ParentGroupID     int
}

type gitlabReqGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`

	Visibility string  `json:"visibility"`
	Avatar     *string `json:"avatar_url"`
}

func (g *GitLab) request(method, path string, data *bytes.Buffer, contentType string) (*gitlabResponse, error) {
	pathQuery := strings.Split(path, "?")

	urlPath := g.URL.JoinPath(pathQuery[0])
	if len(pathQuery) > 1 {
		urlPath.RawQuery = pathQuery[1]
	}

	var req *http.Request
	var err error

	if data != nil {
		req, err = http.NewRequest(method, urlPath.String(), data)
		if len(contentType) != 0 {
			req.Header.Set("Content-Type", contentType)
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequest(method, urlPath.String(), nil)
	}

	if err != nil {
		return nil, err
	}

	req.Header.Add("Private-Token", g.APIToken)
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

	return &gitlabResponse{
		Status: res.StatusCode,
		Body:   body,
	}, nil
}

func (g *GitLab) GetIdentification() DestinationID {
	return DestinationID{
		ID: g.ID,
		Repository: DestinationIDRepository{
			Avatar:   true,
			Wiki:     true,
			Releases: true,
		},
		Config: g.config,
	}
}

func (g *GitLab) RetrieveExistingRepo(gConfig configuration.ConfigGroup, remote sources.SourceRepository) (*Repository, error) {
	parentGroupId, err := g.RetrieveParentGroup(remote, gConfig.GitlabGroupID)
	if err != nil {
		return nil, fmt.Errorf("fetching parent group: %w", err)
	}

	data := url.Values{}
	data.Add("search", remote.Name)
	data.Add("per_page", "100")

	urlPath := fmt.Sprintf("/api/v4/groups/%d/projects?%s", parentGroupId, data.Encode())
	body, err := g.request(http.MethodGet, urlPath, nil, "")
	if err != nil {
		return nil, err
	}

	if body.Status == http.StatusNotFound {
		return nil, nil
	}

	var projects []gitlabProject
	err = json.Unmarshal(body.Body, &projects)
	if err != nil {
		return nil, err
	}

	lowercaseRepoName := strings.ToLower(remote.Name)
	for _, project := range projects {
		if strings.ToLower(project.Name) == lowercaseRepoName {
			return &Repository{
				ID:                fmt.Sprintf("%d", *project.ID),
				Name:              project.Name,
				HttpUrl:           *project.HttpUrl,
				PathWithNamespace: *project.PathWithNamespace,
				FinishedMiration:  project.ImportStatus == "finished" || project.ImportStatus == "none",
				Remote:            remote,
			}, nil
		}
	}

	return nil, nil
}

func (g *GitLab) ImportRepository(gConfig configuration.ConfigGroup, config configuration.ConfigRepo, remote sources.SourceRepository, source sources.Source) (*Repository, error) {
	if isReservedGitlabName(remote.Name) {
		return nil, fmt.Errorf("repository name %s is reserved", remote.Name)
	}

	if !isValidGitlabName(remote.Name) {
		return nil, fmt.Errorf("invalid repository name: %s", remote.Name)
	}

	parentGroupId, err := g.RetrieveParentGroup(remote, gConfig.GitlabGroupID)
	if err != nil {
		return nil, fmt.Errorf("fetching parent group: %w", err)
	}

	data := url.Values{}
	data.Add("name", remote.Name)
	data.Add("namespace_id", strconv.Itoa(parentGroupId))
	data.Add("import_url", source.AddTokenToCloneUrl(remote.URL))

	if remote.Description != nil {
		data.Add("description", *remote.Description)
	}

	body, err := g.request(http.MethodPost, "/api/v4/projects", bytes.NewBuffer([]byte(data.Encode())), "")
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if body.Status != http.StatusCreated {
		return nil, fmt.Errorf("invalid response: %d %s", body.Status, body.Body)
	}

	var result gitlabProject
	if err := json.Unmarshal(body.Body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	return &Repository{
		ID:                fmt.Sprintf("%d", *result.ID),
		Name:              result.Name,
		HttpUrl:           *result.HttpUrl,
		PathWithNamespace: *result.PathWithNamespace,
		Remote:            remote,
	}, nil
}

func (g *GitLab) LockUntilImport(repo *Repository, ping func(string)) error {
	urlPath := fmt.Sprintf("/api/v4/projects/%s", repo.ID)

	for {
		body, err := g.request(http.MethodGet, urlPath, nil, "")
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
			ping(importStatus)
			time.Sleep(5 * time.Second)
		}
	}
}

func (g *GitLab) SetOriginalUrl(repo *Repository, originUrl string) error {
	data := url.Values{}
	data.Add("key", "original_url")
	data.Add("value", originUrl)

	urlPath := fmt.Sprintf("/api/v4/projects/%s/variables", repo.ID)
	_, err := g.request(http.MethodPost, urlPath, bytes.NewBuffer([]byte(data.Encode())), "")
	return err
}

func (g *GitLab) GetProtectedBranches(repo *Repository) ([]string, error) {
	urlPath := fmt.Sprintf("/api/v4/projects/%s/protected_branches", repo.ID)
	body, err := g.request(http.MethodGet, urlPath, nil, "")
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

func (g *GitLab) UnprotectBranch(repo *Repository, branch string) error {
	encodedBranch := url.QueryEscape(branch)

	urlPath := fmt.Sprintf("/api/v4/projects/%s/protected_branches/%s", repo.ID, encodedBranch)
	_, err := g.request(http.MethodDelete, urlPath, nil, "")
	return err
}

func (g *GitLab) AddRemoteToRepo(repo *Repository) error {
	if repo.LocalRepository == nil {
		return fmt.Errorf("no local repository found for gitlabProject %d", repo.ID)
	}

	// Create url
	parsedURL, _ := url.Parse(repo.HttpUrl)
	parsedURL.User = url.UserPassword("oauth2", g.APIToken)

	// Add a new remote, named "gitlab"
	_, err := repo.LocalRepository.CreateRemote(&config.RemoteConfig{
		Name: "gitlab",
		URLs: []string{parsedURL.String()},
	})
	return err
}

func (g *GitLab) ChangeArchivedState(repo *Repository, isArchived bool) error {
	var path string
	if isArchived {
		path = fmt.Sprintf("/api/v4/projects/%s/archive", repo.ID)
	} else {
		path = fmt.Sprintf("/api/v4/projects/%s/unarchive", repo.ID)
	}

	res, err := g.request(http.MethodPost, path, nil, "")
	if err != nil {
		return err
	}

	if res.Status != http.StatusCreated {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return nil
}

func (g *GitLab) ChangeAvatar(repo *Repository, avatar *bytes.Buffer, ext string) error {
	return g.changeAvatar("projects", repo.ID, avatar, ext)
}

func (g *GitLab) ReleaseExists(repo *Repository, tagName string) (bool, error) {
	eTag := url.QueryEscape(tagName)
	urlPath := fmt.Sprintf("/api/v4/projects/%s/releases/%s", repo.ID, eTag)

	body, err := g.request(http.MethodGet, urlPath, nil, "")
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	return body.Status != http.StatusNotFound, nil
}

func (g *GitLab) CreateRelease(repo *Repository, release sources.SourceRelease) error {
	data := url.Values{}
	data.Add("name", release.Name)
	data.Add("tag_name", release.TagName)
	data.Add("description", release.Description)
	data.Add("released_at", release.CreatedAt)

	urlPath := fmt.Sprintf("/api/v4/projects/%s/releases", repo.ID)

	body, err := g.request(http.MethodPost, urlPath, bytes.NewBuffer([]byte(data.Encode())), "")
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if body.Status != http.StatusCreated {
		return fmt.Errorf("create release: status %d", body.Status)
	}

	return nil
}

func (g *GitLab) LinkAsset(repo *Repository, release sources.SourceRelease, assetName, assetUrl string) error {
	eTag := url.QueryEscape(release.TagName)

	data := url.Values{}
	data.Add("name", assetName)
	data.Add("url", assetUrl)

	urlPath := fmt.Sprintf("/api/v4/projects/%s/releases/%s/assets/links?%s", repo.ID, eTag, data.Encode())

	body, err := g.request(http.MethodPost, urlPath, nil, "")
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if body.Status != http.StatusCreated {
		return fmt.Errorf("create release: status %d", body.Status)
	}

	return nil
}

func (g *GitLab) GetWikiProject(repo *Repository, gConfig configuration.ConfigGroup, source sources.Source) *Repository {
	wikiUrl := source.GetWikiURL(gConfig.Username, repo.Remote.Name)
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
		LocalRepository: nil,
	}
}

func (g *GitLab) CreateWiki(repo *Repository, gConfig configuration.ConfigGroup) error {
	// Gitlab automatically creates the wiki on git push
	return nil
}

func (g *GitLab) UploadFile(buffer *bytes.Buffer, dstPath string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func isValidGitlabName(name string) bool {
	// Rule 1: can only include letters, digits, spaces, '_', '-', and '.'
	validChars := regexp.MustCompile(`^[a-zA-Z0-9_. -]+$`)
	if !validChars.MatchString(name) {
		return false
	}

	// Rule 2: should not start/end with '-'
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}

	// Rule 3: should not start/end with "." and end in ".git", or ".atom"
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".git") || strings.HasSuffix(name, ".atom") {
		return false
	}

	return true
}

func isReservedGitlabName(name string) bool {
	rn := []string{
		"badges",
		"blame",
		"blob",
		"builds",
		"commits",
		"create",
		"create_dir",
		"edit",
		"environments/folders",
		"files",
		"find_file",
		"gitlab-lfs/objects",
		"info/lfs/objects",
		"new",
		"preview",
		"raw",
		"refs",
		"tree",
		"update",
		"wikis",
	}

	for _, rn := range rn {
		if name == rn {
			return true
		}
	}
	return false
}

func (g *GitLab) changeAvatar(t string, id string, avatar *bytes.Buffer, ext string) error {
	buff := &bytes.Buffer{}
	writer := multipart.NewWriter(buff)

	// Create a form file field
	part, err := writer.CreateFormFile("avatar", fmt.Sprintf("avatar.%s", ext))
	if err != nil {
		return fmt.Errorf("error creating form file: %v", err)
	}

	// Write the buffer content to the form file
	if _, err := io.Copy(part, avatar); err != nil {
		return fmt.Errorf("error copying avatar buffer: %v", err)
	}

	// Close the writer to finalize the multipart form
	if err := writer.Close(); err != nil {
		return fmt.Errorf("error closing writer: %v", err)
	}

	urlPath := fmt.Sprintf("/api/v4/%s/%s", t, id)
	res, err := g.request(http.MethodPut, urlPath, buff, writer.FormDataContentType())
	if err != nil {
		return err
	}

	if res.Status != http.StatusOK {
		return fmt.Errorf("invalid response: %d %s", res.Status, res.Body)
	}

	return nil
}

func (g *GitLab) RetrieveParentGroup(remote sources.SourceRepository, parentGroupId int) (int, error) {
	groupId := parentGroupId

	path := remote.ParentGroupPath
	if path != nil && len(path) != 0 {
		for _, group := range path {
			id, err := g.upsertGroup(groupId, group)
			if err != nil {
				return -1, err
			}
			groupId = id
		}
	}

	return groupId, nil
}

func (g *GitLab) upsertGroup(parentGroupId int, group sources.SourceRepositoryGroup) (int, error) {
	// Check if the group already exists
	groupId, err := g.getGroupIdByName(parentGroupId, group)
	if err != nil {
		return -1, err // Group exists, return its ID
	}
	if groupId != -1 {
		return groupId, nil
	}

	// Group does not exist, create it
	newGroupId, err := g.createGroup(parentGroupId, group)
	if err != nil {
		return -1, err
	}
	return newGroupId, nil
}

func (g *GitLab) getGroupIdByName(parentGroupId int, group sources.SourceRepositoryGroup) (int, error) {
	data := url.Values{}
	data.Add("search", group.Name)
	data.Add("per_page", "100")

	urlPath := fmt.Sprintf("/api/v4/groups/%d/subgroups?%s", parentGroupId, data.Encode())
	body, err := g.request(http.MethodGet, urlPath, nil, "")
	if err != nil {
		return -1, err
	}

	if body.Status == http.StatusNotFound {
		return -1, nil
	}

	var groups []gitlabReqGroup
	err = json.Unmarshal(body.Body, &groups)
	if err != nil {
		return -1, err
	}

	for _, g := range groups {
		if g.Path == group.Name {
			return g.ID, nil // Return the ID of the existing group
		}
	}

	return -1, nil
}

func (g *GitLab) createGroup(parentGroupId int, group sources.SourceRepositoryGroup) (int, error) {
	data := url.Values{}
	data.Set("name", group.Name)
	data.Set("path", group.Path)
	data.Set("parent_id", fmt.Sprintf("%d", parentGroupId))

	urlPath := fmt.Sprintf("/api/v4/groups?%s", data.Encode())
	body, err := g.request(http.MethodPost, urlPath, nil, "")
	if err != nil {
		return -1, err
	}

	if body.Status != http.StatusCreated {
		return -1, fmt.Errorf("failed to create group %s: %s", group.Name, body.Body)
	}

	var newGroup gitlabReqGroup
	err = json.Unmarshal(body.Body, &newGroup)
	if err != nil {
		return -1, err
	}

	// Download & update avatar
	if newGroup.Avatar != nil {
		ext := utils.ExtractExtension(*newGroup.Avatar)

		buff, err := utils.DownloadAsset(*newGroup.Avatar)
		if err != nil {
			return -1, err
		}

		if err := g.changeAvatar("groups", fmt.Sprintf("%d", newGroup.ID), buff, ext); err != nil {
			return -1, err
		}
	}

	return newGroup.ID, nil
}

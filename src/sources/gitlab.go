package sources

import (
	"encoding/json"
	"fmt"
	"main/src/utils"
	"net/http"
	"net/url"
)

type Gitlab struct {
	URL   url.URL
	Token string

	client *http.Client
}

type GitlabGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`

	ParentID int
}

type GitlabRepository struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	HttpUrl     string  `json:"http_url_to_repo"`
	Visibility  string  `json:"visibility"`
}

type GitlabRelease struct {
	Name        string            `json:"name"`
	TagName     string            `json:"tag_name"`
	Description string            `json:"description"`
	CreatedAt   string            `json:"created_at"`
	Assets      GitlabAssetParent `json:"assets"`
}

type GitlabAssetParent struct {
	Count int           `json:"count"`
	Links []GitlabAsset `json:"links"`
}

type GitlabAsset struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	BrowserDownloadUrl string `json:"direct_asset_url"`
}

type GitlabMetadata struct {
	ReposNextPage        int
	CurrentSubgroupIndex int
	BaseGroupID          int
	CurrentGroup         *GitlabGroup
	Subgroups            []GitlabGroup
	VisitedGroupIDs      map[int]bool
}

func NewGitlab(token string) *Gitlab {
	u, _ := url.Parse("https://gitlab.com")

	return &Gitlab{
		URL:    *u,
		Token:  token,
		client: &http.Client{},
	}
}

func (g *Gitlab) Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error) {
	if prev == nil {
		groupId, err := g.fetchGroupId(username)
		if err != nil {
			return nil, err
		}

		prev = &PaginationResponse{
			Metadata: &GitlabMetadata{
				ReposNextPage:        1,
				BaseGroupID:          groupId,
				CurrentGroup:         &GitlabGroup{ID: groupId, Name: "", ParentID: -1},
				CurrentSubgroupIndex: 0,
				VisitedGroupIDs:      make(map[int]bool),
			},
		}
	}

	metadata := prev.Metadata.(*GitlabMetadata)

	// Fetch all subgroups
	if len(metadata.Subgroups) == 0 {
		// Fetch all subgroups
		if err := g.fetchAllSubgroups(metadata.CurrentGroup.ID, metadata); err != nil {
			return nil, err
		}
	}

	// Fetch repositories for the current group
	repos, err := g.fetchRepositories(metadata.CurrentGroup, metadata.ReposNextPage)
	if err != nil {
		return nil, err
	}

	repos = populateParentPath(repos, metadata)

	if len(repos) > 0 {
		metadata.ReposNextPage += 1
		return &PaginationResponse{
			Repositories: repos,
			Metadata:     metadata,
		}, nil
	}

	// Mark subgroup as visited
	if metadata.CurrentGroup.ID != metadata.BaseGroupID {
		metadata.VisitedGroupIDs[metadata.CurrentSubgroupIndex] = true
		metadata.CurrentSubgroupIndex++
	}

	// Finished repos in the current group, move to subgroups
	for metadata.CurrentSubgroupIndex < len(metadata.Subgroups) {
		subgroup := metadata.Subgroups[metadata.CurrentSubgroupIndex]

		// Ensure we don't repeat requests for already visited groups.
		if metadata.VisitedGroupIDs[subgroup.ID] {
			metadata.VisitedGroupIDs[metadata.CurrentSubgroupIndex] = true
			metadata.CurrentSubgroupIndex++
			continue
		}

		// Append subgroup id to the path
		metadata.CurrentGroup = &subgroup
		metadata.ReposNextPage = 1
		return g.Paginate(username, prev)
	}

	return &PaginationResponse{
		Repositories: make([]SourceRepository, 0),
		Metadata:     metadata,
	}, nil
}

func (g *Gitlab) fetchAllSubgroups(parentId int, metadata *GitlabMetadata) error {
	// Initialize a stack with the initial parent group ID
	stack := []int{parentId}

	// Loop until there are no more subgroups to process
	for len(stack) > 0 {
		// Get the last group ID from the stack
		currentParentId := stack[len(stack)-1]
		stack = stack[:len(stack)-1] // Remove the last element from the stack

		pageNum := 1
	inner:
		for {
			// Fetch subgroups for the current parent ID
			subgroups, err := g.fetchSubgroups(currentParentId, pageNum)
			if err != nil {
				return err
			}

			if len(subgroups) == 0 {
				break inner
			}

			// Append the fetched subgroups to the metadata
			metadata.Subgroups = append(metadata.Subgroups, subgroups...)

			// Add the IDs of the fetched subgroups to the stack for further processing
			for _, subgroup := range subgroups {
				stack = append(stack, subgroup.ID)
			}

			pageNum += 1
		}
	}

	return nil
}

func (g *Gitlab) fetchRepositories(parentGroup *GitlabGroup, pageNumber int) ([]SourceRepository, error) {
	urlPath := fmt.Sprintf("%s/api/v4/groups/%d/projects?per_page=100&page=%d", g.URL.String(), parentGroup.ID, pageNumber)
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, err
	}

	if len(g.Token) > 0 {
		req.Header.Set("Private-Token", g.Token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch repositories: %s", resp.Status)
	}

	var fetchedRepos []GitlabRepository
	if err := json.NewDecoder(resp.Body).Decode(&fetchedRepos); err != nil {
		return nil, err
	}

	repos := make([]SourceRepository, len(fetchedRepos))
	for i, repo := range fetchedRepos {
		repos[i] = SourceRepository{
			ID:          repo.ID,
			Name:        repo.Name,
			URL:         repo.HttpUrl,
			Description: repo.Description,
			Private:     repo.Visibility == "private",
		}
	}

	return repos, nil
}

func (g *Gitlab) fetchSubgroups(parentGroupID, pageNumber int) ([]GitlabGroup, error) {
	urlPath := fmt.Sprintf("%s/api/v4/groups/%d/subgroups?per_page=100&page=%d", g.URL.String(), parentGroupID, pageNumber)
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, err
	}

	if len(g.Token) > 0 {
		req.Header.Set("Private-Token", g.Token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch subgroups: %s", resp.Status)
	}

	var subgroups []GitlabGroup
	if err := json.NewDecoder(resp.Body).Decode(&subgroups); err != nil {
		return nil, err
	}

	// Set the ParentID for each subgroup
	for i := range subgroups {
		subgroups[i].ParentID = parentGroupID
	}

	return subgroups, nil
}

func (g *Gitlab) fetchGroupId(username string) (int, error) {
	urlPath := fmt.Sprintf("%s/api/v4/groups/%s", g.URL.String(), username)
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return 0, err
	}

	if len(g.Token) > 0 {
		req.Header.Set("Private-Token", g.Token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to fetch group ID: %s", resp.Status)
	}

	var group GitlabGroup
	if err := json.NewDecoder(resp.Body).Decode(&group); err != nil {
		return 0, err
	}

	return group.ID, nil
}

func populateParentPath(repos []SourceRepository, metadata *GitlabMetadata) []SourceRepository {
	// For top level repos, do not populate parent id
	if metadata.CurrentGroup.ID == metadata.BaseGroupID {
		return repos
	}

	// Create a map to easily find groups by their ID
	mGroups := make(map[int]GitlabGroup)
	for _, group := range metadata.Subgroups {
		mGroups[group.ID] = group
	}

	// Start from the current group and traverse up to the base group
	path := make([]string, 0)
	curr := metadata.CurrentGroup
	for curr != nil && curr.ID != metadata.BaseGroupID {
		path = append([]string{curr.Name}, path...) // Prepend the current group's name

		// Move to the parent group
		if parentGroup, exists := mGroups[curr.ParentID]; exists {
			curr = &parentGroup
		} else {
			break // No more parent group found
		}
	}

	for i := range repos {
		repos[i].ParentGroupPath = path
	}

	return repos
}

func (g *Gitlab) GetWikiURL(username, repoName string) string {
	return fmt.Sprintf("https://%s:@%s/%s/%s.wiki.git", g.Token, g.URL.Host, username, repoName)
}

func (g *Gitlab) FetchReleases(username string, repo SourceRepository) ([]SourceRelease, error) {
	urlPath := fmt.Sprintf("%s/api/v4/projects/%d/releases?per_page=10", g.URL.String(), repo.ID)
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, err
	}

	if len(g.Token) > 0 {
		req.Header.Set("Private-Token", g.Token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch repositories: %s", resp.Status)
	}

	var fetchedReleases []GitlabRelease
	if err := json.NewDecoder(resp.Body).Decode(&fetchedReleases); err != nil {
		return nil, err
	}

	releases := make([]SourceRelease, len(fetchedReleases))
	for i, rel := range fetchedReleases {
		assets := make([]SourceAsset, rel.Assets.Count)
		for i, asset := range rel.Assets.Links {
			assets[i] = SourceAsset{
				Name: asset.Name,
				URL:  asset.BrowserDownloadUrl,
			}
		}

		releases[i] = SourceRelease{
			Name:        rel.Name,
			TagName:     rel.TagName,
			Description: rel.Description,
			CreatedAt:   rel.CreatedAt,
			Assets:      assets,
		}
	}

	return utils.Reverse(releases), nil
}

func (g *Gitlab) AddTokenToCloneUrl(u string) string {
	parsedURL, _ := url.Parse(u)
	parsedURL.User = url.User(g.Token)
	return parsedURL.String()
}

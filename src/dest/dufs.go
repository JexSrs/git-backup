package dest

import (
	"bytes"
	"fmt"
	"io"
	"main/src/configuration"
	"main/src/sources"
	"net/http"
	"net/url"
)

type Dufs struct {
	ID       string
	URL      url.URL
	RootPath string

	client *http.Client
}

func NewDufs(config configuration.ConfigDestination) *Dufs {
	dufsUrl, _ := url.Parse(config.URL)

	return &Dufs{
		ID:       config.ID,
		URL:      *dufsUrl,
		RootPath: "/",
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

func (g *Dufs) GetIdentification() DestinationID {
	return DestinationID{
		ID: g.ID,
		Repository: DestinationIDRepository{
			Avatar:   true,
			Wiki:     true,
			Releases: true,
		},
	}
}

func (g *Dufs) RetrieveExistingRepo(gConfig configuration.ConfigGroup, remote sources.SourceRepository) (*Repository, error) {
	return nil, fmt.Errorf("not implemented")
}

func (g *Dufs) ImportRepository(gConfig configuration.ConfigGroup, remote sources.SourceRepository, source sources.Source) (*Repository, error) {
	return nil, fmt.Errorf("not implemented")
}

func (g *Dufs) LockUntilImport(repo *Repository, ping func(string)) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) SetOriginalUrl(repo *Repository, originUrl string) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) GetProtectedBranches(repo *Repository) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (g *Dufs) UnprotectBranch(repo *Repository, branch string) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) CloneFromSource(repo *Repository, source sources.Source) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) AddRemoteToRepo(repo *Repository) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) ChangeArchivedState(repo *Repository, isArchived bool) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) GetLocalBranches(repo *Repository) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (g *Dufs) PushLocalBranch(repo *Repository, branch string) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) PushAllTags(repo *Repository) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) ChangeAvatar(repo *Repository, avatar *bytes.Buffer, ext string) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) ReleaseExists(repo *Repository, tagName string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (g *Dufs) CreateRelease(repo *Repository, release sources.SourceRelease) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) LinkAsset(repo *Repository, release sources.SourceRelease, assetName, assetUrl string) error {
	return fmt.Errorf("not implemented")
}

func (g *Dufs) GetWikiProject(repo *Repository, gConfig configuration.ConfigGroup, source sources.Source) *Repository {
	return nil
}

func (g *Dufs) CreateWiki(repo *Repository, gConfig configuration.ConfigGroup) error {
	return fmt.Errorf("not implemented")
}

func (d *Dufs) UploadFile(buffer *bytes.Buffer, dstPath string) (string, error) {
	request, err := http.NewRequest(http.MethodPut, d.URL.JoinPath(d.RootPath, dstPath).String(), buffer)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	response, err := d.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("error performing request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("received non-success status code: %d, body: %s", response.StatusCode, responseBody)
	}

	return d.URL.JoinPath(d.RootPath).JoinPath(dstPath).String(), nil
}

package main

import (
	"bytes"
	"fmt"
	"main/src/configuration"
	"main/src/dest"
	"main/src/sources"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/config"
)

func ptr[T any](v T) *T { return &v }

// fakeSource serves pre-defined pages of repositories and canned releases.
type fakeSource struct {
	pages         [][]sources.SourceRepository
	failOnPage    int // 1-based page whose fetch fails; 0 never fails
	failPageTimes int // how many fetches of failOnPage fail before it succeeds; 0 means every fetch
	releases      map[string][]sources.SourceRelease
	releasesErr   error
	wikiURLs      map[string]string // repository name → wiki clone url
}

func (f *fakeSource) Paginate(_ string, _ configuration.ConfigGroup, prev *sources.PaginationResponse) (*sources.PaginationResponse, error) {
	page := 0
	if prev != nil {
		page = prev.NextPage
	}
	if f.failOnPage == page+1 {
		if f.failPageTimes > 0 {
			f.failPageTimes--
			if f.failPageTimes == 0 {
				f.failOnPage = 0
			}
		}
		return nil, fmt.Errorf("page %d unavailable", page+1)
	}
	if page >= len(f.pages) {
		return &sources.PaginationResponse{NextPage: page + 1}, nil
	}
	return &sources.PaginationResponse{Repositories: f.pages[page], NextPage: page + 1}, nil
}

func (f *fakeSource) GetWikiURL(_ string, repoName string) string {
	return f.wikiURLs[repoName]
}

func (f *fakeSource) FetchReleases(_ string, repo sources.SourceRepository) ([]sources.SourceRelease, error) {
	if f.releasesErr != nil {
		return nil, f.releasesErr
	}
	return f.releases[repo.Name], nil
}

func (f *fakeSource) AddTokenToCloneUrl(u string) string { return u }

func (f *fakeSource) FetchUsernamePassword() (string, string) { return "", "" }

// fakeDest records every call the sync makes, in order, and answers from canned state.
type fakeDest struct {
	id   string
	cfg  configuration.ConfigDestination
	caps dest.DestinationIDRepository

	existing  map[string]*dest.Repository // remote name → repository returned by RetrieveExistingRepo
	protected []string
	releases  map[string]bool             // tags that already exist in the destination
	wikis     map[string]*dest.Repository // repository name → wiki returned by GetWikiProject
	remotes   map[string]string           // local repository name → bare repository AddRemoteToRepo points at
	uploadURL string
	errs      map[string]error // method name → error to return
	onceErrs  map[string]error // method name → error to return on its next call only

	calls   []string
	avatars map[string][]byte // extension → uploaded bytes
	uploads map[string][]byte // storage path → uploaded bytes
}

func newFakeDest(id string) *fakeDest {
	return &fakeDest{
		id:        id,
		cfg:       configuration.ConfigDestination{ID: id, URL: "http://" + id + ".local", Timeout: time.Minute},
		caps:      dest.DestinationIDRepository{Avatar: true, Wiki: true, Releases: true},
		existing:  map[string]*dest.Repository{},
		releases:  map[string]bool{},
		wikis:     map[string]*dest.Repository{},
		remotes:   map[string]string{},
		uploadURL: "https://" + id + ".local/stored",
		errs:      map[string]error{},
		onceErrs:  map[string]error{},
		avatars:   map[string][]byte{},
		uploads:   map[string][]byte{},
	}
}

func (f *fakeDest) record(call string) { f.calls = append(f.calls, call) }

func (f *fakeDest) fail(method string) error {
	if err, ok := f.onceErrs[method]; ok {
		delete(f.onceErrs, method)
		return err
	}
	return f.errs[method]
}

// callsWithPrefix returns the recorded calls starting with prefix, with the prefix removed.
func (f *fakeDest) callsWithPrefix(prefix string) []string {
	out := make([]string, 0)
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, strings.TrimPrefix(c, prefix))
		}
	}
	return out
}

func (f *fakeDest) GetIdentification() dest.DestinationID {
	return dest.DestinationID{ID: f.id, Repository: f.caps, Config: &f.cfg}
}

func (f *fakeDest) EnsureOwner(g configuration.ConfigGroup) (bool, error) {
	f.record("EnsureOwner:" + g.GiteaUsername)
	return false, f.fail("EnsureOwner")
}

func (f *fakeDest) RetrieveExistingRepo(_ configuration.ConfigGroup, remote sources.SourceRepository) (*dest.Repository, error) {
	f.record("RetrieveExistingRepo:" + remote.Name)
	return f.existing[remote.Name], f.fail("RetrieveExistingRepo")
}

func (f *fakeDest) ImportRepository(g configuration.ConfigGroup, _ configuration.ConfigRepo, remote sources.SourceRepository, _ sources.Source) (*dest.Repository, error) {
	f.record("ImportRepository:" + remote.Name)
	if err := f.fail("ImportRepository"); err != nil {
		return nil, err
	}
	return &dest.Repository{ID: "id-" + remote.Name, Name: remote.Name, Remote: remote, ConfigGroup: g}, nil
}

func (f *fakeDest) LockUntilImport(repo *dest.Repository, _ func(string)) error {
	f.record("LockUntilImport:" + repo.Name)
	return f.fail("LockUntilImport")
}

func (f *fakeDest) SetOriginalUrl(_ *dest.Repository, url string) error {
	f.record("SetOriginalUrl:" + url)
	return f.fail("SetOriginalUrl")
}

func (f *fakeDest) GetProtectedBranches(_ *dest.Repository) ([]string, error) {
	f.record("GetProtectedBranches")
	return f.protected, f.fail("GetProtectedBranches")
}

func (f *fakeDest) UnprotectBranch(_ *dest.Repository, branch string) error {
	f.record("UnprotectBranch:" + branch)
	return f.fail("UnprotectBranch")
}

func (f *fakeDest) AddRemoteToRepo(repo *dest.Repository) error {
	f.record("AddRemoteToRepo:" + repo.Name)
	if err := f.fail("AddRemoteToRepo"); err != nil {
		return err
	}
	path, ok := f.remotes[repo.Name]
	if !ok {
		return fmt.Errorf("fakeDest: no remote configured for %s", repo.Name)
	}
	_, err := repo.LocalRepository.CreateRemote(&config.RemoteConfig{Name: f.id, URLs: []string{path}})
	return err
}

func (f *fakeDest) ChangeArchivedState(_ *dest.Repository, archived bool) error {
	f.record(fmt.Sprintf("ChangeArchivedState:%v", archived))
	return f.fail("ChangeArchivedState")
}

func (f *fakeDest) ChangeAvatar(_ *dest.Repository, avatar *bytes.Buffer, ext string) error {
	f.record("ChangeAvatar:" + ext)
	f.avatars[ext] = avatar.Bytes()
	return f.fail("ChangeAvatar")
}

func (f *fakeDest) ReleaseExists(_ *dest.Repository, tag string) (bool, error) {
	f.record("ReleaseExists:" + tag)
	return f.releases[tag], f.fail("ReleaseExists")
}

func (f *fakeDest) CreateRelease(_ *dest.Repository, release sources.SourceRelease) error {
	f.record("CreateRelease:" + release.TagName)
	return f.fail("CreateRelease")
}

func (f *fakeDest) LinkAsset(_ *dest.Repository, release sources.SourceRelease, name, url string) error {
	f.record("LinkAsset:" + release.TagName + ":" + name + ":" + url)
	return f.fail("LinkAsset")
}

func (f *fakeDest) GetWikiProject(repo *dest.Repository, _ configuration.ConfigGroup, _ sources.Source) *dest.Repository {
	f.record("GetWikiProject:" + repo.Name)
	return f.wikis[repo.Name]
}

func (f *fakeDest) CreateWiki(repo *dest.Repository, _ configuration.ConfigGroup) error {
	f.record("CreateWiki:" + repo.Name)
	return f.fail("CreateWiki")
}

func (f *fakeDest) UploadFile(buffer *bytes.Buffer, path string) (string, error) {
	f.record("UploadFile:" + path)
	f.uploads[path] = buffer.Bytes()
	if err := f.fail("UploadFile"); err != nil {
		return "", err
	}
	return f.uploadURL + path, nil
}

// repoConfig is a fully populated repository configuration with avatar, wiki and releases disabled.
func repoConfig(gitID, storageID string) configuration.ConfigRepo {
	return configuration.ConfigRepo{
		FetchAvatar: ptr(false),
		Destination: ptr(gitID),
		LFS:         ptr(true),
		Wiki:        configuration.ConfigRepoWiki{Exclude: ptr(true)},
		Releases: configuration.ConfigRepoReleases{
			Exclude: ptr(true),
			Assets: configuration.ConfigRepoAssets{
				Destination: ptr(storageID),
				Exclude:     ptr(false),
				MaxSize:     ptr("1GB"),
			},
		},
	}
}

func groupConfig() configuration.ConfigGroup {
	return configuration.ConfigGroup{
		Source:        "github",
		Username:      "octo",
		GiteaUsername: "octo",
		Skip:          ptr(0),
		Filter:        configuration.ConfigFilter{OnlyNew: ptr(false)},
		Config:        repoConfig("gitea", "dufs"),
	}
}

func remoteNamed(name string) sources.SourceRepository {
	return sources.SourceRepository{Name: name, URL: "https://github.com/octo/" + name + ".git"}
}

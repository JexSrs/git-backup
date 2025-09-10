package dest

import (
	"bytes"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"main/src/configuration"
	"main/src/sources"
	"os"
	"path/filepath"
	"strings"
)

type Repository struct {
	ID                string
	Name              string
	HttpUrl           string
	PathWithNamespace string
	FinishedMiration  bool

	Remote      sources.SourceRepository
	ConfigGroup configuration.ConfigGroup

	LocalRepository *git.Repository

	OverrideRemoteBranch string
}

func (r *Repository) CloneFromSource(source sources.Source) error {
	path := filepath.Join("/tmp/git-backup/", r.Name)
	os.RemoveAll(path)

	gr, err := git.PlainClone(path, false, &git.CloneOptions{
		URL: r.Remote.URL,
		Auth: func() *http.BasicAuth {
			username, password := source.FetchUsernamePassword()
			if len(username) == 0 && len(password) == 0 {
				return nil
			}
			return &http.BasicAuth{
				Username: username,
				Password: password,
			}
		}(),
	})

	if err != nil {
		return err
	}

	r.LocalRepository = gr
	return nil
}

func (r *Repository) GetLocalBranches() ([]string, error) {
	if r.LocalRepository == nil {
		return nil, fmt.Errorf("no repository found for %s", r.ID)
	}

	branches, err := r.LocalRepository.Branches()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	branches.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()

		// For Gitlab sources
		if strings.HasPrefix(name, "refs/heads/") {
			name = strings.TrimPrefix(name, "refs/heads/")
		}

		names = append(names, name)
		return nil
	})

	return names, nil
}

func (r *Repository) PushLocalBranch(branch, remoteID string) error {
	if r.LocalRepository == nil {
		return fmt.Errorf("no repository found for %s", r.ID)
	}

	remoteBranch := branch
	if r.OverrideRemoteBranch != "" {
		remoteBranch = r.OverrideRemoteBranch
	}

	pushOptions := &git.PushOptions{
		RemoteName: remoteID,
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/" + branch + ":refs/heads/" + remoteBranch),
		},
		Force: true,
	}

	// Perform the push
	if err := r.LocalRepository.Push(pushOptions); err != nil && err.Error() != "already up-to-date" {
		return err
	}

	return nil
}

func (r *Repository) PushAllTags(remoteID string) error {
	if r.LocalRepository == nil {
		return fmt.Errorf("no repository found for %s", r.ID)
	}

	pushOptions := &git.PushOptions{
		RemoteName: remoteID,
		RefSpecs:   []config.RefSpec{"refs/tags/*:refs/tags/*"},
		Force:      true,
	}

	// Perform the push
	if err := r.LocalRepository.Push(pushOptions); err != nil && err.Error() != "already up-to-date" {
		return err
	}

	return nil
}

func (r *Repository) Prune() error {
	path := filepath.Join("/tmp/git-backup/", r.Name)
	return os.RemoveAll(path)
}

type DestinationIDRepository struct {
	Avatar   bool
	Wiki     bool
	Releases bool
}

type DestinationID struct {
	ID         string
	Repository DestinationIDRepository
}

type Destination interface {
	GetIdentification() DestinationID

	RetrieveExistingRepo(gConfig configuration.ConfigGroup, remote sources.SourceRepository) (*Repository, error)
	ImportRepository(gConfig configuration.ConfigGroup, config configuration.ConfigRepo, remote sources.SourceRepository, source sources.Source) (*Repository, error)
	LockUntilImport(repo *Repository, ping func(status string)) error
	SetOriginalUrl(repo *Repository, url string) error

	GetProtectedBranches(repo *Repository) ([]string, error)
	UnprotectBranch(repo *Repository, branch string) error

	AddRemoteToRepo(repo *Repository) error
	ChangeArchivedState(repo *Repository, isArchived bool) error

	ChangeAvatar(repo *Repository, avatar *bytes.Buffer, ext string) error
	ReleaseExists(repo *Repository, tagName string) (bool, error)
	CreateRelease(repo *Repository, release sources.SourceRelease) error
	LinkAsset(repo *Repository, release sources.SourceRelease, assetName, assetUrl string) error

	GetWikiProject(repo *Repository, gConfig configuration.ConfigGroup, source sources.Source) *Repository
	CreateWiki(repo *Repository, gConfig configuration.ConfigGroup) error

	UploadFile(buffer *bytes.Buffer, dstPath string) (string, error)
}

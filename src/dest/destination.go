package dest

import (
	"bytes"
	"fmt"
	"main/src/configuration"
	"main/src/sources"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
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

	DstIgnoreTLS bool
	DstCABundle  []byte
}

// useDestinationTransport records the destination's TLS settings on the repository and installs an
// HTTPS client honouring the destination's timeout and certificates for every go-git operation.
func (r *Repository) useDestinationTransport(dstCfg configuration.ConfigDestination) error {
	caBundle, err := dstCfg.LoadCABundle()
	if err != nil {
		return err
	}
	r.DstIgnoreTLS = dstCfg.IgnoreTLS
	r.DstCABundle = caBundle

	customClient := &http.Client{
		Timeout: dstCfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: dstCfg.TLSConfig(),
		},
	}
	client.InstallProtocol("https", githttp.NewClient(customClient))
	return nil
}

// sourceAuth turns the source credentials into go-git basic auth, or nil when there are none.
func sourceAuth(source sources.Source) transport.AuthMethod {
	username, password := source.FetchUsernamePassword()
	if len(username) == 0 && len(password) == 0 {
		return nil
	}
	return &githttp.BasicAuth{Username: username, Password: password}
}

func (r *Repository) CloneFromSource(source sources.Source, dstCfg configuration.ConfigDestination) error {
	path := filepath.Join("/tmp/git-backup/", r.Name)
	os.RemoveAll(path)

	if err := r.useDestinationTransport(dstCfg); err != nil {
		return err
	}

	gr, err := git.PlainClone(path, false, &git.CloneOptions{
		URL:             r.Remote.URL,
		InsecureSkipTLS: r.DstIgnoreTLS,
		CABundle:        r.DstCABundle,
		Auth:            sourceAuth(source),
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
		if after, ok := strings.CutPrefix(name, "refs/heads/"); ok {
			name = after
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
		Force:           true,
		InsecureSkipTLS: r.DstIgnoreTLS,
		CABundle:        r.DstCABundle,
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
		RemoteName:      remoteID,
		RefSpecs:        []config.RefSpec{"refs/tags/*:refs/tags/*"},
		Force:           true,
		InsecureSkipTLS: r.DstIgnoreTLS,
		CABundle:        r.DstCABundle,
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
	Config     *configuration.ConfigDestination
}

type Destination interface {
	GetIdentification() DestinationID

	// EnsureOwner makes sure the namespace that will own the repositories exists, creating it when the
	// destination is configured to do so. It reports whether it created the owner.
	EnsureOwner(gConfig configuration.ConfigGroup) (bool, error)

	RetrieveExistingRepo(gConfig configuration.ConfigGroup, remote sources.SourceRepository) (*Repository, error)
	ImportRepository(gConfig configuration.ConfigGroup, config configuration.ConfigRepo, remote sources.SourceRepository, source sources.Source) (*Repository, error)
	LockUntilImport(repo *Repository, ping func(status string)) error
	SetOriginalUrl(repo *Repository, url string) error

	GetProtectedBranches(repo *Repository) ([]string, error)
	UnprotectBranch(repo *Repository, branch string) error

	// RemoteURL returns the url, credentials included, through which the destination repository is
	// listed and pushed to; empty when the destination cannot host repositories.
	RemoteURL(repo *Repository) string
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

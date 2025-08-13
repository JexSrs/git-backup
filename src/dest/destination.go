package dest

import (
	"bytes"
	"github.com/go-git/go-git/v5"
	"main/src/configuration"
	"main/src/sources"
	"os"
	"path/filepath"
)

type Repository struct {
	ID                string
	Name              string
	HttpUrl           string
	PathWithNamespace string

	Remote sources.SourceRepository

	LocalRepository *git.Repository
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
	ImportRepository(gConfig configuration.ConfigGroup, remote sources.SourceRepository, source sources.Source) (*Repository, error)
	LockUntilImport(repo *Repository) error
	SetOriginalUrl(repo *Repository, url string) error

	GetProtectedBranches(repo *Repository) ([]string, error)
	UnprotectBranch(repo *Repository, branch string) error

	CloneFromSource(repo *Repository, source sources.Source) error
	AddRemoteToRepo(repo *Repository) error
	ChangeArchivedState(repo *Repository, isArchived bool) error
	GetLocalBranches(repo *Repository) ([]string, error)
	PushLocalBranch(repo *Repository, branch string) error
	PushAllTags(repo *Repository) error

	ChangeAvatar(repo *Repository, avatar *bytes.Buffer, ext string) error
	ReleaseExists(repo *Repository, tagName string) (bool, error)
	CreateRelease(repo *Repository, release sources.SourceRelease) error
	LinkAsset(repo *Repository, tagName, assetName, assetUrl string) error

	GetWikiProject(repo *Repository, gConfig configuration.ConfigGroup, source sources.Source) *Repository

	UploadFile(buffer *bytes.Buffer, dstPath string) (string, error)
}

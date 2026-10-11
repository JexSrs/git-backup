package sources

import (
	"errors"
	"main/src/configuration"
)

// ErrStatsUnavailable is wrapped by FetchStats when a source cannot provide statistics at all, for
// example GitHub without a token or a source that has no issues, pull requests or releases.
var ErrStatsUnavailable = errors.New("repository statistics unavailable")

type Source interface {
	Paginate(username string, groupCfg configuration.ConfigGroup, prev *PaginationResponse) (*PaginationResponse, error)
	GetWikiURL(username, repoName string) string
	FetchReleases(username string, repo SourceRepository) ([]SourceRelease, error)
	AddTokenToCloneUrl(url string) string
	FetchUsernamePassword() (string, string)

	// FetchStats returns repo.Stats completed with the statistics that needs asks for and that the
	// listing did not already provide. Statistics the source cannot determine stay nil.
	FetchStats(username string, repo SourceRepository, needs StatsNeeds) (RepositoryStats, error)
}

type PaginationResponse struct {
	Repositories []SourceRepository

	NextPage   int
	NextCursor *string

	Metadata any
}

type SourceRepository struct {
	Name        string
	URL         string
	Description *string
	Avatar      *string

	Private  bool
	Archived bool
	Empty    bool
	Forked   bool

	ParentGroupPath []SourceRepositoryGroup

	// Stats holds the counts the filters may need; the listing fills what it can, FetchStats the rest.
	Stats RepositoryStats

	// Used by GitLab source
	ID int
}

// RepositoryStats are per-repository counts. A nil count is unknown: not requested, or not provided
// by the source.
type RepositoryStats struct {
	Releases     *int
	Commits      *int // commits reachable from the default branch
	Issues       StateCounts
	PullRequests StateCounts // Closed includes merged
}

type StateCounts struct {
	All    *int
	Open   *int
	Closed *int
}

// StatsNeeds says which statistics the configured filters require.
type StatsNeeds struct {
	Releases     bool
	Commits      bool
	Issues       StateNeeds
	PullRequests StateNeeds
}

type StateNeeds struct {
	All    bool
	Open   bool
	Closed bool
}

func (n StateNeeds) Any() bool { return n.All || n.Open || n.Closed }

func (n StatsNeeds) Any() bool {
	return n.Releases || n.Commits || n.Issues.Any() || n.PullRequests.Any()
}

type SourceRepositoryGroup struct {
	Path   string
	Name   string
	Avatar *string
}

type SourceRelease struct {
	Name        string
	TagName     string
	Description string
	CreatedAt   string
	Assets      []SourceAsset
}

type SourceAsset struct {
	Name string
	URL  string
}

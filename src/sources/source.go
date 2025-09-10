package sources

type Source interface {
	Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error)
	GetWikiURL(username, repoName string) string
	FetchReleases(username string, repo SourceRepository) ([]SourceRelease, error)
	AddTokenToCloneUrl(url string) string
	FetchUsernamePassword() (string, string)
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

	// Used by GitLab source
	ID int
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

package sources

const (
	GitHubID      = "github"
	HuggingFaceID = "huggingface"
	GitlabID      = "gitlab"
)

type Source interface {
	Paginate(username string, prev *PaginationResponse) (*PaginationResponse, error)
	GetWikiURL(username, repoName string) string
	FetchReleases(username, repoName string) ([]SourceRelease, error)
	AddTokenToCloneUrl(url string) string
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
	Private     bool
}

type SourceRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Description string        `json:"body"`
	CreatedAt   string        `json:"created_at"`
	Assets      []SourceAsset `json:"assets"`
}

type SourceAsset struct {
	Name               string `json:"name"`
	BrowserDownloadUrl string `json:"browser_download_url"`
}

package sources

type Source interface {
	GetID() string
	Paginate(username string, page int) ([]SourceRepository, error)
	GetWikiURL(username, repoName string) string
	FetchReleases(username, repoName string) ([]SourceRelease, error)
	//DownloadAsset() error
}

type SourceRepository struct {
	Name        string  `json:"name"`
	URL         string  `json:"clone_url"`
	Description *string `json:"description"`
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

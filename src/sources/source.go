package sources

type Source interface {
	Paginate(username string, page int) ([]SourceRepository, error)
	GetWikiURL(username, repoName string) string
	//DownloadAsset() error
}

type SourceRepository struct {
	Name        string
	URL         string
	Description *string
}

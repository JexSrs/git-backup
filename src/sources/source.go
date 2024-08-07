package sources

type Source interface {
	Paginate(username string, page int) ([]SourceRepository, error)
	Clone(url, dst string) error
	//DownloadAsset() error
}

type SourceRepository struct {
	Name        string
	URL         string
	Description string
}

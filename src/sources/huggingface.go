package sources

import "main/src/utils"

type HuggingFace struct {
	Token string
}

func NewHuggingFace(token string) *HuggingFace {
	return &HuggingFace{Token: token}
}

func (g *HuggingFace) Paginate(username string, page int) ([]SourceRepository, error) {
	return []SourceRepository{}, nil
}

func (g *HuggingFace) GetWikiURL(username, repoName string) string {
	return ""
}

func (g *HuggingFace) FetchReleases(username, repoName string) ([]SourceRelease, error) {
	return utils.Reverse([]SourceRelease{}), nil
}

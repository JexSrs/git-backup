package sources

import (
	"main/src/configuration"
	"main/src/utils"
	"net/url"
)

type Git struct {
	Username string
	Token    string
}

func NewGit(config configuration.ConfigSource) *Git {
	return &Git{
		Username: config.Auth.Username,
		Token:    config.Auth.Token,
	}
}

func (g *Git) Paginate(username string, groupCfg configuration.ConfigGroup, prev *PaginationResponse) (*PaginationResponse, error) {
	repos := make([]SourceRepository, 0)
	if prev == nil {
		for _, repo := range groupCfg.Repositories {
			name := repo.Name
			if len(name) == 0 {
				name = utils.ExtractNameFromGitURL(repo.URL)
			}

			repos = append(repos, SourceRepository{
				Name:        name,
				Description: nil,
				URL:         repo.URL,
				Private:     false,
				Archived:    false,
				Forked:      false,
				Empty:       false,
			})
		}
		prev = &PaginationResponse{NextPage: 1}
	}

	return &PaginationResponse{
		Repositories: repos,
		NextPage:     prev.NextPage + 1,
	}, nil
}

func (g *Git) GetWikiURL(username, repoName string) string {
	return ""
}

func (g *Git) FetchReleases(username string, repo SourceRepository) ([]SourceRelease, error) {
	return make([]SourceRelease, 0), nil
}

func (g *Git) AddTokenToCloneUrl(u string) string {
	parsedURL, _ := url.Parse(u)

	if len(g.Username) != 0 && len(g.Token) != 0 {
		parsedURL.User = url.UserPassword(g.Username, g.Token)
	} else if len(g.Username) != 0 {
		parsedURL.User = url.User(g.Username)
	} else if len(g.Token) != 0 {
		parsedURL.User = url.UserPassword("", g.Token)
	}

	return parsedURL.String()
}

func (g *Git) FetchUsernamePassword() (string, string) {
	return g.Username, g.Token
}

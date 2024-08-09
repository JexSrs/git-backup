package main

import (
	"fmt"
	"main/src/utils"
)

type Configuration struct {
	Gitlab  ConfigGitLab  `json:"gitlab"`
	Dufs    ConfigDufs    `json:"dufs"`
	Config  ConfigRepo    `json:"config"`
	Sources ConfigSources `json:"sources"`
	Groups  []ConfigGroup `json:"groups"`
}

type ConfigGitLab struct {
	URL   *string `json:"url"`
	Token *string `json:"token"`
}

type ConfigDufs struct {
	URL *string `json:"url"`
}

// Sources configuration

type ConfigSources struct {
	GitHub      *ConfigSourcesGitHub      `json:"github"`
	HuggingFace *ConfigSourcesHuggingFace `json:"huggingface"`
}

type ConfigSourcesGitHub struct {
	Token  string     `json:"token"`
	Config ConfigRepo `json:"config"`
}

type ConfigSourcesHuggingFace struct {
	Token  string     `json:"token"`
	Config ConfigRepo `json:"config"`
}

// Repository configuration

type ConfigRepo struct {
	Wiki     ConfigRepoWiki     `json:"wiki"`
	Releases ConfigRepoReleases `json:"releases"`
}

type ConfigRepoWiki struct {
	Exclude *bool `json:"exclude"`
}

type ConfigRepoReleases struct {
	Exclude *bool            `json:"exclude"`
	Assets  ConfigRepoAssets `json:"assets"`
}

type ConfigRepoAssets struct {
	Exclude   *bool   `json:"exclude"`
	Threshold *int    `json:"threshold"`
	MaxSize   *string `json:"max_size"`
}

// Repositories configuration

type ConfigGroup struct {
	Source        string `json:"source"`
	Username      string `json:"username"`
	GitLabGroupID *int   `json:"gitlab_group_id"`

	Skip *int `json:"skip"`

	Repositories []ConfigRepositoryRepository `json:"repositories"`
}

func (c *ConfigGroup) GetConfig(repoName string) *ConfigRepositoryRepository {
	if len(c.Repositories) > 0 {
		for _, repo := range c.Repositories {
			if repo.Name == repoName {
				return &repo
			}
		}
	}

	return nil
}

type ConfigRepositoryRepository struct {
	ConfigRepo

	Name    string `json:"name"`
	Exclude *bool  `json:"exclude"`
}

func (c *Configuration) PopulateDefault() {
	if c.Gitlab.URL == nil {
		c.Gitlab.URL = utils.Pointer("https://gitlab.com/")
	}

	if c.Config.Wiki.Exclude == nil {
		c.Config.Wiki.Exclude = utils.Pointer(false)
	}

	if c.Config.Releases.Exclude == nil {
		c.Config.Releases.Exclude = utils.Pointer(false)
	}

	if c.Config.Releases.Assets.Exclude == nil {
		c.Config.Releases.Assets.Exclude = utils.Pointer(false)
	}

	if c.Config.Releases.Assets.Threshold == nil {
		c.Config.Releases.Assets.Threshold = utils.Pointer(5)
	}

	if c.Config.Releases.Assets.MaxSize == nil {
		c.Config.Releases.Assets.MaxSize = utils.Pointer("1G")
	}

	if c.Groups == nil {
		c.Groups = make([]ConfigGroup, 0)
	}

	if c.Sources.GitHub != nil {
		if c.Sources.GitHub.Config.Wiki.Exclude == nil {
			c.Sources.GitHub.Config.Wiki.Exclude = c.Config.Wiki.Exclude
		}

		if c.Sources.GitHub.Config.Releases.Exclude == nil {
			c.Sources.GitHub.Config.Releases.Exclude = c.Config.Releases.Exclude
		}

		if c.Sources.GitHub.Config.Releases.Assets.Exclude == nil {
			c.Sources.GitHub.Config.Releases.Assets.Exclude = c.Config.Releases.Assets.Exclude
		}

		if c.Sources.GitHub.Config.Releases.Assets.Threshold == nil {
			c.Sources.GitHub.Config.Releases.Assets.Threshold = c.Config.Releases.Assets.Threshold
		}

		if c.Sources.GitHub.Config.Releases.Assets.MaxSize == nil {
			c.Sources.GitHub.Config.Releases.Assets.MaxSize = c.Config.Releases.Assets.MaxSize
		}
	}

	for i := range c.Groups {
		repo := &c.Groups[i]

		if repo.Skip == nil {
			repo.Skip = utils.Pointer(0)
		}

		for j := range repo.Repositories {
			repo2 := &repo.Repositories[j]

			if repo2.Exclude == nil {
				repo2.Exclude = utils.Pointer(false)
			}

			// Default to global variables
			if repo2.Wiki.Exclude == nil {
				repo2.Wiki.Exclude = c.Config.Wiki.Exclude
			}

			if repo2.Releases.Exclude == nil {
				repo2.Releases.Exclude = c.Config.Releases.Exclude
			}

			if repo2.Releases.Assets.Exclude == nil {
				repo2.Releases.Assets.Exclude = c.Config.Releases.Assets.Exclude
			}

			if repo2.Releases.Assets.Threshold == nil {
				repo2.Releases.Assets.Threshold = c.Config.Releases.Assets.Threshold
			}

			if repo2.Releases.Assets.MaxSize == nil {
				repo2.Releases.Assets.MaxSize = c.Config.Releases.Assets.MaxSize
			}
		}
	}
}

func (c *Configuration) Validate() error {
	if c.Gitlab.Token == nil {
		return fmt.Errorf("gitlab token is required")
	}

	if c.Dufs.URL == nil {
		return fmt.Errorf("dufs url is required")
	}

	if c.Sources.GitHub == nil && c.Sources.HuggingFace == nil {
		return fmt.Errorf("at least one source is required")
	}

	for i, repo := range c.Groups {
		if repo.Source != "github" && repo.Source != "huggingface" {
			return fmt.Errorf("source must be github or huggingface at index %d", i)
		}

		if len(repo.Username) == 0 {
			return fmt.Errorf("username is required at index %d", i)
		}

		if repo.GitLabGroupID == nil || *repo.GitLabGroupID < 0 {
			return fmt.Errorf("gitlab_group_id is required at index %d", i)
		}

		for j, repo2 := range repo.Repositories {
			if len(repo2.Name) == 0 {
				return fmt.Errorf("name is required at index %d.%d", i, j)
			}
		}
	}

	return nil
}

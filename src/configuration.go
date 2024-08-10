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
	Exclude *bool   `json:"exclude"`
	MaxSize *string `json:"max_size"`
}

// Repositories configuration

type ConfigGroup struct {
	Source        string `json:"source"`
	Username      string `json:"username"`
	GitLabGroupID *int   `json:"gitlab_group_id"`

	Skip   *int       `json:"skip"`
	Config ConfigRepo `json:"config"`

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

	c.Config.DefaultFrom(ConfigRepo{
		Wiki: ConfigRepoWiki{
			Exclude: utils.Pointer(false),
		},
		Releases: ConfigRepoReleases{
			Exclude: utils.Pointer(false),
			Assets: ConfigRepoAssets{
				Exclude: utils.Pointer(false),
				MaxSize: utils.Pointer("1G"),
			},
		},
	})

	if c.Groups == nil {
		c.Groups = make([]ConfigGroup, 0)
	}

	if c.Sources.GitHub != nil {
		c.Sources.GitHub.Config.DefaultFrom(c.Config)
	}

	if c.Sources.HuggingFace != nil {
		c.Sources.HuggingFace.Config.DefaultFrom(c.Config)
	}

	for i := range c.Groups {
		group := &c.Groups[i]

		if group.Skip == nil {
			group.Skip = utils.Pointer(0)
		}

		if group.Source == "github" {
			group.Config.DefaultFrom(c.Sources.GitHub.Config)
		} else if group.Source == "huggingface" {
			group.Config.DefaultFrom(c.Sources.HuggingFace.Config)
		}

		for j := range group.Repositories {
			repo := &group.Repositories[j]

			if repo.Exclude == nil {
				repo.Exclude = utils.Pointer(false)
			}

			repo.ConfigRepo.DefaultFrom(group.Config)
		}
	}
}

func (c *ConfigRepo) DefaultFrom(from ConfigRepo) {
	if c.Wiki.Exclude == nil {
		c.Wiki.Exclude = from.Wiki.Exclude
	}

	if c.Releases.Exclude == nil {
		c.Releases.Exclude = from.Releases.Exclude
	}

	if c.Releases.Assets.Exclude == nil {
		c.Releases.Assets.Exclude = from.Releases.Assets.Exclude
	}

	if c.Releases.Assets.MaxSize == nil {
		c.Releases.Assets.MaxSize = from.Releases.Assets.MaxSize
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

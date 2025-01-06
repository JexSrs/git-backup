package configuration

import (
	"fmt"
	"main/src/utils"
	"strings"
)

type Configuration struct {
	Gitlab  ConfigGitLab    `json:"gitlab"`
	Dufs    ConfigDufs      `json:"dufs"`
	Config  ConfigRepo      `json:"config"`
	Sources []ConfigSources `json:"sources"`
	Groups  []ConfigGroup   `json:"groups"`
}

func (c *Configuration) GetSource(id string) *ConfigSources {
	for i := range c.Sources {
		if id == c.Sources[i].Id {
			return &c.Sources[i]
		}
	}
	return nil
}

type ConfigGitLab struct {
	URL   *string `json:"url"`
	Token *string `json:"token"`
}

type ConfigDufs struct {
	URL      *string `json:"url"`
	RootPath *string `json:"root_path"`
}

// Sources configuration

type ConfigSources struct {
	Id      string     `json:"id"`
	BaseURL string     `json:"base_url"`
	Token   string     `json:"token"`
	Config  ConfigRepo `json:"config"`
}

// Repository configuration

type ConfigRepo struct {
	FetchAvatar *bool              `json:"fetch_avatar"`
	Wiki        ConfigRepoWiki     `json:"wiki"`
	Releases    ConfigRepoReleases `json:"releases"`
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

	IncludeOnly  []string           `json:"include_only"`
	Exclude      []string           `json:"exclude"`
	Repositories []ConfigRepository `json:"repositories"`
}

func (c *ConfigGroup) GetConfig(name string) ConfigRepo {
	cfg := c.Config
	if len(c.Repositories) > 0 {
		nameLower := strings.ToLower(name)
		for _, repo := range c.Repositories {
			if strings.ToLower(repo.Name) == nameLower {
				return repo.ConfigRepo
			}
		}
	}

	return cfg
}

type ConfigRepository struct {
	ConfigRepo

	Name string `json:"name"`
}

func (c *Configuration) PopulateDefault() {
	if c.Gitlab.URL == nil {
		c.Gitlab.URL = utils.Pointer("https://gitlab.com/")
	}

	if c.Dufs.RootPath == nil {
		c.Dufs.RootPath = utils.Pointer("/")
	}

	c.Config.DefaultFrom(ConfigRepo{
		FetchAvatar: utils.Pointer(true),
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

	if c.Sources == nil {
		c.Sources = make([]ConfigSources, 0)
	}

	for i := range c.Sources {
		if strings.HasPrefix(c.Sources[i].Id, "gitlab") && len(c.Sources[i].BaseURL) == 0 {
			c.Sources[i].BaseURL = "https://gitlab.com"
		}

		c.Sources[i].Config.DefaultFrom(c.Config)
	}

	for i := range c.Groups {
		group := &c.Groups[i]

		if group.Skip == nil {
			group.Skip = utils.Pointer(0)
		}

		source := c.GetSource(group.Source)
		if source != nil {
			group.Config.DefaultFrom((*source).Config)
		}

		for j := range group.Repositories {
			repo := &group.Repositories[j]
			repo.ConfigRepo.DefaultFrom(group.Config)
		}
	}
}

func (c *ConfigRepo) DefaultFrom(from ConfigRepo) {
	if c.FetchAvatar == nil {
		c.FetchAvatar = from.FetchAvatar
	}

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
	if c.Dufs.URL == nil {
		return fmt.Errorf("dufs url is required")
	}

	for i, group := range c.Groups {
		source := c.GetSource(group.Source)
		if source == nil {
			return fmt.Errorf("source %s does not exist at index %d", group.Source, i)
		}

		if len(group.Username) == 0 {
			return fmt.Errorf("username is required at index %d", i)
		}

		if group.GitLabGroupID == nil || *group.GitLabGroupID < 0 {
			return fmt.Errorf("gitlab_group_id is required at index %d", i)
		}

		for j, repo2 := range group.Repositories {
			if len(repo2.Name) == 0 {
				return fmt.Errorf("name is required at index %d.%d", i, j)
			}
		}
	}

	return nil
}

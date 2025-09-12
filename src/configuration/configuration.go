package configuration

import (
	"fmt"
	"main/src/utils"
	"net/url"
	"strings"
	"time"
)

type Configuration struct {
	Filter       ConfigFilter        `json:"filter"`
	Config       ConfigRepo          `json:"config"`
	Destinations []ConfigDestination `json:"destinations"`
	Sources      []ConfigSource      `json:"sources"`
	Groups       []ConfigGroup       `json:"groups"`
}

func (c *Configuration) GetSource(id string) *ConfigSource {
	for i := range c.Sources {
		if id == c.Sources[i].ID {
			return &c.Sources[i]
		}
	}
	return nil
}

func (c *Configuration) GetDestination(id string) *ConfigDestination {
	for i := range c.Destinations {
		if id == c.Destinations[i].ID {
			return &c.Destinations[i]
		}
	}
	return nil
}

// Destinations configuration

type ConfigDestination struct {
	ID      string        `json:"id"`
	URL     string        `json:"url"`
	Token   string        `json:"token"`
	Timeout time.Duration `json:"timeout"`
}

// Sources configuration

type ConfigSourceAuth struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

type ConfigSource struct {
	ID      string           `json:"id"`
	BaseURL string           `json:"base_url"`
	Auth    ConfigSourceAuth `json:"auth"`
	Timeout time.Duration    `json:"timeout"`
	Config  ConfigRepo       `json:"config"`
	Filter  ConfigFilter     `json:"filter"`
}

// Repository configuration

type ConfigRepo struct {
	FetchAvatar *bool              `json:"fetch_avatar"`
	Destination *string            `json:"destination"`
	LFS         *bool              `json:"lfs"`
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
	Destination *string `json:"destination"`
	Exclude     *bool   `json:"exclude"`
	MaxSize     *string `json:"max_size"`
}

// Filter configuration

type ConfigFilter struct {
	OnlyNew        *bool                      `json:"only_new"`
	Visibility     []string                   `json:"visibility"`
	Archived       *bool                      `json:"archived"`
	Empty          *bool                      `json:"empty"`
	HasDescription *bool                      `json:"has_description"`
	License        []string                   `json:"license"`
	Topics         []string                   `json:"topics"`
	Pages          *bool                      `json:"pages"`
	Discussions    *bool                      `json:"discussions"`
	Forked         *bool                      `json:"forked"`
	NameRegex      *string                    `json:"name"`
	Language       []string                   `json:"language"`
	Stars          ConfigFilterMinMax[int]    `json:"stars"`
	Watchers       ConfigFilterMinMax[int]    `json:"watchers"`
	Forks          ConfigFilterMinMax[int]    `json:"forks"`
	Branches       ConfigFilterMinMax[int]    `json:"branches"`
	Tags           ConfigFilterMinMax[int]    `json:"tags"`
	CreatedAt      ConfigFilterMinMax[string] `json:"created"`
	Updated        ConfigFilterMinMax[string] `json:"updated"`
	Size           ConfigFilterMinMax[int]    `json:"size"`
	Issues         ConfigFilterIssues         `json:"issues"`
}

type ConfigFilterMinMax[T any] struct {
	Min *T `json:"min"`
	Max *T `json:"max"`
}

type ConfigFilterIssues struct {
	Enabled *bool                   `json:"enabled"`
	Open    ConfigFilterMinMax[int] `json:"open"`
}

// Repositories configuration

type ConfigGroup struct {
	Source   string `json:"source"`
	Username string `json:"username"`

	GitlabGroupID int    `json:"gitlab_group_id"`
	GiteaUsername string `json:"gitea_username"`

	Skip   *int         `json:"skip"`
	Filter ConfigFilter `json:"filter"`
	Config ConfigRepo   `json:"config"`

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
	Filter ConfigFilter `json:"filter"`

	Name string `json:"name"`
	URL  string `json:"url"` // user only for git source
}

func (c *Configuration) PopulateDefault() {
	c.Filter.DefaultFrom(ConfigFilter{
		Visibility: []string{"public", "private"},
		OnlyNew:    utils.Pointer(false),
	})

	c.Config.DefaultFrom(ConfigRepo{
		FetchAvatar: utils.Pointer(true),
		LFS:         utils.Pointer(true),
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

	if c.Destinations == nil {
		c.Destinations = make([]ConfigDestination, 0)
	}

	if c.Sources == nil {
		c.Sources = make([]ConfigSource, 0)
	}

	for i := range c.Sources {
		if strings.HasPrefix(c.Sources[i].ID, "gitlab") && len(c.Sources[i].BaseURL) == 0 {
			c.Sources[i].BaseURL = "https://gitlab.com"
		}

		if c.Sources[i].Timeout == 0 {
			c.Sources[i].Timeout = time.Minute * 10
		} else {
			c.Sources[i].Timeout = time.Second * c.Sources[i].Timeout // User is passing seconds
		}

		c.Sources[i].Config.DefaultFrom(c.Config)
		c.Sources[i].Filter.DefaultFrom(c.Filter)
	}

	for i := range c.Destinations {
		if c.Destinations[i].Timeout == 0 {
			c.Destinations[i].Timeout = time.Minute * 10
		} else {
			c.Destinations[i].Timeout = time.Second * c.Destinations[i].Timeout // User is passing seconds
		}

		c.Sources[i].Config.DefaultFrom(c.Config)
		c.Sources[i].Filter.DefaultFrom(c.Filter)
	}

	for i := range c.Groups {
		group := &c.Groups[i]

		if group.Skip == nil {
			group.Skip = utils.Pointer(0)
		}

		if len(group.GiteaUsername) == 0 {
			group.GiteaUsername = group.Username
		}

		source := c.GetSource(group.Source)
		if source != nil {
			group.Config.DefaultFrom((*source).Config)
			group.Filter.DefaultFrom((*source).Filter)
		}

		for j := range group.Repositories {
			repo := &group.Repositories[j]
			repo.ConfigRepo.DefaultFrom(group.Config)
			repo.Filter.DefaultFrom(group.Filter)
		}
	}
}

func (c *ConfigRepo) DefaultFrom(from ConfigRepo) {
	if c.FetchAvatar == nil {
		c.FetchAvatar = from.FetchAvatar
	}

	if c.LFS == nil {
		c.LFS = from.LFS
	}

	if c.Destination == nil {
		c.Destination = from.Destination
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

	if c.Releases.Assets.Destination == nil {
		c.Releases.Assets.Destination = from.Releases.Assets.Destination
	}
}

func (c *ConfigFilter) DefaultFrom(from ConfigFilter) {
	if c.OnlyNew == nil {
		c.OnlyNew = from.OnlyNew
	}

	if c.Visibility == nil {
		c.Visibility = from.Visibility
	}

	if c.Archived == nil {
		c.Archived = from.Archived
	}

	if c.Empty == nil {
		c.Empty = from.Empty
	}

	if c.HasDescription == nil {
		c.HasDescription = from.HasDescription
	}

	if c.License == nil {
		c.License = from.License
	}

	if c.Topics == nil {
		c.Topics = from.Topics
	}

	if c.Pages == nil {
		c.Pages = from.Pages
	}

	if c.Discussions == nil {
		c.Discussions = from.Discussions
	}

	if c.Forked == nil {
		c.Forked = from.Forked
	}

	if c.NameRegex == nil {
		c.NameRegex = from.NameRegex
	}

	if c.Language == nil {
		c.Language = from.Language
	}

	if c.Stars.Min == nil {
		c.Stars.Min = from.Stars.Min
	}

	if c.Stars.Max == nil {
		c.Stars.Max = from.Stars.Max
	}

	if c.Watchers.Min == nil {
		c.Watchers.Min = from.Watchers.Min
	}

	if c.Watchers.Max == nil {
		c.Watchers.Max = from.Watchers.Max
	}

	if c.Forks.Min == nil {
		c.Forks.Min = from.Forks.Min
	}

	if c.Forks.Max == nil {
		c.Forks.Max = from.Forks.Max
	}

	if c.Branches.Min == nil {
		c.Branches.Min = from.Branches.Min
	}

	if c.Branches.Max == nil {
		c.Branches.Max = from.Branches.Max
	}

	if c.Tags.Min == nil {
		c.Tags.Min = from.Tags.Min
	}

	if c.Tags.Max == nil {
		c.Tags.Max = from.Tags.Max
	}

	if c.CreatedAt.Min == nil {
		c.CreatedAt.Min = from.CreatedAt.Min
	}

	if c.CreatedAt.Max == nil {
		c.CreatedAt.Max = from.CreatedAt.Max
	}

	if c.Updated.Min == nil {
		c.Updated.Min = from.Updated.Min
	}

	if c.Updated.Max == nil {
		c.Updated.Max = from.Updated.Max
	}

	if c.Size.Min == nil {
		c.Size.Min = from.Size.Min
	}

	if c.Size.Max == nil {
		c.Size.Max = from.Size.Max
	}

	if c.Issues.Enabled == nil {
		c.Issues.Enabled = from.Issues.Enabled
	}

	if c.Issues.Open.Min == nil {
		c.Issues.Open.Min = from.Issues.Open.Min
	}

	if c.Issues.Open.Max == nil {
		c.Issues.Open.Max = from.Issues.Open.Max
	}
}

func (c *Configuration) Validate() error {
	for _, source := range c.Sources {
		_, err := url.Parse(source.BaseURL)
		if err != nil {
			return fmt.Errorf("source %s has invalid base url: %w", source.ID, err)
		}
	}

	for _, dest := range c.Destinations {
		_, err := url.Parse(dest.URL)
		if err != nil {
			return fmt.Errorf("destination %s has invalid base url: %w", dest.ID, err)
		}
	}

	assetsEnabled := !*c.Config.Releases.Assets.Exclude
	for i, group := range c.Groups {
		source := c.GetSource(group.Source)
		if source == nil {
			return fmt.Errorf("source %s does not exist at index %d", group.Source, i)
		}

		assetsEnabled = assetsEnabled || !*source.Config.Releases.Assets.Exclude

		if len(group.Username) == 0 {
			return fmt.Errorf("username is required at index %d", i)
		}

		if group.GitlabGroupID < 0 {
			return fmt.Errorf("gitlab_group_id is invalid at index %d", i)
		}

		for j, repo := range group.Repositories {
			if len(repo.Name) == 0 && !utils.CheckPrefix(source.ID, "git") {
				return fmt.Errorf("name is required at index %d.%d", i, j)
			}

			assetsEnabled = assetsEnabled || !*repo.Releases.Assets.Exclude

			if repo.Destination == nil {
				return fmt.Errorf("repository destination is required at index %d.%d", i, j)
			}

			gitDst := c.GetDestination(*repo.Destination)
			if gitDst == nil {
				return fmt.Errorf("destination %s does not exist at index %d.%d", *repo.Destination, i, j)
			}

			if repo.Releases.Assets.Destination == nil {
				return fmt.Errorf("asset destination is required at index %d.%d", i, j)
			}

			storageDst := c.GetDestination(*repo.Releases.Assets.Destination)
			if storageDst == nil {
				return fmt.Errorf("asset destination %s does not exist at index %d.%d", *repo.Destination, i, j)
			}
		}
	}

	return nil
}

package configuration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPopulateDefault_RetriesDefaultToZeroAtEveryLevel(t *testing.T) {
	c := validConfig()
	c.PopulateDefault()

	if c.Config.Retries == nil || *c.Config.Retries != 0 {
		t.Fatalf("expected global config.retries to default to 0, got %v", c.Config.Retries)
	}
	for _, s := range c.Sources {
		if s.Config.Retries == nil || *s.Config.Retries != 0 {
			t.Fatalf("expected source %s config.retries to default to 0, got %v", s.ID, s.Config.Retries)
		}
	}
	for _, g := range c.Groups {
		if g.Config.Retries == nil || *g.Config.Retries != 0 {
			t.Fatalf("expected group %s config.retries to default to 0, got %v", g.Username, g.Config.Retries)
		}
		for _, r := range g.Repositories {
			if r.Retries == nil || *r.Retries != 0 {
				t.Fatalf("expected repository %s retries to default to 0, got %v", r.Name, r.Retries)
			}
		}
	}
}

func TestPopulateDefault_RetriesCascadeFromGlobalThroughSourceGroupAndRepository(t *testing.T) {
	c := validConfig()
	c.Config.Retries = intPtr(3)
	c.Sources = []ConfigSource{
		{ID: "github", Auth: ConfigSourceAuth{Token: "gh"}},    // inherits global → 3
		{ID: "gitlab", Config: ConfigRepo{Retries: intPtr(5)}}, // overrides global
	}
	c.Groups = []ConfigGroup{
		{Source: "github", Username: "a", Repositories: []ConfigRepository{{Name: "r"}}}, // inherits github → 3
		{Source: "gitlab", Username: "b", Repositories: []ConfigRepository{{Name: "r"}}}, // inherits gitlab → 5
		{
			Source: "gitlab", Username: "c",
			Config: ConfigRepo{Retries: intPtr(1)}, // own value
			Repositories: []ConfigRepository{
				{Name: "inherits"}, // inherits group → 1
				{Name: "own", ConfigRepo: ConfigRepo{Retries: intPtr(7)}}, // own value
			},
		},
	}
	c.PopulateDefault()

	if got := *c.Sources[0].Config.Retries; got != 3 {
		t.Errorf("github source should inherit global retries 3, got %d", got)
	}
	if got := *c.Sources[1].Config.Retries; got != 5 {
		t.Errorf("gitlab source should keep its own retries 5, got %d", got)
	}
	if got := *c.Groups[0].Config.Retries; got != 3 {
		t.Errorf("group a should inherit 3 from github, got %d", got)
	}
	if got := *c.Groups[1].Config.Retries; got != 5 {
		t.Errorf("group b should inherit 5 from gitlab, got %d", got)
	}
	if got := *c.Groups[2].Config.Retries; got != 1 {
		t.Errorf("group c should keep its own retries 1, got %d", got)
	}
	if got := *c.Groups[2].Repositories[0].Retries; got != 1 {
		t.Errorf("repository 'inherits' should inherit 1 from group c, got %d", got)
	}
	if got := *c.Groups[2].Repositories[1].Retries; got != 7 {
		t.Errorf("repository 'own' should keep its own retries 7, got %d", got)
	}
}

func TestConfigRepo_RetryCount(t *testing.T) {
	if got := (ConfigRepo{}).RetryCount(); got != 0 {
		t.Fatalf("nil retries should count as 0, got %d", got)
	}
	if got := (ConfigRepo{Retries: intPtr(4)}).RetryCount(); got != 4 {
		t.Fatalf("expected 4, got %d", got)
	}
}

func TestValidate_RejectsNegativeRetries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Configuration)
	}{
		{"global", func(c *Configuration) { c.Config.Retries = intPtr(-1) }},
		{"source", func(c *Configuration) { c.Sources[0].Config.Retries = intPtr(-1) }},
		{"group", func(c *Configuration) { c.Groups[0].Config.Retries = intPtr(-2) }},
		{"repository", func(c *Configuration) { c.Groups[0].Repositories[0].Retries = intPtr(-3) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.mutate(&c)
			c.PopulateDefault()

			err := c.Validate()
			if err == nil {
				t.Fatal("expected an error for negative retries, got nil")
			}
			if !strings.Contains(err.Error(), "retries") {
				t.Fatalf("error %q should mention retries", err)
			}
		})
	}
}

func TestRetries_DecodeFromJSONInsideConfigAtEveryLevel(t *testing.T) {
	raw := `{
		"config": {"retries": 2},
		"sources": [{"id": "github", "config": {"retries": 3}}],
		"groups": [{
			"source": "github", "username": "octo",
			"config": {"retries": 4},
			"repositories": [{"name": "r", "retries": 5}]
		}]
	}`

	var c Configuration
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Config.Retries == nil || *c.Config.Retries != 2 {
		t.Errorf("global config.retries = %v, want 2", c.Config.Retries)
	}
	if c.Sources[0].Config.Retries == nil || *c.Sources[0].Config.Retries != 3 {
		t.Errorf("source config.retries = %v, want 3", c.Sources[0].Config.Retries)
	}
	if c.Groups[0].Config.Retries == nil || *c.Groups[0].Config.Retries != 4 {
		t.Errorf("group config.retries = %v, want 4", c.Groups[0].Config.Retries)
	}
	if c.Groups[0].Repositories[0].Retries == nil || *c.Groups[0].Repositories[0].Retries != 5 {
		t.Errorf("repository retries = %v, want 5", c.Groups[0].Repositories[0].Retries)
	}
}

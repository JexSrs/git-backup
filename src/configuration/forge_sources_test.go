package configuration

import (
	"strings"
	"testing"
)

func TestPopulateDefault_LavaforgeSourceBaseURL(t *testing.T) {
	c := Configuration{Sources: []ConfigSource{
		{ID: "lavaforge"},
		{ID: "lavaforge-mirror", BaseURL: "https://forge.example.com"},
		{ID: "gitea"},
	}}
	c.PopulateDefault()

	if got := c.Sources[0].BaseURL; got != "https://lavaforge.org" {
		t.Errorf("expected lavaforge source to default to https://lavaforge.org, got %q", got)
	}
	if got := c.Sources[1].BaseURL; got != "https://forge.example.com" {
		t.Errorf("expected explicit base url to be kept, got %q", got)
	}
	if got := c.Sources[2].BaseURL; got != "" {
		t.Errorf("expected gitea source to have no default base url, got %q", got)
	}
}

func TestValidate_GiteaSourceRequiresBaseURL(t *testing.T) {
	c := validConfig()
	c.Sources = append(c.Sources, ConfigSource{ID: "gitea-home"})
	c.PopulateDefault()

	err := c.Validate()
	if err == nil {
		t.Fatal("expected an error for a gitea source without base_url")
	}
	if !strings.Contains(err.Error(), "gitea-home") || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("error %q should name the source and base_url", err)
	}

	c = validConfig()
	c.Sources = append(c.Sources, ConfigSource{ID: "gitea-home", BaseURL: "https://gitea.home.lan"})
	c.PopulateDefault()
	if err := c.Validate(); err != nil {
		t.Fatalf("expected a gitea source with base_url to be valid, got %v", err)
	}
}

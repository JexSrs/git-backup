package main

import (
	"main/src/configuration"
	"main/src/dest"
	"main/src/sources"
	"testing"
)

func TestMapSources_SelectsTheImplementationByIDPrefix(t *testing.T) {
	cfg := configuration.Configuration{Sources: []configuration.ConfigSource{
		{ID: "github"},
		{ID: "github-work"},
		{ID: "huggingface"},
		{ID: "gitlab"},
		{ID: "gitlab-custom-1", BaseURL: "https://gitlab.example.com"},
		{ID: "git"},
		{ID: "git-proxmox"},
		{ID: "gitea", BaseURL: "https://gitea.example.com"},
		{ID: "gitea-home", BaseURL: "https://gitea.home.lan"},
		{ID: "lavaforge", BaseURL: "https://lavaforge.org"},
		{ID: "bitbucket"},
	}}

	got := mapSources(cfg)

	isGithub := func(s sources.Source) bool { _, ok := s.(*sources.Github); return ok }
	isHuggingFace := func(s sources.Source) bool { _, ok := s.(*sources.HuggingFace); return ok }
	isGitlab := func(s sources.Source) bool { _, ok := s.(*sources.Gitlab); return ok }
	isGit := func(s sources.Source) bool { _, ok := s.(*sources.Git); return ok }
	isGitea := func(s sources.Source) bool { _, ok := s.(*sources.Gitea); return ok }

	checks := map[string]func(sources.Source) bool{
		"github":          isGithub,
		"github-work":     isGithub,
		"huggingface":     isHuggingFace,
		"gitlab":          isGitlab,
		"gitlab-custom-1": isGitlab,
		"git":             isGit,
		"git-proxmox":     isGit,
		"gitea":           isGitea,
		"gitea-home":      isGitea,
		"lavaforge":       isGitea,
	}
	for id, check := range checks {
		if source, ok := got[id]; !ok || !check(source) {
			t.Errorf("source %q mapped to %T", id, got[id])
		}
	}
	if _, ok := got["bitbucket"]; ok {
		t.Errorf("expected an unknown prefix not to be mapped, got %T", got["bitbucket"])
	}
}

func TestMapDestinations_SelectsTheImplementationByIDPrefix(t *testing.T) {
	cfg := configuration.Configuration{Destinations: []configuration.ConfigDestination{
		{ID: "gitlab", URL: "https://gitlab.example.com"},
		{ID: "gitlab-backup", URL: "https://gitlab.example.com"},
		{ID: "dufs", URL: "https://dufs.example.com"},
		{ID: "gitea-home", URL: "https://gitea.example.com"},
		{ID: "s3", URL: "https://s3.example.com"},
	}}

	got := mapDestinations(cfg)

	isGitlab := func(d dest.Destination) bool { _, ok := d.(*dest.GitLab); return ok }
	isDufs := func(d dest.Destination) bool { _, ok := d.(*dest.Dufs); return ok }
	isGitea := func(d dest.Destination) bool { _, ok := d.(*dest.Gitea); return ok }

	checks := map[string]func(dest.Destination) bool{
		"gitlab":        isGitlab,
		"gitlab-backup": isGitlab,
		"dufs":          isDufs,
		"gitea-home":    isGitea,
	}
	for id, check := range checks {
		if destination, ok := got[id]; !ok || !check(destination) {
			t.Errorf("destination %q mapped to %T", id, got[id])
		}
	}
	if _, ok := got["s3"]; ok {
		t.Errorf("expected an unknown prefix not to be mapped, got %T", got["s3"])
	}
	if id := got["gitea-home"].GetIdentification(); id.ID != "gitea-home" {
		t.Errorf("expected the destination to keep its configured id, got %q", id.ID)
	}
}

package sources

import (
	"main/src/configuration"
	"testing"
)

func proxmoxGroup() configuration.ConfigGroup {
	return configuration.ConfigGroup{
		Source:   "git-proxmox",
		Username: "proxmox",
		Repositories: []configuration.ConfigRepository{
			{Name: "lxc", URL: "https://git.proxmox.com/git/lxc.git"},
			{URL: "https://git.proxmox.com/git/pve-manager.git"},
		},
	}
}

func TestGit_Paginate_FirstPageListsConfiguredRepositories(t *testing.T) {
	g := NewGit(configuration.ConfigSource{ID: "git-proxmox"})

	res, err := g.Paginate("proxmox", proxmoxGroup(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(res.Repositories))
	}

	if got := res.Repositories[0]; got.Name != "lxc" || got.URL != "https://git.proxmox.com/git/lxc.git" {
		t.Errorf("expected lxc repository with its url, got %+v", got)
	}
	if got := res.Repositories[1]; got.Name != "pve-manager" {
		t.Errorf("expected the name to be derived from the url when missing, got %q", got.Name)
	}
	if res.NextPage != 2 {
		t.Errorf("expected next page 2, got %d", res.NextPage)
	}
}

func TestGit_Paginate_SecondPageIsEmpty(t *testing.T) {
	g := NewGit(configuration.ConfigSource{ID: "git-proxmox"})

	first, err := g.Paginate("proxmox", proxmoxGroup(), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.Paginate("proxmox", proxmoxGroup(), first)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(second.Repositories) != 0 {
		t.Fatalf("expected no repositories after the first page, got %d", len(second.Repositories))
	}
}

func TestGit_AddTokenToCloneUrl(t *testing.T) {
	tests := []struct {
		name     string
		username string
		token    string
		want     string
	}{
		{"username and token", "user", "tok", "https://user:tok@git.example.com/repo.git"},
		{"username only", "user", "", "https://user@git.example.com/repo.git"},
		{"token only", "", "tok", "https://:tok@git.example.com/repo.git"},
		{"no credentials", "", "", "https://git.example.com/repo.git"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGit(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Username: tc.username, Token: tc.token}})
			if got := g.AddTokenToCloneUrl("https://git.example.com/repo.git"); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGit_FetchUsernamePassword_ReturnsConfiguredCredentials(t *testing.T) {
	g := NewGit(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Username: "user", Token: "tok"}})

	if u, p := g.FetchUsernamePassword(); u != "user" || p != "tok" {
		t.Fatalf("expected (user, tok), got (%q, %q)", u, p)
	}
}

func TestGit_HasNoWikiOrReleases(t *testing.T) {
	g := NewGit(configuration.ConfigSource{})

	if got := g.GetWikiURL("proxmox", "lxc"); got != "" {
		t.Errorf("expected no wiki url, got %q", got)
	}
	releases, err := g.FetchReleases("proxmox", SourceRepository{Name: "lxc"})
	if err != nil || releases == nil || len(releases) != 0 {
		t.Errorf("expected an empty, non-nil release list, got (%v, %v)", releases, err)
	}
}

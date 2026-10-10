package dest

import (
	"net/http"
	"testing"
)

func TestGitea_RemoteURL_EmbedsTheTokenInTheRepositoryURL(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {})

	if got, want := g.RemoteURL(giteaRepo()), "http://oauth2:secret@gitea.local/owner/my-repo.git"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGitLab_RemoteURL_EmbedsTheTokenInTheRepositoryURL(t *testing.T) {
	g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {})

	if got, want := g.RemoteURL(gitlabRepo()), "https://oauth2:secret@gitlab.local/grp/myrepo.git"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDufs_RemoteURL_IsEmptyBecauseItHostsNoRepositories(t *testing.T) {
	d, _ := newFakeDufs(t, func(w http.ResponseWriter, r *http.Request) {})

	if got := d.RemoteURL(&Repository{ID: "1", HttpUrl: "https://dufs.local/x"}); got != "" {
		t.Fatalf("expected an empty remote url, got %q", got)
	}
}

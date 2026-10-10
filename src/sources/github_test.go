package sources

import (
	"main/src/configuration"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newGithub(t *testing.T, token string, handler http.HandlerFunc) (*Github, *[]recordedRequest) {
	t.Helper()

	server, requests := newRecordingServer(t, handler)
	g := NewGithub(configuration.ConfigSource{
		ID:      "github",
		Auth:    configuration.ConfigSourceAuth{Token: token},
		Timeout: time.Minute,
	})
	g.client.Transport = redirectHost(t, "api.github.com", server)

	return g, requests
}

const githubSearchPage = `{"items": [
  {"name": "alpha", "clone_url": "https://github.com/octo/alpha.git", "description": "first", "private": false, "archived": true, "fork": false, "size": 120},
  {"name": "beta", "clone_url": "https://github.com/octo/beta.git", "description": null, "private": true, "archived": false, "fork": true, "size": 0}
]}`

func TestGithub_Paginate_MapsRepositories(t *testing.T) {
	g, requests := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(githubSearchPage))
	})

	res, err := g.Paginate("octo", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := (*requests)[0]
	if req.Path != "/search/repositories" {
		t.Errorf("expected the search endpoint, got %s", req.Path)
	}
	if q := req.Query.Get("q"); q != "user:octo fork:true" {
		t.Errorf("expected query to select the user's repositories including forks, got %q", q)
	}
	if req.Query.Get("page") != "1" || req.Query.Get("per_page") != "100" {
		t.Errorf("expected page=1 and per_page=100, got %v", req.Query)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("expected bearer token, got %q", got)
	}

	if len(res.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(res.Repositories))
	}
	alpha := res.Repositories[0]
	if alpha.Name != "alpha" || alpha.URL != "https://github.com/octo/alpha.git" {
		t.Errorf("unexpected name/url: %+v", alpha)
	}
	if alpha.Description == nil || *alpha.Description != "first" {
		t.Errorf("expected description 'first', got %v", alpha.Description)
	}
	if alpha.Private || !alpha.Archived || alpha.Forked || alpha.Empty {
		t.Errorf("expected public, archived, not forked, not empty; got %+v", alpha)
	}

	beta := res.Repositories[1]
	if beta.Description != nil {
		t.Errorf("expected nil description for null, got %q", *beta.Description)
	}
	if !beta.Private || beta.Archived || !beta.Forked || !beta.Empty {
		t.Errorf("expected private, not archived, forked, empty (size 0); got %+v", beta)
	}
	if res.NextPage != 2 {
		t.Errorf("expected next page 2, got %d", res.NextPage)
	}
}

func TestGithub_Paginate_RequestsThePageAfterThePreviousOne(t *testing.T) {
	g, requests := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items": []}`))
	})

	res, err := g.Paginate("octo", configuration.ConfigGroup{}, &PaginationResponse{NextPage: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := (*requests)[0].Query.Get("page"); got != "3" {
		t.Errorf("expected page=3, got %q", got)
	}
	if len(res.Repositories) != 0 || res.NextPage != 4 {
		t.Errorf("expected an empty page advancing to 4, got %d repos / next %d", len(res.Repositories), res.NextPage)
	}
}

func TestGithub_Paginate_WithoutTokenSendsNoAuthorization(t *testing.T) {
	g, requests := newGithub(t, "", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items": []}`))
	})

	if _, err := g.Paginate("octo", configuration.ConfigGroup{}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := (*requests)[0].Header.Get("Authorization"); got != "" {
		t.Fatalf("expected no Authorization header, got %q", got)
	}
}

func TestGithub_Paginate_NonOKStatusIsError(t *testing.T) {
	g, _ := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if _, err := g.Paginate("octo", configuration.ConfigGroup{}, nil); err == nil {
		t.Fatal("expected an error for a 403 response")
	}
}

func TestGithub_Paginate_InvalidJSONIsError(t *testing.T) {
	g, _ := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items": [`))
	})

	if _, err := g.Paginate("octo", configuration.ConfigGroup{}, nil); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

const githubReleases = `[
  {"name": "Release 2", "tag_name": "v2.0.0", "body": "second", "created_at": "2024-02-01T00:00:00Z",
   "assets": [{"name": "app-v2.zip", "url": "https://api.github.com/repos/octo/repo/releases/assets/2", "browser_download_url": "https://github.com/octo/repo/releases/download/v2.0.0/app-v2.zip"}]},
  {"name": "Release 1", "tag_name": "v1.0.0", "body": "first", "created_at": "2024-01-01T00:00:00Z", "assets": []}
]`

// FetchReleases uses its own http.Client, so the default transport is redirected for the test.
func redirectDefaultTransport(t *testing.T, host string, server *httptest.Server) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = redirectHost(t, host, server)
	t.Cleanup(func() { http.DefaultTransport = original })
}

func TestGithub_FetchReleases_ReturnsOldestFirstWithAssets(t *testing.T) {
	server, requests := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(githubReleases))
	})
	redirectDefaultTransport(t, "api.github.com", server)

	g := NewGithub(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Token: "tok"}, Timeout: time.Minute})
	releases, err := g.FetchReleases("octo", SourceRepository{Name: "repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := (*requests)[0]
	if req.Path != "/repos/octo/repo/releases" {
		t.Errorf("expected the releases endpoint for octo/repo, got %s", req.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("expected bearer token, got %q", got)
	}

	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}
	if releases[0].TagName != "v1.0.0" || releases[1].TagName != "v2.0.0" {
		t.Errorf("expected oldest release first, got %q then %q", releases[0].TagName, releases[1].TagName)
	}
	if r := releases[0]; r.Name != "Release 1" || r.Description != "first" || r.CreatedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("unexpected release mapping: %+v", r)
	}
	if len(releases[1].Assets) != 1 || releases[1].Assets[0].Name != "app-v2.zip" {
		t.Errorf("expected one asset named app-v2.zip, got %+v", releases[1].Assets)
	}
}

func TestGithub_FetchReleases_NonOKStatusIsError(t *testing.T) {
	server, _ := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	redirectDefaultTransport(t, "api.github.com", server)

	g := NewGithub(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Token: "tok"}})
	if _, err := g.FetchReleases("octo", SourceRepository{Name: "repo"}); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestGithub_AddTokenToCloneUrl_EmbedsTokenAsUser(t *testing.T) {
	g := NewGithub(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Token: "tok"}})

	if got := g.AddTokenToCloneUrl("https://github.com/octo/repo.git"); got != "https://tok@github.com/octo/repo.git" {
		t.Fatalf("got %q", got)
	}
}

func TestGithub_GetWikiURL(t *testing.T) {
	g := NewGithub(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Token: "tok"}})

	want := "https://tok:x-oauth-basic@github.com/octo/repo.wiki.git"
	if got := g.GetWikiURL("octo", "repo"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGithub_FetchUsernamePassword_UsesTokenAsUsername(t *testing.T) {
	g := NewGithub(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Token: "tok"}})

	if u, p := g.FetchUsernamePassword(); u != "tok" || p != "" {
		t.Fatalf("expected (tok, \"\"), got (%q, %q)", u, p)
	}
}

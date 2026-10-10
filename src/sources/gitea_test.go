package sources

import (
	"main/src/configuration"
	"net/http"
	"strings"
	"testing"
	"time"
)

func newGiteaSource(t *testing.T, token string, handler http.HandlerFunc) (*Gitea, *[]recordedRequest) {
	t.Helper()

	server, requests := newRecordingServer(t, handler)
	g := NewGitea(configuration.ConfigSource{
		ID:      "gitea",
		BaseURL: server.URL,
		Auth:    configuration.ConfigSourceAuth{Token: token},
		Timeout: time.Minute,
	})

	return g, requests
}

const giteaReposPage = `[
  {"name": "alpha", "clone_url": "https://gitea.example.com/octo/alpha.git", "description": "first", "avatar_url": "https://gitea.example.com/repo-avatars/1",
   "private": false, "archived": true, "empty": false, "fork": false},
  {"name": "beta", "clone_url": "https://gitea.example.com/octo/beta.git", "description": "", "avatar_url": "",
   "private": true, "archived": false, "empty": true, "fork": true}
]`

// giteaOwner serves octo's repositories through the organisation endpoint (orgPage) and the user
// endpoint (userPage); a nil page makes that endpoint answer 404.
func giteaOwner(orgPage, userPage *string) http.HandlerFunc {
	serve := func(w http.ResponseWriter, page *string) {
		if page == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(*page))
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/orgs/"):
			serve(w, orgPage)
		case strings.HasPrefix(r.URL.Path, "/api/v1/users/"):
			serve(w, userPage)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func strp(s string) *string { return &s }

func TestGitea_Paginate_ListsAnOrganisationsRepositories(t *testing.T) {
	g, requests := newGiteaSource(t, "tok", giteaOwner(strp(giteaReposPage), nil))

	res, err := g.Paginate("octo", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("expected a single request, got %d", len(*requests))
	}
	req := (*requests)[0]
	if req.Path != "/api/v1/orgs/octo/repos" {
		t.Errorf("expected the organisation repos endpoint, got %s", req.Path)
	}
	if req.Query.Get("page") != "1" || req.Query.Get("limit") != "50" {
		t.Errorf("expected page=1 and limit=50, got %v", req.Query)
	}
	if got := req.Header.Get("Authorization"); got != "token tok" {
		t.Errorf("expected the gitea token header, got %q", got)
	}

	if len(res.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(res.Repositories))
	}
	alpha := res.Repositories[0]
	if alpha.Name != "alpha" || alpha.URL != "https://gitea.example.com/octo/alpha.git" {
		t.Errorf("unexpected name/url: %+v", alpha)
	}
	if alpha.Description == nil || *alpha.Description != "first" {
		t.Errorf("expected description 'first', got %v", alpha.Description)
	}
	if alpha.Avatar == nil || *alpha.Avatar != "https://gitea.example.com/repo-avatars/1" {
		t.Errorf("expected the avatar url, got %v", alpha.Avatar)
	}
	if alpha.Private || !alpha.Archived || alpha.Forked || alpha.Empty {
		t.Errorf("expected public, archived, not forked, not empty; got %+v", alpha)
	}

	beta := res.Repositories[1]
	if beta.Description != nil {
		t.Errorf("expected an empty description to map to nil, got %q", *beta.Description)
	}
	if beta.Avatar != nil {
		t.Errorf("expected an empty avatar url to map to nil, got %q", *beta.Avatar)
	}
	if !beta.Private || beta.Archived || !beta.Forked || !beta.Empty {
		t.Errorf("expected private, not archived, forked, empty; got %+v", beta)
	}
	if res.NextPage != 2 {
		t.Errorf("expected next page 2, got %d", res.NextPage)
	}
}

func TestGitea_Paginate_FallsBackToTheUserEndpointWhenTheOwnerIsNotAnOrganisation(t *testing.T) {
	g, requests := newGiteaSource(t, "tok", giteaOwner(nil, strp(giteaReposPage)))

	res, err := g.Paginate("octo", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*requests) != 2 {
		t.Fatalf("expected the org probe followed by the user request, got %d requests", len(*requests))
	}
	if (*requests)[0].Path != "/api/v1/orgs/octo/repos" || (*requests)[1].Path != "/api/v1/users/octo/repos" {
		t.Errorf("unexpected request order: %s then %s", (*requests)[0].Path, (*requests)[1].Path)
	}
	if len(res.Repositories) != 2 {
		t.Fatalf("expected 2 repositories from the user endpoint, got %d", len(res.Repositories))
	}
}

func TestGitea_Paginate_RemembersTheOwnerKindOnLaterPages(t *testing.T) {
	g, requests := newGiteaSource(t, "tok", giteaOwner(nil, strp(`[]`)))

	first, err := g.Paginate("octo", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error on page 1: %v", err)
	}
	*requests = (*requests)[:0]

	second, err := g.Paginate("octo", configuration.ConfigGroup{}, first)
	if err != nil {
		t.Fatalf("unexpected error on page 2: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("expected exactly one request for page 2 (no re-probe), got %d", len(*requests))
	}
	req := (*requests)[0]
	if req.Path != "/api/v1/users/octo/repos" || req.Query.Get("page") != "2" {
		t.Errorf("expected page 2 of the user endpoint, got %s page=%s", req.Path, req.Query.Get("page"))
	}
	if second.NextPage != 3 {
		t.Errorf("expected next page 3, got %d", second.NextPage)
	}
}

func TestGitea_Paginate_UnknownOwnerIsError(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", giteaOwner(nil, nil))

	if _, err := g.Paginate("octo", configuration.ConfigGroup{}, nil); err == nil {
		t.Fatal("expected an error when neither endpoint knows the owner")
	}
}

func TestGitea_Paginate_WithoutTokenSendsNoAuthorization(t *testing.T) {
	g, requests := newGiteaSource(t, "", giteaOwner(strp(`[]`), nil))

	if _, err := g.Paginate("octo", configuration.ConfigGroup{}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := (*requests)[0].Header.Get("Authorization"); got != "" {
		t.Fatalf("expected no Authorization header, got %q", got)
	}
}

func TestGitea_Paginate_NonOKStatusIsError(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := g.Paginate("octo", configuration.ConfigGroup{}, nil); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestGitea_Paginate_InvalidJSONIsError(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", giteaOwner(strp(`[{"name": `), nil))

	if _, err := g.Paginate("octo", configuration.ConfigGroup{}, nil); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

const giteaReleases = `[
  {"name": "Release 2", "tag_name": "v2.0.0", "body": "second", "created_at": "2024-02-01T00:00:00Z",
   "assets": [{"name": "app-v2.zip", "browser_download_url": "https://gitea.example.com/octo/repo/releases/download/v2.0.0/app-v2.zip"}]},
  {"name": "Release 1", "tag_name": "v1.0.0", "body": "first", "created_at": "2024-01-01T00:00:00Z", "assets": []}
]`

func TestGitea_FetchReleases_ReturnsOldestFirstWithAssets(t *testing.T) {
	g, requests := newGiteaSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(giteaReleases))
	})

	releases, err := g.FetchReleases("octo", SourceRepository{Name: "repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := (*requests)[0]
	if req.Path != "/api/v1/repos/octo/repo/releases" {
		t.Errorf("expected the releases endpoint for octo/repo, got %s", req.Path)
	}
	if got := req.Header.Get("Authorization"); got != "token tok" {
		t.Errorf("expected the gitea token header, got %q", got)
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
	if len(releases[1].Assets) != 1 || releases[1].Assets[0].Name != "app-v2.zip" ||
		releases[1].Assets[0].URL != "https://gitea.example.com/octo/repo/releases/download/v2.0.0/app-v2.zip" {
		t.Errorf("expected one asset with its download url, got %+v", releases[1].Assets)
	}
}

func TestGitea_FetchReleases_NotFoundMeansNoReleases(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	releases, err := g.FetchReleases("octo", SourceRepository{Name: "repo"})
	if err != nil {
		t.Fatalf("expected a 404 to mean releases are unavailable, got error %v", err)
	}
	if releases != nil {
		t.Fatalf("expected nil releases, got %+v", releases)
	}
}

func TestGitea_FetchReleases_OtherFailuresAreErrors(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := g.FetchReleases("octo", SourceRepository{Name: "repo"}); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestGitea_GetWikiURL(t *testing.T) {
	withToken := NewGitea(configuration.ConfigSource{BaseURL: "https://gitea.example.com/", Auth: configuration.ConfigSourceAuth{Token: "tok"}})
	if got, want := withToken.GetWikiURL("octo", "repo"), "https://tok@gitea.example.com/octo/repo.wiki.git"; got != want {
		t.Errorf("with token: got %q, want %q", got, want)
	}

	withoutToken := NewGitea(configuration.ConfigSource{BaseURL: "https://gitea.example.com"})
	if got, want := withoutToken.GetWikiURL("octo", "repo"), "https://gitea.example.com/octo/repo.wiki.git"; got != want {
		t.Errorf("without token: got %q, want %q", got, want)
	}

	subPath := NewGitea(configuration.ConfigSource{BaseURL: "https://example.com/gitea"})
	if got, want := subPath.GetWikiURL("octo", "repo"), "https://example.com/gitea/octo/repo.wiki.git"; got != want {
		t.Errorf("sub path: got %q, want %q", got, want)
	}
}

func TestGitea_AddTokenToCloneUrl(t *testing.T) {
	withToken := NewGitea(configuration.ConfigSource{BaseURL: "https://gitea.example.com", Auth: configuration.ConfigSourceAuth{Token: "tok"}})
	if got, want := withToken.AddTokenToCloneUrl("https://gitea.example.com/octo/repo.git"), "https://tok@gitea.example.com/octo/repo.git"; got != want {
		t.Errorf("with token: got %q, want %q", got, want)
	}

	withoutToken := NewGitea(configuration.ConfigSource{BaseURL: "https://gitea.example.com"})
	if got, want := withoutToken.AddTokenToCloneUrl("https://gitea.example.com/octo/repo.git"), "https://gitea.example.com/octo/repo.git"; got != want {
		t.Errorf("without token: got %q, want %q", got, want)
	}
}

func TestGitea_FetchUsernamePassword_UsesTheTokenAsUsername(t *testing.T) {
	withToken := NewGitea(configuration.ConfigSource{BaseURL: "https://gitea.example.com", Auth: configuration.ConfigSourceAuth{Token: "tok"}})
	if u, p := withToken.FetchUsernamePassword(); u != "tok" || p != "" {
		t.Errorf("expected (tok, \"\"), got (%q, %q)", u, p)
	}

	withoutToken := NewGitea(configuration.ConfigSource{BaseURL: "https://gitea.example.com"})
	if u, p := withoutToken.FetchUsernamePassword(); u != "" || p != "" {
		t.Errorf("expected empty credentials, got (%q, %q)", u, p)
	}
}

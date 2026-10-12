package sources

import (
	"fmt"
	"main/src/configuration"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newGitlabSource(t *testing.T, token string, handler http.HandlerFunc) (*Gitlab, *[]recordedRequest) {
	t.Helper()

	server, requests := newRecordingServer(t, handler)
	g := NewGitlab(configuration.ConfigSource{
		ID:      "gitlab",
		BaseURL: server.URL,
		Auth:    configuration.ConfigSourceAuth{Token: token},
		Timeout: time.Minute,
	})

	return g, requests
}

// gitlabTree serves a fake group tree. subgroups and projects map a group id to the JSON returned
// for the first page of that group's subgroups/projects; every other page is empty.
func gitlabTree(groupName string, baseID int, subgroups, projects map[int]string) http.HandlerFunc {
	groupID := func(path string) int {
		parts := strings.Split(path, "/")
		id, _ := strconv.Atoi(parts[4])
		return id
	}

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		firstPage := r.URL.Query().Get("page") == "1"

		switch {
		case path == "/api/v4/groups/"+groupName:
			_, _ = fmt.Fprintf(w, `{"id": %d, "name": %q, "path": %q}`, baseID, groupName, groupName)
		case strings.HasSuffix(path, "/subgroups"):
			if body, ok := subgroups[groupID(path)]; ok && firstPage {
				_, _ = w.Write([]byte(body))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(path, "/projects"):
			if body, ok := projects[groupID(path)]; ok && firstPage {
				_, _ = w.Write([]byte(body))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestGitlab_Paginate_WalksBaseGroupThenSubgroups(t *testing.T) {
	g, requests := newGitlabSource(t, "tok", gitlabTree("grp", 10,
		map[int]string{
			10: `[{"id": 20, "name": "Sub", "path": "sub", "avatar_url": null}]`,
		},
		map[int]string{
			10: `[{"id": 101, "name": "Top", "description": "top repo", "avatar_url": "https://gitlab.example.com/a.png", "http_url_to_repo": "https://gitlab.example.com/grp/top.git", "visibility": "private", "archived": true, "empty_repo": false}]`,
			20: `[{"id": 201, "name": "Nested", "description": null, "avatar_url": null, "http_url_to_repo": "https://gitlab.example.com/grp/sub/nested.git", "visibility": "public", "archived": false, "empty_repo": true}]`,
		},
	))

	first, err := g.Paginate("grp", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := (*requests)[0]; got.Path != "/api/v4/groups/grp" || got.Header.Get("Private-Token") != "tok" {
		t.Errorf("expected the group lookup to carry the private token, got %+v", got)
	}
	if len(first.Repositories) != 1 {
		t.Fatalf("expected the base group's single repository first, got %d", len(first.Repositories))
	}
	top := first.Repositories[0]
	if top.ID != 101 || top.Name != "Top" || top.URL != "https://gitlab.example.com/grp/top.git" {
		t.Errorf("unexpected repository mapping: %+v", top)
	}
	if top.Description == nil || *top.Description != "top repo" || top.Avatar == nil || *top.Avatar != "https://gitlab.example.com/a.png" {
		t.Errorf("expected description and avatar to be mapped, got %+v", top)
	}
	if !top.Private || !top.Archived || top.Empty {
		t.Errorf("expected private, archived, non-empty; got %+v", top)
	}
	if len(top.ParentGroupPath) != 0 {
		t.Errorf("expected no parent path for a base group repository, got %+v", top.ParentGroupPath)
	}

	second, err := g.Paginate("grp", configuration.ConfigGroup{}, first)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(second.Repositories) != 1 {
		t.Fatalf("expected the subgroup's repository next, got %d", len(second.Repositories))
	}
	nested := second.Repositories[0]
	if nested.Name != "Nested" || nested.Private || nested.Archived || !nested.Empty || nested.Description != nil {
		t.Errorf("unexpected subgroup repository mapping: %+v", nested)
	}
	wantPath := []SourceRepositoryGroup{{Path: "sub", Name: "Sub"}}
	if !reflect.DeepEqual(nested.ParentGroupPath, wantPath) {
		t.Errorf("expected parent path %+v, got %+v", wantPath, nested.ParentGroupPath)
	}

	third, err := g.Paginate("grp", configuration.ConfigGroup{}, second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(third.Repositories) != 0 {
		t.Fatalf("expected no more repositories, got %+v", third.Repositories)
	}
}

func TestGitlab_Paginate_NestedSubgroupsCarryTheFullParentPath(t *testing.T) {
	g, _ := newGitlabSource(t, "tok", gitlabTree("grp", 10,
		map[int]string{
			10: `[{"id": 20, "name": "Sub", "path": "sub"}]`,
			20: `[{"id": 30, "name": "Deep", "path": "deep"}]`,
		},
		map[int]string{
			30: `[{"id": 301, "name": "Leaf", "http_url_to_repo": "https://gitlab.example.com/grp/sub/deep/leaf.git", "visibility": "public"}]`,
		},
	))

	res, err := g.Paginate("grp", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Repositories) != 1 || res.Repositories[0].Name != "Leaf" {
		t.Fatalf("expected the deeply nested repository, got %+v", res.Repositories)
	}
	wantPath := []SourceRepositoryGroup{{Path: "sub", Name: "Sub"}, {Path: "deep", Name: "Deep"}}
	if got := res.Repositories[0].ParentGroupPath; !reflect.DeepEqual(got, wantPath) {
		t.Fatalf("expected parent path %+v, got %+v", wantPath, got)
	}

	last, err := g.Paginate("grp", configuration.ConfigGroup{}, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(last.Repositories) != 0 {
		t.Fatalf("expected no more repositories, got %+v", last.Repositories)
	}
}

func TestGitlab_Paginate_GroupLookupFailureIsError(t *testing.T) {
	g, _ := newGitlabSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := g.Paginate("grp", configuration.ConfigGroup{}, nil); err == nil {
		t.Fatal("expected an error when the group cannot be resolved")
	}
}

const gitlabReleases = `[
  {"name": "v2", "tag_name": "v2.0.0", "description": "second", "created_at": "2024-02-01T00:00:00Z",
   "assets": {"count": 1, "links": [{"id": 1, "name": "app.zip", "direct_asset_url": "https://gitlab.example.com/grp/top/-/releases/v2.0.0/downloads/app.zip"}]}},
  {"name": "v1", "tag_name": "v1.0.0", "description": "first", "created_at": "2024-01-01T00:00:00Z",
   "assets": {"count": 0, "links": []}}
]`

func TestGitlab_FetchReleases_ReturnsOldestFirstWithAssets(t *testing.T) {
	g, requests := newGitlabSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(gitlabReleases))
	})

	releases, err := g.FetchReleases("grp", SourceRepository{ID: 55, Name: "top"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := (*requests)[0]
	if req.Path != "/api/v4/projects/55/releases" || req.Header.Get("Private-Token") != "tok" {
		t.Errorf("expected an authenticated request for project 55, got %+v", req)
	}
	if len(releases) != 2 || releases[0].TagName != "v1.0.0" || releases[1].TagName != "v2.0.0" {
		t.Fatalf("expected oldest release first, got %+v", releases)
	}
	if r := releases[0]; r.Name != "v1" || r.Description != "first" || r.CreatedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("unexpected release mapping: %+v", r)
	}
	wantAssets := []SourceAsset{{Name: "app.zip", URL: "https://gitlab.example.com/grp/top/-/releases/v2.0.0/downloads/app.zip"}}
	if !reflect.DeepEqual(releases[1].Assets, wantAssets) {
		t.Errorf("expected assets %+v, got %+v", wantAssets, releases[1].Assets)
	}
}

// GitLab's assets.count also counts the auto-generated source archives, which are not links.
func TestGitlab_FetchReleases_IgnoresSourceArchivesInAssetCount(t *testing.T) {
	const payload = `[
  {"name": "dav1d 1.4.2", "tag_name": "1.4.2", "description": "", "created_at": "2024-05-01T00:00:00Z",
   "assets": {
     "count": 5,
     "sources": [
       {"format": "zip", "url": "https://code.videolan.org/videolan/dav1d/-/archive/1.4.2/dav1d-1.4.2.zip"},
       {"format": "tar.gz", "url": "https://code.videolan.org/videolan/dav1d/-/archive/1.4.2/dav1d-1.4.2.tar.gz"},
       {"format": "tar.bz2", "url": "https://code.videolan.org/videolan/dav1d/-/archive/1.4.2/dav1d-1.4.2.tar.bz2"},
       {"format": "tar", "url": "https://code.videolan.org/videolan/dav1d/-/archive/1.4.2/dav1d-1.4.2.tar"}
     ],
     "links": [
       {"id": 13, "name": "Dav1d 1.4.2 Source tarball", "url": "http://downloads.videolan.org/videolan/dav1d/1.4.2/dav1d-1.4.2.tar.xz",
        "direct_asset_url": "http://downloads.videolan.org/videolan/dav1d/1.4.2/dav1d-1.4.2.tar.xz", "link_type": "package"}
     ]
   }}
]`
	g, _ := newGitlabSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	})

	releases, err := g.FetchReleases("videolan", SourceRepository{ID: 55, Name: "dav1d"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected one release, got %+v", releases)
	}

	want := []SourceAsset{{Name: "Dav1d 1.4.2 Source tarball", URL: "http://downloads.videolan.org/videolan/dav1d/1.4.2/dav1d-1.4.2.tar.xz"}}
	if got := releases[0].Assets; !reflect.DeepEqual(got, want) {
		t.Errorf("expected only the linked asset %+v, got %+v", want, got)
	}
}

func TestGitlab_FetchReleases_ForbiddenMeansNoReleases(t *testing.T) {
	g, _ := newGitlabSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	releases, err := g.FetchReleases("grp", SourceRepository{ID: 55})
	if err != nil || releases != nil {
		t.Fatalf("expected (nil, nil) for a forbidden project, got (%v, %v)", releases, err)
	}
}

func TestGitlab_FetchReleases_OtherFailuresAreErrors(t *testing.T) {
	g, _ := newGitlabSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := g.FetchReleases("grp", SourceRepository{ID: 55}); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestGitlab_GetWikiURL(t *testing.T) {
	g := NewGitlab(configuration.ConfigSource{BaseURL: "https://gitlab.example.com", Auth: configuration.ConfigSourceAuth{Token: "tok"}})

	want := "https://tok:@gitlab.example.com/grp/repo.wiki.git"
	if got := g.GetWikiURL("grp", "repo"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGitlab_AddTokenToCloneUrl(t *testing.T) {
	withToken := NewGitlab(configuration.ConfigSource{BaseURL: "https://gitlab.example.com", Auth: configuration.ConfigSourceAuth{Token: "tok"}})
	if got := withToken.AddTokenToCloneUrl("https://gitlab.example.com/grp/repo.git"); got != "https://oauth2:tok@gitlab.example.com/grp/repo.git" {
		t.Errorf("expected oauth2 credentials in the url, got %q", got)
	}

	withoutToken := NewGitlab(configuration.ConfigSource{BaseURL: "https://gitlab.example.com"})
	if got := withoutToken.AddTokenToCloneUrl("https://gitlab.example.com/grp/repo.git"); got != "https://gitlab.example.com/grp/repo.git" {
		t.Errorf("expected the url to be unchanged without a token, got %q", got)
	}
}

func TestGitlab_FetchUsernamePassword(t *testing.T) {
	withToken := NewGitlab(configuration.ConfigSource{BaseURL: "https://gitlab.example.com", Auth: configuration.ConfigSourceAuth{Token: "tok"}})
	if u, p := withToken.FetchUsernamePassword(); u != "oauth2" || p != "tok" {
		t.Errorf("expected (oauth2, tok), got (%q, %q)", u, p)
	}

	withoutToken := NewGitlab(configuration.ConfigSource{BaseURL: "https://gitlab.example.com"})
	if u, p := withoutToken.FetchUsernamePassword(); u != "" || p != "" {
		t.Errorf("expected empty credentials without a token, got (%q, %q)", u, p)
	}
}

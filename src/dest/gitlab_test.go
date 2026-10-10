package dest

import (
	"bytes"
	"fmt"
	"io"
	"main/src/configuration"
	"main/src/sources"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
)

func newFakeGitLab(t *testing.T, handler http.HandlerFunc) (*GitLab, *[]recordedCall) {
	t.Helper()

	server, calls := newRecordingServer(t, handler)
	g := NewGitLab(configuration.ConfigDestination{
		ID:      "gitlab",
		URL:     server.URL,
		Token:   "secret",
		Timeout: time.Minute,
	})

	return g, calls
}

func gitlabGroup() configuration.ConfigGroup {
	return configuration.ConfigGroup{Username: "octo", GiteaUsername: "other", GitlabGroupID: 1227}
}

func gitlabRemote() sources.SourceRepository {
	return sources.SourceRepository{
		Name:        "MyRepo",
		URL:         "https://github.com/octo/myrepo.git",
		Description: strPtr("desc"),
	}
}

func gitlabRepo() *Repository {
	return &Repository{
		ID:                "77",
		Name:              "MyRepo",
		HttpUrl:           "https://gitlab.local/grp/myrepo.git",
		PathWithNamespace: "grp/myrepo",
		Remote:            gitlabRemote(),
	}
}

const gitlabProjectJSON = `{"id": 77, "name": "myrepo", "http_url_to_repo": "https://gitlab.local/grp/myrepo.git", "path_with_namespace": "grp/myrepo", "import_status": "finished"}`

func TestGitLab_RequestsCarryPrivateToken(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := g.RetrieveExistingRepo(gitlabGroup(), gitlabRemote()); err != nil {
		t.Fatal(err)
	}
	if got := (*calls)[0].Header.Get("Private-Token"); got != "secret" {
		t.Fatalf("expected the private token header, got %q", got)
	}
}

func TestGitLab_GetIdentification_ExposesConfiguredDestination(t *testing.T) {
	g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {})

	id := g.GetIdentification()
	if id.ID != "gitlab" || id.Config == nil || id.Config.Token != "secret" {
		t.Fatalf("expected id gitlab with its config, got %+v", id)
	}
}

// --- RetrieveExistingRepo / parent groups ---

func TestGitLab_RetrieveExistingRepo_FindsProjectByNameCaseInsensitively(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/1227/projects" {
			respond(w, http.StatusOK, `[{"id": 5, "name": "other", "http_url_to_repo": "x", "path_with_namespace": "grp/other", "import_status": "finished"}, `+gitlabProjectJSON+`]`)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	})

	repo, err := g.RetrieveExistingRepo(gitlabGroup(), gitlabRemote())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Query.Get("search") != "MyRepo" || call.Query.Get("per_page") != "100" {
		t.Errorf("expected a search for MyRepo with per_page=100, got %v", call.Query)
	}
	if repo == nil || repo.ID != "77" || repo.Name != "myrepo" || repo.HttpUrl != "https://gitlab.local/grp/myrepo.git" || repo.PathWithNamespace != "grp/myrepo" {
		t.Fatalf("unexpected repository: %+v", repo)
	}
	if !repo.FinishedMiration || repo.Remote.Name != "MyRepo" {
		t.Errorf("expected a finished migration with the remote attached, got %+v", repo)
	}
}

func TestGitLab_RetrieveExistingRepo_ImportStatus(t *testing.T) {
	tests := []struct {
		status   string
		finished bool
	}{
		{"finished", true},
		{"none", true},
		{"started", false},
		{"scheduled", false},
		{"failed", false},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
				respond(w, http.StatusOK, `[`+strings.Replace(gitlabProjectJSON, "finished", tc.status, 1)+`]`)
			})

			repo, err := g.RetrieveExistingRepo(gitlabGroup(), gitlabRemote())
			if err != nil || repo == nil {
				t.Fatalf("expected a repository, got (%+v, %v)", repo, err)
			}
			if repo.FinishedMiration != tc.finished {
				t.Fatalf("import_status %q: expected finished=%v", tc.status, tc.finished)
			}
		})
	}
}

func TestGitLab_RetrieveExistingRepo_NoMatchReturnsNil(t *testing.T) {
	t.Run("empty search result", func(t *testing.T) {
		g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
			respond(w, http.StatusOK, `[{"id": 5, "name": "other", "http_url_to_repo": "x", "path_with_namespace": "grp/other", "import_status": "finished"}]`)
		})
		if repo, err := g.RetrieveExistingRepo(gitlabGroup(), gitlabRemote()); repo != nil || err != nil {
			t.Fatalf("expected (nil, nil), got (%+v, %v)", repo, err)
		}
	})

	t.Run("group not found", func(t *testing.T) {
		g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		if repo, err := g.RetrieveExistingRepo(gitlabGroup(), gitlabRemote()); repo != nil || err != nil {
			t.Fatalf("expected (nil, nil), got (%+v, %v)", repo, err)
		}
	})
}

func TestGitLab_RetrieveExistingRepo_SearchesInsideTheExistingSubgroup(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/1227/subgroups":
			respond(w, http.StatusOK, `[{"id": 39, "name": "Sub", "path": "sub-old"}, {"id": 40, "name": "Sub", "path": "sub"}]`)
		case "/api/v4/groups/40/projects":
			respond(w, http.StatusOK, `[`+gitlabProjectJSON+`]`)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	})

	remote := gitlabRemote()
	remote.ParentGroupPath = []sources.SourceRepositoryGroup{{Path: "sub", Name: "Sub"}}

	repo, err := g.RetrieveExistingRepo(gitlabGroup(), remote)
	if err != nil || repo == nil {
		t.Fatalf("expected the repository inside the subgroup, got (%+v, %v)", repo, err)
	}
	search := findCall(*calls, http.MethodGet, "/api/v4/groups/1227/subgroups")
	if search == nil || search.Query.Get("search") != "Sub" {
		t.Errorf("expected the subgroup to be searched by name, got %+v", *calls)
	}
	if n := countCalls(*calls, http.MethodPost); n != 0 {
		t.Errorf("expected no group to be created when it exists, got %d POSTs", n)
	}
}

func TestGitLab_RetrieveParentGroup_CreatesMissingSubgroupsInOrder(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/subgroups"):
			respond(w, http.StatusOK, `[]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups":
			switch r.URL.Query().Get("path") {
			case "sub":
				respond(w, http.StatusCreated, `{"id": 41, "name": "Sub", "path": "sub", "avatar_url": null}`)
			case "deep":
				respond(w, http.StatusCreated, `{"id": 42, "name": "Deep", "path": "deep", "avatar_url": null}`)
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	})

	remote := gitlabRemote()
	remote.ParentGroupPath = []sources.SourceRepositoryGroup{{Path: "sub", Name: "Sub"}, {Path: "deep", Name: "Deep"}}

	id, err := g.RetrieveParentGroup(remote, 1227)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("expected the innermost group id 42, got %d", id)
	}

	var posts []recordedCall
	for _, c := range *calls {
		if c.Method == http.MethodPost {
			posts = append(posts, c)
		}
	}
	if len(posts) != 2 {
		t.Fatalf("expected two groups to be created, got %d", len(posts))
	}
	if q := posts[0].Query; q.Get("name") != "Sub" || q.Get("path") != "sub" || q.Get("parent_id") != "1227" {
		t.Errorf("expected sub to be created under 1227, got %v", q)
	}
	if q := posts[1].Query; q.Get("name") != "Deep" || q.Get("path") != "deep" || q.Get("parent_id") != "41" {
		t.Errorf("expected deep to be created under the new sub group 41, got %v", q)
	}
}

func TestGitLab_CreateGroup_CopiesTheGroupAvatar(t *testing.T) {
	var serverURL, filename, content string
	g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/subgroups"):
			respond(w, http.StatusOK, `[]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups":
			respond(w, http.StatusCreated, fmt.Sprintf(`{"id": 41, "name": "Sub", "path": "sub", "avatar_url": "%s/uploads/avatar.png"}`, serverURL))
		case r.Method == http.MethodGet && r.URL.Path == "/uploads/avatar.png":
			respond(w, http.StatusOK, "PNGDATA")
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/41":
			file, header, err := r.FormFile("avatar")
			if err != nil {
				respond(w, http.StatusBadRequest, err.Error())
				return
			}
			data, _ := io.ReadAll(file)
			filename, content = header.Filename, string(data)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	})
	serverURL = g.URL.String()

	remote := gitlabRemote()
	remote.ParentGroupPath = []sources.SourceRepositoryGroup{{Path: "sub", Name: "Sub"}}

	id, err := g.RetrieveParentGroup(remote, 1227)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 41 {
		t.Errorf("expected group 41, got %d", id)
	}
	if filename != "avatar.png" || content != "PNGDATA" {
		t.Errorf("expected the downloaded avatar to be uploaded as avatar.png, got %q with %q", filename, content)
	}
}

// --- ImportRepository ---

func TestGitLab_ImportRepository_RejectsReservedAndInvalidNames(t *testing.T) {
	for _, name := range []string{"new", "tree", "-leading", "trailing-", ".hidden", "repo.git", "repo.atom", "bad/slash"} {
		t.Run(name, func(t *testing.T) {
			g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			})

			remote := gitlabRemote()
			remote.Name = name
			if _, err := g.ImportRepository(gitlabGroup(), configuration.ConfigRepo{}, remote, &stubSource{}); err == nil {
				t.Fatalf("expected %q to be rejected", name)
			}
			if len(*calls) != 0 {
				t.Fatalf("expected no API call for a rejected name, got %+v", *calls)
			}
		})
	}
}

func TestGitLab_ImportRepository_CreatesProjectWithImportUrl(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects" {
			respond(w, http.StatusCreated, gitlabProjectJSON)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	})

	repo, err := g.ImportRepository(gitlabGroup(), configuration.ConfigRepo{}, gitlabRemote(), &stubSource{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	post := findCall(*calls, http.MethodPost, "/api/v4/projects")
	if post == nil {
		t.Fatalf("expected POST /api/v4/projects, got %+v", *calls)
	}
	if post.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("expected a form request, got %q", post.ContentType)
	}
	form := formBody(t, *post)
	if form.Get("name") != "MyRepo" || form.Get("namespace_id") != "1227" || form.Get("description") != "desc" {
		t.Errorf("unexpected project fields: %v", form)
	}
	if got := form.Get("import_url"); got != "https://tok@github.com/octo/myrepo.git" {
		t.Errorf("expected the import url with the source token, got %q", got)
	}
	if repo.ID != "77" || repo.PathWithNamespace != "grp/myrepo" || repo.Remote.Name != "MyRepo" {
		t.Errorf("unexpected repository mapping: %+v", repo)
	}
}

func TestGitLab_ImportRepository_FailureIsError(t *testing.T) {
	g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusBadRequest, `{"message": "name already taken"}`)
	})

	if _, err := g.ImportRepository(gitlabGroup(), configuration.ConfigRepo{}, gitlabRemote(), &stubSource{}); err == nil {
		t.Fatal("expected an error")
	}
}

// --- LockUntilImport ---

func TestGitLab_LockUntilImport(t *testing.T) {
	t.Run("finished", func(t *testing.T) {
		g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
			respond(w, http.StatusOK, gitlabProjectJSON)
		})

		pinged := false
		if err := g.LockUntilImport(gitlabRepo(), func(string) { pinged = true }); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pinged {
			t.Error("expected no waiting for a finished import")
		}
		if (*calls)[0].Path != "/api/v4/projects/77" {
			t.Errorf("expected the project to be polled, got %s", (*calls)[0].Path)
		}
	})

	t.Run("failed", func(t *testing.T) {
		g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
			respond(w, http.StatusOK, strings.Replace(gitlabProjectJSON, "finished", "failed", 1))
		})

		if err := g.LockUntilImport(gitlabRepo(), func(string) {}); err == nil {
			t.Fatal("expected a failed import to be reported")
		}
	})
}

// --- variables / branches ---

func TestGitLab_SetOriginalUrl_CreatesProjectVariable(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	if err := g.SetOriginalUrl(gitlabRepo(), "https://github.com/octo/myrepo.git"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodPost || call.Path != "/api/v4/projects/77/variables" {
		t.Errorf("expected POST /api/v4/projects/77/variables, got %s %s", call.Method, call.Path)
	}
	form := formBody(t, call)
	if form.Get("key") != "original_url" || form.Get("value") != "https://github.com/octo/myrepo.git" {
		t.Errorf("unexpected variable payload: %v", form)
	}
}

func TestGitLab_GetProtectedBranches_ReturnsBranchNames(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, `[{"name": "main"}, {"name": "release"}]`)
	})

	branches, err := g.GetProtectedBranches(gitlabRepo())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(branches, ",") != "main,release" {
		t.Errorf("expected [main release], got %v", branches)
	}
	if (*calls)[0].Path != "/api/v4/projects/77/protected_branches" {
		t.Errorf("expected the protected branches endpoint, got %s", (*calls)[0].Path)
	}
}

func TestGitLab_UnprotectBranch_EscapesTheBranchName(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := g.UnprotectBranch(gitlabRepo(), "release/1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodDelete || call.RawPath != "/api/v4/projects/77/protected_branches/release%2F1" {
		t.Errorf("expected DELETE with an escaped branch name, got %s %s", call.Method, call.RawPath)
	}
}

func TestGitLab_AddRemoteToRepo_AddsAuthenticatedGitlabRemote(t *testing.T) {
	g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {})

	local, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	repo := gitlabRepo()
	repo.LocalRepository = local

	if err := g.AddRemoteToRepo(repo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	remote, err := local.Remote("gitlab")
	if err != nil {
		t.Fatalf("expected a remote named gitlab: %v", err)
	}
	if got := remote.Config().URLs[0]; got != "https://oauth2:secret@gitlab.local/grp/myrepo.git" {
		t.Errorf("expected the remote url to carry the token, got %q", got)
	}

	if err := g.AddRemoteToRepo(gitlabRepo()); err == nil {
		t.Error("expected an error without a local clone")
	}
}

// --- archive / avatar ---

func TestGitLab_ChangeArchivedState(t *testing.T) {
	tests := []struct {
		archived bool
		wantPath string
	}{
		{true, "/api/v4/projects/77/archive"},
		{false, "/api/v4/projects/77/unarchive"},
	}

	for _, tc := range tests {
		t.Run(tc.wantPath, func(t *testing.T) {
			g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			})

			if err := g.ChangeArchivedState(gitlabRepo(), tc.archived); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			call := (*calls)[0]
			if call.Method != http.MethodPost || call.Path != tc.wantPath {
				t.Errorf("expected POST %s, got %s %s", tc.wantPath, call.Method, call.Path)
			}
		})
	}

	failing, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	if err := failing.ChangeArchivedState(gitlabRepo(), true); err == nil {
		t.Error("expected an error for a 400")
	}
}

func TestGitLab_ChangeAvatar_UploadsMultipartAvatar(t *testing.T) {
	var filename, content string
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		file, header, err := r.FormFile("avatar")
		if err != nil {
			respond(w, http.StatusBadRequest, err.Error())
			return
		}
		data, _ := io.ReadAll(file)
		filename, content = header.Filename, string(data)
		w.WriteHeader(http.StatusOK)
	})

	if err := g.ChangeAvatar(gitlabRepo(), bytes.NewBufferString("PNGDATA"), "png"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodPut || call.Path != "/api/v4/projects/77" {
		t.Errorf("expected PUT /api/v4/projects/77, got %s %s", call.Method, call.Path)
	}
	if filename != "avatar.png" || content != "PNGDATA" {
		t.Errorf("expected avatar.png with the image bytes, got %q with %q", filename, content)
	}

	failing, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	if err := failing.ChangeAvatar(gitlabRepo(), bytes.NewBufferString("PNGDATA"), "png"); err == nil {
		t.Error("expected an error for a 400")
	}
}

// --- releases ---

func TestGitLab_ReleaseExists(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   bool
	}{{http.StatusOK, true}, {http.StatusNotFound, false}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			})

			got, err := g.ReleaseExists(gitlabRepo(), "v1.0.0")
			if err != nil || got != tc.want {
				t.Fatalf("expected %v, got (%v, %v)", tc.want, got, err)
			}
			if (*calls)[0].Path != "/api/v4/projects/77/releases/v1.0.0" {
				t.Errorf("expected the release to be looked up by tag, got %s", (*calls)[0].Path)
			}
		})
	}
}

func TestGitLab_CreateRelease_PostsReleaseFields(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	release := sources.SourceRelease{Name: "Version 1", TagName: "v1.0.0", Description: "notes", CreatedAt: "2024-01-01T00:00:00Z"}
	if err := g.CreateRelease(gitlabRepo(), release); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodPost || call.Path != "/api/v4/projects/77/releases" {
		t.Errorf("expected POST /api/v4/projects/77/releases, got %s %s", call.Method, call.Path)
	}
	form := formBody(t, call)
	if form.Get("name") != "Version 1" || form.Get("tag_name") != "v1.0.0" || form.Get("description") != "notes" || form.Get("released_at") != "2024-01-01T00:00:00Z" {
		t.Errorf("unexpected release payload: %v", form)
	}

	failing, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	if err := failing.CreateRelease(gitlabRepo(), release); err == nil {
		t.Error("expected an error for a 409")
	}
}

func TestGitLab_LinkAsset_CreatesAssetLink(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	if err := g.LinkAsset(gitlabRepo(), sources.SourceRelease{TagName: "v1.0.0"}, "app.zip", "https://dufs.local/app.zip"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodPost || call.Path != "/api/v4/projects/77/releases/v1.0.0/assets/links" {
		t.Errorf("expected POST to the release asset links, got %s %s", call.Method, call.Path)
	}
	if call.Query.Get("name") != "app.zip" || call.Query.Get("url") != "https://dufs.local/app.zip" {
		t.Errorf("expected the asset name and url as query parameters, got %v", call.Query)
	}

	failing, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	if err := failing.LinkAsset(gitlabRepo(), sources.SourceRelease{TagName: "v1.0.0"}, "app.zip", "u"); err == nil {
		t.Error("expected an error for a 400")
	}
}

// --- wiki ---

func TestGitLab_GetWikiProject(t *testing.T) {
	g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {})

	if wiki := g.GetWikiProject(gitlabRepo(), gitlabGroup(), &stubSource{}); wiki != nil {
		t.Fatalf("expected nil for a source without wiki, got %+v", wiki)
	}

	source := &stubSource{wikiURL: "https://tok:x@github.com/octo/myrepo.wiki.git"}
	wiki := g.GetWikiProject(gitlabRepo(), gitlabGroup(), source)
	if wiki == nil {
		t.Fatal("expected a wiki repository")
	}
	if len(source.wikiCalls) != 1 || source.wikiCalls[0] != [2]string{"octo", "MyRepo"} {
		t.Errorf("expected the wiki url to be resolved for the source username, got %v", source.wikiCalls)
	}
	if wiki.Name != "MyRepo.wiki" || wiki.Remote.URL != source.wikiURL || wiki.Remote.Name != "MyRepo.wiki" {
		t.Errorf("unexpected wiki repository: %+v", wiki)
	}
	if want := g.URL.String() + "/grp/myrepo.wiki.git"; wiki.HttpUrl != want {
		t.Errorf("expected wiki url %q, got %q", want, wiki.HttpUrl)
	}
	if wiki.OverrideRemoteBranch != "" {
		t.Errorf("expected gitlab wikis to keep their branch names, got override %q", wiki.OverrideRemoteBranch)
	}
}

func TestGitLab_CreateWiki_IsANoOp(t *testing.T) {
	g, calls := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {})

	if err := g.CreateWiki(gitlabRepo(), gitlabGroup()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("expected no requests, got %+v", *calls)
	}
}

func TestGitLab_UploadFile_IsNotSupported(t *testing.T) {
	g, _ := newFakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {})

	if _, err := g.UploadFile(bytes.NewBufferString("x"), "/x"); err == nil {
		t.Fatal("expected an error, gitlab does not store release assets")
	}
}

// --- name rules ---

func TestIsValidGitlabName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"my-repo", true},
		{"My Repo 2.0", true},
		{"under_score", true},
		{"-leading", false},
		{"trailing-", false},
		{".hidden", false},
		{"trailing.", false},
		{"repo.git", false},
		{"repo.atom", false},
		{"bad/slash", false},
		{"bad@char", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidGitlabName(tc.name); got != tc.want {
				t.Fatalf("isValidGitlabName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestIsReservedGitlabName(t *testing.T) {
	for _, name := range []string{"new", "tree", "raw", "wikis", "info/lfs/objects"} {
		if !isReservedGitlabName(name) {
			t.Errorf("expected %q to be reserved", name)
		}
	}
	for _, name := range []string{"newer", "my-tree", "repo"} {
		if isReservedGitlabName(name) {
			t.Errorf("expected %q not to be reserved", name)
		}
	}
}

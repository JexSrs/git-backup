package dest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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

// newFakeGitea starts a fake Gitea API server and returns a destination pointing at it,
// together with the list of calls the destination made.
func newFakeGitea(t *testing.T, createOwner bool, handler http.HandlerFunc) (*Gitea, *[]recordedCall) {
	t.Helper()

	server, calls := newRecordingServer(t, handler)
	g := NewGitea(configuration.ConfigDestination{
		ID:          "gitea",
		URL:         server.URL,
		Token:       "secret",
		Timeout:     time.Minute,
		CreateOwner: createOwner,
	})

	return g, calls
}

func giteaGroup() configuration.ConfigGroup {
	return configuration.ConfigGroup{Username: "octo", GiteaUsername: "owner"}
}

func giteaRemote() sources.SourceRepository {
	return sources.SourceRepository{
		Name:        "my repo",
		URL:         "https://github.com/octo/my-repo.git",
		Description: strPtr("desc"),
	}
}

func giteaRepo() *Repository {
	return &Repository{
		ID:                "12",
		Name:              "my-repo",
		HttpUrl:           "http://gitea.local/owner/my-repo.git",
		PathWithNamespace: "owner/my-repo",
		ConfigGroup:       giteaGroup(),
		Remote:            giteaRemote(),
	}
}

const giteaProjectJSON = `{"id": 12, "name": "my-repo", "html_url": "http://gitea.local/owner/my-repo", "full_name": "owner/my-repo", "empty": false}`

// --- EnsureOwner ---

func TestGiteaEnsureOwner_CreatesMissingOwnerAsOrganization(t *testing.T) {
	g, calls := newFakeGitea(t, true, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/freepbx":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 7, "username": "freepbx"}`))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	})

	created, err := g.EnsureOwner(configuration.ConfigGroup{Username: "freepbx", GiteaUsername: "freepbx"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("expected owner to be reported as created")
	}

	var post *recordedCall
	for i := range *calls {
		if (*calls)[i].Method == http.MethodPost {
			post = &(*calls)[i]
		}
	}
	if post == nil || post.Path != "/api/v1/orgs" {
		t.Fatalf("expected POST /api/v1/orgs, got calls: %+v", *calls)
	}

	var payload map[string]any
	if err := json.Unmarshal(post.Body, &payload); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if payload["username"] != "freepbx" {
		t.Fatalf("expected organization username freepbx, got %v", payload["username"])
	}
}

func existingOwnerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/asterisk" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 3, "login": "asterisk"}`))
		return
	}
	w.WriteHeader(http.StatusTeapot)
}

func TestGiteaEnsureOwner_LeavesExistingOwnerAlone(t *testing.T) {
	g, calls := newFakeGitea(t, true, existingOwnerHandler)

	created, err := g.EnsureOwner(configuration.ConfigGroup{Username: "asterisk", GiteaUsername: "asterisk"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Fatal("expected no owner to be created")
	}
	if n := countCalls(*calls, http.MethodPost); n != 0 {
		t.Fatalf("expected no POST calls, got %d: %+v", n, *calls)
	}
}

func TestGiteaEnsureOwner_DisabledMakesNoRequests(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	created, err := g.EnsureOwner(configuration.ConfigGroup{Username: "asterisk", GiteaUsername: "asterisk"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Fatal("expected no owner to be created when create_owner is disabled")
	}
	if len(*calls) != 0 {
		t.Fatalf("expected no API calls when create_owner is disabled, got %+v", *calls)
	}
}

func TestGiteaEnsureOwner_ChecksEachOwnerOnce(t *testing.T) {
	g, calls := newFakeGitea(t, true, existingOwnerHandler)
	group := configuration.ConfigGroup{Username: "asterisk", GiteaUsername: "asterisk"}

	for i := 0; i < 3; i++ {
		if _, err := g.EnsureOwner(group); err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
	}

	if n := countCalls(*calls, http.MethodGet); n != 1 {
		t.Fatalf("expected the owner to be looked up once, got %d lookups", n)
	}
}

func TestGiteaEnsureOwner_ReportsCreationFailure(t *testing.T) {
	g, _ := newFakeGitea(t, true, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message": "user is not allowed to create organizations"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	created, err := g.EnsureOwner(configuration.ConfigGroup{Username: "freepbx", GiteaUsername: "freepbx"})
	if err == nil {
		t.Fatal("expected an error when the organization cannot be created")
	}
	if created {
		t.Fatal("expected created to be false on failure")
	}
}

// --- request plumbing ---

func TestGitea_RequestsCarryTokenAuthorization(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := g.RetrieveExistingRepo(giteaGroup(), giteaRemote()); err != nil {
		t.Fatal(err)
	}
	call := (*calls)[0]
	if got := call.Header.Get("Authorization"); got != "token secret" {
		t.Errorf("expected 'token secret' authorization, got %q", got)
	}
	if got := call.Header.Get("Accept"); got != "*/*" {
		t.Errorf("expected Accept */*, got %q", got)
	}
}

func TestGitea_GetIdentification_ExposesConfiguredDestination(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {})

	id := g.GetIdentification()
	if id.ID != "gitea" || id.Config == nil || id.Config.Token != "secret" {
		t.Fatalf("expected id gitea with its config, got %+v", id)
	}
}

// --- RetrieveExistingRepo ---

func TestGitea_RetrieveExistingRepo_MapsProjectAndReplacesSpacesInName(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, giteaProjectJSON)
	})

	repo, err := g.RetrieveExistingRepo(giteaGroup(), giteaRemote())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := (*calls)[0]; got.Method != http.MethodGet || got.Path != "/api/v1/repos/owner/my-repo" {
		t.Errorf("expected GET /api/v1/repos/owner/my-repo, got %s %s", got.Method, got.Path)
	}
	if repo == nil {
		t.Fatal("expected a repository")
	}
	if repo.ID != "12" || repo.Name != "my-repo" || repo.HttpUrl != "http://gitea.local/owner/my-repo" || repo.PathWithNamespace != "owner/my-repo" {
		t.Errorf("unexpected repository mapping: %+v", repo)
	}
	if !repo.FinishedMiration {
		t.Errorf("expected a non-empty repository to count as a finished migration")
	}
	if repo.Remote.Name != "my repo" || repo.ConfigGroup.GiteaUsername != "owner" {
		t.Errorf("expected the remote and group to be attached, got %+v", repo)
	}
}

func TestGitea_RetrieveExistingRepo_EmptyRepositoryIsAnUnfinishedMigration(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, strings.Replace(giteaProjectJSON, `"empty": false`, `"empty": true`, 1))
	})

	repo, err := g.RetrieveExistingRepo(giteaGroup(), giteaRemote())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.FinishedMiration {
		t.Fatal("expected an empty repository to be reported as unfinished")
	}
}

func TestGitea_RetrieveExistingRepo_NotFoundReturnsNil(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	repo, err := g.RetrieveExistingRepo(giteaGroup(), giteaRemote())
	if err != nil || repo != nil {
		t.Fatalf("expected (nil, nil), got (%+v, %v)", repo, err)
	}
}

// --- ImportRepository ---

func TestGitea_ImportRepository_SendsMigrationRequest(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/migrate" {
			respond(w, http.StatusCreated, giteaProjectJSON)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	})

	lfs := true
	repo, err := g.ImportRepository(giteaGroup(), configuration.ConfigRepo{LFS: &lfs}, giteaRemote(), &stubSource{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	post := findCall(*calls, http.MethodPost, "/api/v1/repos/migrate")
	if post == nil {
		t.Fatalf("expected POST /api/v1/repos/migrate, got %+v", *calls)
	}
	if post.ContentType != "application/json" {
		t.Errorf("expected a JSON request, got %q", post.ContentType)
	}
	payload := jsonBody(t, *post)
	if payload["clone_addr"] != "https://tok@github.com/octo/my-repo.git" {
		t.Errorf("expected the clone address with the source token, got %v", payload["clone_addr"])
	}
	if payload["repo_name"] != "my-repo" || payload["repo_owner"] != "owner" {
		t.Errorf("expected repo my-repo under owner, got name=%v owner=%v", payload["repo_name"], payload["repo_owner"])
	}
	if payload["lfs"] != true || payload["description"] != "desc" || payload["service"] != "git" || payload["mirror"] != false {
		t.Errorf("unexpected migration options: %v", payload)
	}

	if repo.ID != "12" || repo.Name != "my-repo" || repo.PathWithNamespace != "owner/my-repo" {
		t.Errorf("unexpected repository mapping: %+v", repo)
	}
}

func TestGitea_ImportRepository_ServerErrorDeletesRepositoryAndFails(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusUnprocessableEntity} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					respond(w, status, "boom")
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			lfs := false
			_, err := g.ImportRepository(giteaGroup(), configuration.ConfigRepo{LFS: &lfs}, giteaRemote(), &stubSource{})
			if err == nil {
				t.Fatal("expected an error")
			}
			if del := findCall(*calls, http.MethodDelete, "/api/v1/repos/owner/my-repo"); del == nil {
				t.Fatalf("expected the half-created repository to be deleted, got %+v", *calls)
			}
		})
	}
}

func TestGitea_ImportRepository_OtherFailuresDoNotDelete(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusForbidden, "nope")
	})

	lfs := false
	if _, err := g.ImportRepository(giteaGroup(), configuration.ConfigRepo{LFS: &lfs}, giteaRemote(), &stubSource{}); err == nil {
		t.Fatal("expected an error")
	}
	if n := countCalls(*calls, http.MethodDelete); n != 0 {
		t.Fatalf("expected no delete for a 403, got %d", n)
	}
}

func TestGitea_ImportRepository_GatewayTimeoutFallsBackToLookup(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		respond(w, http.StatusOK, giteaProjectJSON)
	})

	lfs := false
	repo, err := g.ImportRepository(giteaGroup(), configuration.ConfigRepo{LFS: &lfs}, giteaRemote(), &stubSource{})
	if err != nil {
		t.Fatalf("expected the import to be looked up after a timeout, got error: %v", err)
	}
	if repo == nil || repo.ID != "12" {
		t.Fatalf("expected the existing repository, got %+v", repo)
	}
}

// --- LockUntilImport ---

func TestGitea_LockUntilImport_ReturnsOnceRepositoryHasContent(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, giteaProjectJSON)
	})

	pinged := false
	if err := g.LockUntilImport(giteaRepo(), func(string) { pinged = true }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pinged {
		t.Fatal("expected no waiting when the repository already has content")
	}
}

func TestGitea_LockUntilImport_NotFoundIsError(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if err := g.LockUntilImport(giteaRepo(), func(string) {}); err == nil {
		t.Fatal("expected an error when the repository disappears")
	}
}

// --- SetOriginalUrl ---

func TestGitea_SetOriginalUrl(t *testing.T) {
	tests := []struct {
		status  int
		wantErr bool
	}{
		{http.StatusCreated, false},
		{http.StatusNoContent, false},
		{http.StatusConflict, false}, // already set
		{http.StatusBadRequest, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			})

			err := g.SetOriginalUrl(giteaRepo(), "https://github.com/octo/my-repo.git")
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
			call := (*calls)[0]
			if call.Method != http.MethodPost || call.Path != "/api/v1/repos/owner/my-repo/actions/variables/original_url" {
				t.Errorf("expected POST to the original_url variable, got %s %s", call.Method, call.Path)
			}
			if got := jsonBody(t, call)["value"]; got != "https://github.com/octo/my-repo.git" {
				t.Errorf("expected the url as the variable value, got %v", got)
			}
		})
	}
}

// --- protected branches ---

func TestGitea_GetProtectedBranches_CollectsProtectedBranchesAcrossPages(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "0":
			respond(w, http.StatusOK, `[{"name": "main", "protected": true}, {"name": "dev", "protected": false}]`)
		case "1":
			respond(w, http.StatusOK, `[{"name": "release", "protected": true}]`)
		default:
			respond(w, http.StatusOK, `[]`)
		}
	})

	branches, err := g.GetProtectedBranches(giteaRepo())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(branches, ",") != "main,release" {
		t.Errorf("expected [main release], got %v", branches)
	}
	if len(*calls) != 3 || (*calls)[0].Path != "/api/v1/repos/owner/my-repo/branches" {
		t.Errorf("expected three page requests to the branches endpoint, got %+v", *calls)
	}
}

func TestGitea_UnprotectBranch(t *testing.T) {
	t.Run("deletes the branch protection", func(t *testing.T) {
		g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		if err := g.UnprotectBranch(giteaRepo(), "main"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		call := (*calls)[0]
		if call.Method != http.MethodDelete || call.Path != "/api/v1/repos/owner/my-repo/branch_protections/main" {
			t.Errorf("expected DELETE /api/v1/repos/owner/my-repo/branch_protections/main, got %s %s", call.Method, call.Path)
		}
	})

	t.Run("reports failures", func(t *testing.T) {
		g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		if err := g.UnprotectBranch(giteaRepo(), "main"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

// --- local git integration ---

func TestGitea_AddRemoteToRepo_AddsAuthenticatedRemoteNamedAfterDestination(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {})

	local, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	repo := giteaRepo()
	repo.LocalRepository = local

	if err := g.AddRemoteToRepo(repo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	remote, err := local.Remote("gitea")
	if err != nil {
		t.Fatalf("expected a remote named gitea: %v", err)
	}
	if got := remote.Config().URLs[0]; got != "http://oauth2:secret@gitea.local/owner/my-repo.git" {
		t.Errorf("expected the remote url to carry the token, got %q", got)
	}
}

func TestGitea_AddRemoteToRepo_RequiresLocalRepository(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {})

	if err := g.AddRemoteToRepo(giteaRepo()); err == nil {
		t.Fatal("expected an error without a local clone")
	}
}

// --- archive / avatar ---

func TestGitea_ChangeArchivedState(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, giteaProjectJSON)
	})

	if err := g.ChangeArchivedState(giteaRepo(), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodPatch || call.Path != "/api/v1/repos/owner/my-repo" {
		t.Errorf("expected PATCH /api/v1/repos/owner/my-repo, got %s %s", call.Method, call.Path)
	}
	if got := jsonBody(t, call)["archived"]; got != true {
		t.Errorf("expected archived=true, got %v", got)
	}

	failing, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if err := failing.ChangeArchivedState(giteaRepo(), false); err == nil {
		t.Error("expected an error for a 403")
	}
}

func TestGitea_ChangeAvatar_PostsBase64Image(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := g.ChangeAvatar(giteaRepo(), bytes.NewBufferString("PNGDATA"), "png"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodPost || call.Path != "/api/v1/repos/owner/my-repo/avatar" {
		t.Errorf("expected POST to the avatar endpoint, got %s %s", call.Method, call.Path)
	}
	if got := jsonBody(t, call)["image"]; got != base64.StdEncoding.EncodeToString([]byte("PNGDATA")) {
		t.Errorf("expected the image to be base64 encoded, got %v", got)
	}

	failing, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := failing.ChangeAvatar(giteaRepo(), bytes.NewBufferString("PNGDATA"), "png"); err == nil {
		t.Error("expected an error for a 500")
	}
}

// --- releases ---

func TestGitea_ReleaseExists(t *testing.T) {
	tests := []struct {
		status  int
		want    bool
		wantErr bool
	}{
		{http.StatusOK, true, false},
		{http.StatusNotFound, false, false},
		{http.StatusInternalServerError, false, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
				respond(w, tc.status, `{"id": 99, "name": "v1", "tag_name": "v1.0.0"}`)
			})

			got, err := g.ReleaseExists(giteaRepo(), "v1.0.0")
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("expected (%v, err=%v), got (%v, %v)", tc.want, tc.wantErr, got, err)
			}
			if path := (*calls)[0].Path; path != "/api/v1/repos/owner/my-repo/releases/tags/v1.0.0" {
				t.Errorf("expected the release to be looked up by tag, got %s", path)
			}
		})
	}
}

func TestGitea_CreateRelease_PostsReleaseFields(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	release := sources.SourceRelease{Name: "Version 1", TagName: "v1.0.0", Description: "notes"}
	if err := g.CreateRelease(giteaRepo(), release); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := (*calls)[0]
	if call.Method != http.MethodPost || call.Path != "/api/v1/repos/owner/my-repo/releases" {
		t.Errorf("expected POST to the releases endpoint, got %s %s", call.Method, call.Path)
	}
	payload := jsonBody(t, call)
	if payload["name"] != "Version 1" || payload["tag_name"] != "v1.0.0" || payload["body"] != "notes" {
		t.Errorf("unexpected release payload: %v", payload)
	}
	if payload["draft"] != false || payload["prerelease"] != false {
		t.Errorf("expected a published, non-prerelease release, got %v", payload)
	}

	failing, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	if err := failing.CreateRelease(giteaRepo(), release); err == nil {
		t.Error("expected an error for a 409")
	}
}

func TestGitea_LinkAsset_UploadsTheUrlAsATextAttachment(t *testing.T) {
	var filename, content string
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releases/tags/v1.0.0"):
			respond(w, http.StatusOK, `{"id": 99, "name": "v1", "tag_name": "v1.0.0"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/my-repo/releases/99/assets":
			file, header, err := r.FormFile("attachment")
			if err != nil {
				respond(w, http.StatusBadRequest, err.Error())
				return
			}
			data, _ := io.ReadAll(file)
			filename, content = header.Filename, string(data)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	})

	release := sources.SourceRelease{TagName: "v1.0.0"}
	if err := g.LinkAsset(giteaRepo(), release, "app.zip", "https://dufs.local/app.zip"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	post := findCall(*calls, http.MethodPost, "/api/v1/repos/owner/my-repo/releases/99/assets")
	if post == nil {
		t.Fatalf("expected the asset to be posted to release 99, got %+v", *calls)
	}
	if got := post.Query.Get("name"); got != "app.zip.txt" {
		t.Errorf("expected the attachment to be named app.zip.txt, got %q", got)
	}
	if filename != "app.zip.txt" || content != "https://dufs.local/app.zip" {
		t.Errorf("expected a text file containing the asset url, got %q with content %q", filename, content)
	}
}

func TestGitea_LinkAsset_EscapesAssetNamesWithSpaces(t *testing.T) {
	var filename string
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releases/tags/1.4.2"):
			respond(w, http.StatusOK, `{"id": 99, "name": "dav1d 1.4.2", "tag_name": "1.4.2"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/my-repo/releases/99/assets":
			_, header, err := r.FormFile("attachment")
			if err != nil {
				respond(w, http.StatusBadRequest, err.Error())
				return
			}
			filename = header.Filename
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	})

	release := sources.SourceRelease{TagName: "1.4.2"}
	name := "Dav1d 1.4.2 Source tarball"
	if err := g.LinkAsset(giteaRepo(), release, name, "http://downloads.videolan.org/videolan/dav1d/1.4.2/dav1d-1.4.2.tar.xz"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	post := findCall(*calls, http.MethodPost, "/api/v1/repos/owner/my-repo/releases/99/assets")
	if post == nil {
		t.Fatalf("expected the asset to be posted to release 99, got %+v", *calls)
	}
	if got := post.Query.Get("name"); got != name+".txt" {
		t.Errorf("expected the attachment name to survive the query string, got %q", got)
	}
	if filename != name+".txt" {
		t.Errorf("expected the multipart filename to keep the spaces, got %q", filename)
	}
}

func TestGitea_LinkAsset_ReportsRejectionWithStatusAndBody(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			respond(w, http.StatusOK, `{"id": 99, "name": "v1", "tag_name": "v1.0.0"}`)
			return
		}
		respond(w, http.StatusBadRequest, `{"message":"invalid external url"}`)
	})

	err := g.LinkAsset(giteaRepo(), sources.SourceRelease{TagName: "v1.0.0"}, "app.zip", "u")
	if err == nil {
		t.Fatal("expected an error for a 400")
	}
	msg := err.Error()
	if !strings.Contains(msg, "400") || !strings.Contains(msg, "invalid external url") {
		t.Errorf("expected the status and response body in the error, got %q", msg)
	}
	if strings.Contains(msg, "create release") {
		t.Errorf("linking an asset must not be reported as creating a release, got %q", msg)
	}
}

func TestGitea_LinkAsset_FailsWhenReleaseIsMissing(t *testing.T) {
	g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if err := g.LinkAsset(giteaRepo(), sources.SourceRelease{TagName: "v1.0.0"}, "app.zip", "u"); err == nil {
		t.Fatal("expected an error when the release does not exist")
	}
	if n := countCalls(*calls, http.MethodPost); n != 0 {
		t.Fatalf("expected no upload without a release, got %d", n)
	}
}

// --- wiki ---

func TestGitea_GetWikiProject(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {})

	t.Run("source without wiki", func(t *testing.T) {
		if wiki := g.GetWikiProject(giteaRepo(), giteaGroup(), &stubSource{}); wiki != nil {
			t.Fatalf("expected nil, got %+v", wiki)
		}
	})

	t.Run("source with wiki", func(t *testing.T) {
		source := &stubSource{wikiURL: "https://tok:x@github.com/octo/my-repo.wiki.git"}
		wiki := g.GetWikiProject(giteaRepo(), giteaGroup(), source)
		if wiki == nil {
			t.Fatal("expected a wiki repository")
		}
		if len(source.wikiCalls) != 1 || source.wikiCalls[0] != [2]string{"owner", "my repo"} {
			t.Errorf("expected the wiki url to be resolved for the gitea owner and remote name, got %v", source.wikiCalls)
		}
		if wiki.Name != "my-repo.wiki" || wiki.Remote.Name != "my repo.wiki" || wiki.Remote.URL != source.wikiURL {
			t.Errorf("unexpected wiki repository: %+v", wiki)
		}
		if want := g.URL.String() + "/owner/my-repo.wiki.git"; wiki.HttpUrl != want {
			t.Errorf("expected wiki url %q, got %q", want, wiki.HttpUrl)
		}
		if wiki.OverrideRemoteBranch != "main" {
			t.Errorf("expected wiki branches to be pushed to main, got %q", wiki.OverrideRemoteBranch)
		}
	})
}

func TestGitea_CreateWiki(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"created", http.StatusCreated, `{}`, false},
		{"already exists", http.StatusBadRequest, `{"message": "wiki page already exists"}`, false},
		{"other failure", http.StatusInternalServerError, `{"message": "internal error"}`, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, calls := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {
				respond(w, tc.status, tc.body)
			})

			err := g.CreateWiki(giteaRepo(), giteaGroup())
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
			call := (*calls)[0]
			if call.Method != http.MethodPost || call.Path != "/api/v1/repos/owner/my-repo/wiki/new" {
				t.Errorf("expected POST /api/v1/repos/owner/my-repo/wiki/new, got %s %s", call.Method, call.Path)
			}
		})
	}
}

func TestGitea_UploadFile_IsNotSupported(t *testing.T) {
	g, _ := newFakeGitea(t, false, func(w http.ResponseWriter, r *http.Request) {})

	if _, err := g.UploadFile(bytes.NewBufferString("x"), "/x"); err == nil {
		t.Fatal("expected an error, gitea does not store release assets")
	}
}

package dest

import (
	"bytes"
	"main/src/configuration"
	"main/src/sources"
	"net/http"
	"strings"
	"testing"
	"time"
)

func newFakeDufs(t *testing.T, handler http.HandlerFunc) (*Dufs, *[]recordedCall) {
	t.Helper()

	server, calls := newRecordingServer(t, handler)
	d := NewDufs(configuration.ConfigDestination{ID: "dufs", URL: server.URL, Timeout: time.Minute})
	return d, calls
}

func TestDufs_UploadFile_PutsContentAndReturnsItsPublicURL(t *testing.T) {
	d, calls := newFakeDufs(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	got, err := d.UploadFile(bytes.NewBufferString("payload"), "/gitea/repositories/repo_1/tag_v1.0.0/app.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := d.URL.String() + "/gitea/repositories/repo_1/tag_v1.0.0/app.zip"
	if got != want {
		t.Errorf("expected public url %q, got %q", want, got)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected exactly one request, got %d", len(*calls))
	}
	call := (*calls)[0]
	if call.Method != http.MethodPut || call.Path != "/gitea/repositories/repo_1/tag_v1.0.0/app.zip" {
		t.Errorf("expected PUT to the asset path, got %s %s", call.Method, call.Path)
	}
	if string(call.Body) != "payload" {
		t.Errorf("expected the buffer to be uploaded as the body, got %q", call.Body)
	}
}

func TestDufs_UploadFile_NonSuccessStatusIsError(t *testing.T) {
	d, _ := newFakeDufs(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusForbidden, "denied")
	})

	_, err := d.UploadFile(bytes.NewBufferString("payload"), "/x")
	if err == nil {
		t.Fatal("expected an error for a 403 response")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected the status and body in the error, got %q", err)
	}
}

func TestDufs_UploadFile_UnreachableServerIsError(t *testing.T) {
	d, _ := newFakeDufs(t, func(w http.ResponseWriter, r *http.Request) {})
	d.client.Timeout = time.Second
	d.URL.Host = "127.0.0.1:1"

	if _, err := d.UploadFile(bytes.NewBufferString("payload"), "/x"); err == nil {
		t.Fatal("expected an error when the server cannot be reached")
	}
}

func TestDufs_GetIdentification_ExposesConfiguredDestination(t *testing.T) {
	d, _ := newFakeDufs(t, func(w http.ResponseWriter, r *http.Request) {})

	id := d.GetIdentification()
	if id.ID != "dufs" {
		t.Errorf("expected id dufs, got %q", id.ID)
	}
	if id.Config == nil || id.Config.URL != d.URL.String() {
		t.Errorf("expected the destination config to be exposed, got %+v", id.Config)
	}
}

func TestDufs_EnsureOwner_IsANoOp(t *testing.T) {
	d, calls := newFakeDufs(t, func(w http.ResponseWriter, r *http.Request) {})

	created, err := d.EnsureOwner(configuration.ConfigGroup{Username: "octo"})
	if err != nil || created {
		t.Fatalf("expected (false, nil), got (%v, %v)", created, err)
	}
	if len(*calls) != 0 {
		t.Fatalf("expected no requests, got %+v", *calls)
	}
}

func TestDufs_RepositoryOperationsAreNotSupported(t *testing.T) {
	d, _ := newFakeDufs(t, func(w http.ResponseWriter, r *http.Request) {})
	repo := &Repository{ID: "1"}
	group := configuration.ConfigGroup{}

	checks := map[string]func() error{
		"RetrieveExistingRepo": func() error { _, err := d.RetrieveExistingRepo(group, sources.SourceRepository{}); return err },
		"ImportRepository": func() error {
			_, err := d.ImportRepository(group, configuration.ConfigRepo{}, sources.SourceRepository{}, &stubSource{})
			return err
		},
		"LockUntilImport":      func() error { return d.LockUntilImport(repo, func(string) {}) },
		"SetOriginalUrl":       func() error { return d.SetOriginalUrl(repo, "u") },
		"GetProtectedBranches": func() error { _, err := d.GetProtectedBranches(repo); return err },
		"UnprotectBranch":      func() error { return d.UnprotectBranch(repo, "main") },
		"AddRemoteToRepo":      func() error { return d.AddRemoteToRepo(repo) },
		"ChangeArchivedState":  func() error { return d.ChangeArchivedState(repo, true) },
		"ChangeAvatar":         func() error { return d.ChangeAvatar(repo, bytes.NewBuffer(nil), "png") },
		"ReleaseExists":        func() error { _, err := d.ReleaseExists(repo, "v1"); return err },
		"CreateRelease":        func() error { return d.CreateRelease(repo, sources.SourceRelease{}) },
		"LinkAsset":            func() error { return d.LinkAsset(repo, sources.SourceRelease{}, "a", "u") },
		"CreateWiki":           func() error { return d.CreateWiki(repo, group) },
	}

	for name, call := range checks {
		if err := call(); err == nil {
			t.Errorf("%s: expected an error, DUFS cannot host repositories", name)
		}
	}
	if wiki := d.GetWikiProject(repo, group, &stubSource{wikiURL: "x"}); wiki != nil {
		t.Errorf("expected no wiki project from DUFS, got %+v", wiki)
	}
}

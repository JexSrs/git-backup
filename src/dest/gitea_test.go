package dest

import (
	"encoding/json"
	"io"
	"main/src/configuration"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordedCall struct {
	Method string
	Path   string
	Body   []byte
}

// newFakeGitea starts a fake Gitea API server and returns a destination pointing at it,
// together with the list of calls the destination made.
func newFakeGitea(t *testing.T, createOwner bool, handler http.HandlerFunc) (*Gitea, *[]recordedCall) {
	t.Helper()

	calls := make([]recordedCall, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, recordedCall{Method: r.Method, Path: r.URL.Path, Body: body})
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	g := NewGitea(configuration.ConfigDestination{
		ID:          "gitea",
		URL:         server.URL,
		Token:       "secret",
		CreateOwner: createOwner,
	})

	return g, &calls
}

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

func countCalls(calls []recordedCall, method string) int {
	n := 0
	for _, c := range calls {
		if c.Method == method {
			n++
		}
	}
	return n
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

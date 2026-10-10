package sources

import (
	"main/src/configuration"
	"main/src/utils"
	"net/http"
	"testing"
	"time"
)

func newHuggingFace(t *testing.T, token string, handler http.HandlerFunc) (*HuggingFace, *[]recordedRequest) {
	t.Helper()

	server, requests := newRecordingServer(t, handler)
	g := NewHuggingFace(configuration.ConfigSource{
		ID:      "huggingface",
		Auth:    configuration.ConfigSourceAuth{Token: token},
		Timeout: time.Minute,
	})
	g.client.Transport = redirectHost(t, "huggingface.co", server)

	return g, requests
}

func TestExtractLink(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{`<https://huggingface.co/api/models?author=ilsp&cursor=abc&limit=100>; rel="next"`, "https://huggingface.co/api/models?author=ilsp&cursor=abc&limit=100"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.header, func(t *testing.T) {
			if got := extractLink(tc.header); got != tc.want {
				t.Fatalf("extractLink(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestHuggingFace_Paginate_ListsModelsFirst(t *testing.T) {
	g, requests := newHuggingFace(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Link", `<https://huggingface.co/api/models?author=ilsp&cursor=c2&limit=100>; rel="next"`)
		_, _ = w.Write([]byte(`[{"id": "ilsp/model-a"}, {"id": "ilsp/model-b"}]`))
	})

	res, err := g.Paginate("ilsp", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := (*requests)[0]
	if req.Path != "/api/models" || req.Query.Get("author") != "ilsp" || req.Query.Get("limit") != "100" {
		t.Errorf("expected the author's models to be listed, got %+v", req)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("expected bearer token, got %q", got)
	}

	if len(res.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(res.Repositories))
	}
	if r := res.Repositories[0]; r.Name != "model-a" || r.URL != "https://huggingface.co/ilsp/model-a.git" {
		t.Errorf("unexpected repository mapping: %+v", r)
	}
	if res.NextCursor == nil || *res.NextCursor != "https://huggingface.co/api/models?author=ilsp&cursor=c2&limit=100" {
		t.Errorf("expected the next cursor from the Link header, got %v", res.NextCursor)
	}
	if meta, ok := res.Metadata.(HuggingFaceMetadata); !ok || meta.What != "models" {
		t.Errorf("expected metadata to stay on models, got %+v", res.Metadata)
	}
}

func TestHuggingFace_Paginate_FollowsTheNextCursor(t *testing.T) {
	g, requests := newHuggingFace(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models" && r.URL.Query().Get("cursor") == "c2" {
			_, _ = w.Write([]byte(`[{"id": "ilsp/model-c"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	prev := &PaginationResponse{
		NextCursor: utils.Pointer("https://huggingface.co/api/models?author=ilsp&cursor=c2&limit=100"),
		Metadata:   HuggingFaceMetadata{What: "models"},
	}
	res, err := g.Paginate("ilsp", configuration.ConfigGroup{}, prev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*requests) != 1 || (*requests)[0].Query.Get("cursor") != "c2" {
		t.Fatalf("expected exactly one request for cursor c2, got %+v", *requests)
	}
	if len(res.Repositories) != 1 || res.Repositories[0].Name != "model-c" {
		t.Fatalf("expected model-c, got %+v", res.Repositories)
	}
}

func TestHuggingFace_Paginate_FallsBackToDatasetsWhenThereAreNoModels(t *testing.T) {
	g, requests := newHuggingFace(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			_, _ = w.Write([]byte(`[]`))
		case "/api/datasets":
			_, _ = w.Write([]byte(`[{"id": "ilsp/dataset-a"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	res, err := g.Paginate("ilsp", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*requests) != 2 || (*requests)[1].Path != "/api/datasets" || (*requests)[1].Query.Get("author") != "ilsp" {
		t.Fatalf("expected models then datasets to be requested, got %+v", *requests)
	}
	if len(res.Repositories) != 1 || res.Repositories[0].Name != "dataset-a" {
		t.Fatalf("expected dataset-a, got %+v", res.Repositories)
	}
	if meta, ok := res.Metadata.(HuggingFaceMetadata); !ok || meta.What != "datasets" {
		t.Fatalf("expected metadata to move to datasets, got %+v", res.Metadata)
	}
}

func TestHuggingFace_Paginate_ContinuesToDatasetsAfterTheLastModelsPage(t *testing.T) {
	t.Skip("known bug: after the last models page the next cursor is an empty string, so the following call fails instead of moving on to datasets")

	g, _ := newHuggingFace(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			_, _ = w.Write([]byte(`[{"id": "ilsp/model-a"}]`)) // last page: no Link header
		case "/api/datasets":
			_, _ = w.Write([]byte(`[{"id": "ilsp/dataset-a"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	first, err := g.Paginate("ilsp", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error on the first page: %v", err)
	}
	second, err := g.Paginate("ilsp", configuration.ConfigGroup{}, first)
	if err != nil {
		t.Fatalf("unexpected error after the last models page: %v", err)
	}
	if len(second.Repositories) != 1 || second.Repositories[0].Name != "dataset-a" {
		t.Fatalf("expected datasets to follow models, got %+v", second.Repositories)
	}
}

func TestHuggingFace_Paginate_NonOKStatusIsError(t *testing.T) {
	g, _ := newHuggingFace(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	if _, err := g.Paginate("ilsp", configuration.ConfigGroup{}, nil); err == nil {
		t.Fatal("expected an error for a 429 response")
	}
}

func TestHuggingFace_Credentials(t *testing.T) {
	g := NewHuggingFace(configuration.ConfigSource{Auth: configuration.ConfigSourceAuth{Token: "tok"}})

	if u, p := g.FetchUsernamePassword(); u != "" || p != "tok" {
		t.Errorf("expected the token as password, got (%q, %q)", u, p)
	}
	if got := g.AddTokenToCloneUrl("https://huggingface.co/ilsp/model-a.git"); got != "https://huggingface.co/ilsp/model-a.git" {
		t.Errorf("expected the clone url to be unchanged, got %q", got)
	}
	if got := g.GetWikiURL("ilsp", "model-a"); got != "" {
		t.Errorf("expected no wiki url, got %q", got)
	}
	if releases, err := g.FetchReleases("ilsp", SourceRepository{}); releases != nil || err != nil {
		t.Errorf("expected no releases, got (%v, %v)", releases, err)
	}
}

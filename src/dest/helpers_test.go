package dest

import (
	"bytes"
	"encoding/json"
	"io"
	"main/src/configuration"
	"main/src/sources"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type recordedCall struct {
	Method      string
	Path        string // decoded path
	RawPath     string // path as sent on the wire
	Query       url.Values
	Header      http.Header
	ContentType string
	Body        []byte
}

// newRecordingServer starts an HTTP server that records every request (including its body) before
// delegating to handler. The body is restored so handlers can still parse it.
func newRecordingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *[]recordedCall) {
	t.Helper()

	calls := make([]recordedCall, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))

		calls = append(calls, recordedCall{
			Method:      r.Method,
			Path:        r.URL.Path,
			RawPath:     r.URL.EscapedPath(),
			Query:       r.URL.Query(),
			Header:      r.Header.Clone(),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		})
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	return server, &calls
}

func respond(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
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

func findCall(calls []recordedCall, method, path string) *recordedCall {
	for i := range calls {
		if calls[i].Method == method && calls[i].Path == path {
			return &calls[i]
		}
	}
	return nil
}

func jsonBody(t *testing.T, call recordedCall) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(call.Body, &payload); err != nil {
		t.Fatalf("invalid JSON body %q: %v", call.Body, err)
	}
	return payload
}

func formBody(t *testing.T, call recordedCall) url.Values {
	t.Helper()

	values, err := url.ParseQuery(string(call.Body))
	if err != nil {
		t.Fatalf("invalid form body %q: %v", call.Body, err)
	}
	return values
}

func strPtr(s string) *string { return &s }

// stubSource is a minimal sources.Source for destination tests.
type stubSource struct {
	wikiURL   string
	username  string
	password  string
	wikiCalls [][2]string
}

func (s *stubSource) Paginate(string, configuration.ConfigGroup, *sources.PaginationResponse) (*sources.PaginationResponse, error) {
	return nil, nil
}

func (s *stubSource) GetWikiURL(username, repoName string) string {
	s.wikiCalls = append(s.wikiCalls, [2]string{username, repoName})
	return s.wikiURL
}

func (s *stubSource) FetchReleases(string, sources.SourceRepository) ([]sources.SourceRelease, error) {
	return nil, nil
}

// AddTokenToCloneUrl inserts a fixed "tok@" credential so tests can check the destination forwards it.
func (s *stubSource) AddTokenToCloneUrl(u string) string {
	return strings.Replace(u, "://", "://tok@", 1)
}

func (s *stubSource) FetchUsernamePassword() (string, string) {
	return s.username, s.password
}

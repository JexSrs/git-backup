package sources

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
}

// newRecordingServer starts an HTTP server that records every request before delegating to handler.
func newRecordingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *[]recordedRequest) {
	t.Helper()

	requests := make([]recordedRequest, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Header: r.Header.Clone(),
		})
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	return server, &requests
}

// hostRewriter is an http.RoundTripper that delivers requests addressed to allowedHost to a local
// test server instead, and rejects requests to any other host so tests can never reach the network.
type hostRewriter struct {
	allowedHost string
	target      *url.URL
	next        http.RoundTripper
}

func (h hostRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != h.allowedHost {
		return nil, fmt.Errorf("unexpected request to host %q (%s)", req.URL.Host, req.URL.String())
	}

	clone := req.Clone(req.Context())
	clone.URL.Scheme = h.target.Scheme
	clone.URL.Host = h.target.Host
	clone.Host = h.target.Host
	return h.next.RoundTrip(clone)
}

func redirectHost(t *testing.T, host string, server *httptest.Server) http.RoundTripper {
	t.Helper()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return hostRewriter{allowedHost: host, target: target, next: http.DefaultTransport}
}

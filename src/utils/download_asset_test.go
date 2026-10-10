package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadAsset_ReturnsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/app.zip" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ZIPDATA"))
	}))
	t.Cleanup(server.Close)

	buf, err := DownloadAsset(server.URL + "/releases/app.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "ZIPDATA" {
		t.Fatalf("expected body ZIPDATA, got %q", buf.String())
	}
}

func TestDownloadAsset_NonSuccessStatusIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	if _, err := DownloadAsset(server.URL + "/missing"); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestDownloadAsset_UnreachableServerIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	if _, err := DownloadAsset(url + "/x"); err == nil {
		t.Fatal("expected an error when the server is unreachable")
	}
}

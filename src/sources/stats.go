package sources

import (
	"net/http"
	"strconv"
)

// headerCount reads a numeric total from a response header (GitLab's X-Total, Gitea's X-Total-Count);
// nil when the header is missing or not a number, which the callers treat as unknown.
func headerCount(h http.Header, name string) *int {
	raw := h.Get(name)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

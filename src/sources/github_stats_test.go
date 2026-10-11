package sources

import (
	"encoding/json"
	"errors"
	"main/src/configuration"
	"net/http"
	"testing"
)

const githubStatsJSON = `{"data":{"repository":{
  "releases":{"totalCount":95},
  "defaultBranchRef":{"target":{"history":{"totalCount":4036}}},
  "issuesAll":{"totalCount":900},"issuesOpen":{"totalCount":250},"issuesClosed":{"totalCount":650},
  "prsAll":{"totalCount":1519},"prsOpen":{"totalCount":137},"prsClosed":{"totalCount":1382}}}}`

func TestGithub_FetchStats_AsksGraphQLOnceForEveryCount(t *testing.T) {
	var body map[string]any
	g, requests := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(githubStatsJSON))
	})

	stats, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Releases: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("expected a single GraphQL request, got %d", len(*requests))
	}
	req := (*requests)[0]
	if req.Method != http.MethodPost || req.Path != "/graphql" {
		t.Errorf("expected POST /graphql, got %s %s", req.Method, req.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("expected bearer token, got %q", got)
	}
	vars, _ := body["variables"].(map[string]any)
	if vars["owner"] != "octo" || vars["name"] != "repo" {
		t.Errorf("expected owner/name variables, got %v", vars)
	}

	want := map[string]struct {
		got  *int
		want int
	}{
		"releases":      {stats.Releases, 95},
		"commits":       {stats.Commits, 4036},
		"issues all":    {stats.Issues.All, 900},
		"issues open":   {stats.Issues.Open, 250},
		"issues closed": {stats.Issues.Closed, 650},
		"prs all":       {stats.PullRequests.All, 1519},
		"prs open":      {stats.PullRequests.Open, 137},
		"prs closed":    {stats.PullRequests.Closed, 1382},
	}
	for name, c := range want {
		if c.got == nil || *c.got != c.want {
			t.Errorf("%s = %v, want %d", name, c.got, c.want)
		}
	}
}

func TestGithub_FetchStats_EmptyRepositoryHasZeroCommits(t *testing.T) {
	g, _ := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"releases":{"totalCount":0},"defaultBranchRef":null,
		  "issuesAll":{"totalCount":0},"issuesOpen":{"totalCount":0},"issuesClosed":{"totalCount":0},
		  "prsAll":{"totalCount":0},"prsOpen":{"totalCount":0},"prsClosed":{"totalCount":0}}}}`))
	})

	stats, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Commits: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Commits == nil || *stats.Commits != 0 {
		t.Fatalf("expected 0 commits without a default branch, got %v", stats.Commits)
	}
}

func TestGithub_FetchStats_WithoutTokenIsUnavailableAndMakesNoRequest(t *testing.T) {
	g, requests := newGithub(t, "", func(w http.ResponseWriter, r *http.Request) {})

	_, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Releases: true})
	if !errors.Is(err, ErrStatsUnavailable) {
		t.Fatalf("expected ErrStatsUnavailable, got %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("expected no request without a token, got %d", len(*requests))
	}
}

func TestGithub_FetchStats_NothingNeededMakesNoRequest(t *testing.T) {
	g, requests := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {})

	known := 7
	stats, err := g.FetchStats("octo", SourceRepository{Name: "repo", Stats: RepositoryStats{Releases: &known}}, StatsNeeds{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("expected no request when nothing is needed, got %d", len(*requests))
	}
	if stats.Releases == nil || *stats.Releases != 7 {
		t.Fatalf("expected the known stats to be returned unchanged, got %+v", stats)
	}
}

func TestGithub_FetchStats_GraphQLErrorsAreErrors(t *testing.T) {
	g, _ := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":null},"errors":[{"message":"Could not resolve to a Repository"}]}`))
	})

	if _, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Releases: true}); err == nil {
		t.Fatal("expected an error when GraphQL reports errors")
	}
}

func TestGithub_FetchStats_NonOKStatusIsError(t *testing.T) {
	g, _ := newGithub(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Releases: true}); err == nil {
		t.Fatal("expected an error for a 401 response")
	}
}

var _ = configuration.ConfigGroup{} // keep the import used if helpers change

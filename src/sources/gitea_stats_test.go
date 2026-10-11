package sources

import (
	"net/http"
	"strings"
	"testing"

	"main/src/configuration"
)

const giteaReposPageWithCounters = `[
  {"name": "alpha", "clone_url": "https://gitea.example.com/octo/alpha.git", "description": "", "avatar_url": "",
   "private": false, "archived": false, "empty": false, "fork": false,
   "open_issues_count": 1552, "open_pr_counter": 143, "release_counter": 119}
]`

func TestGitea_Paginate_PrefillsCountersFromTheListing(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", giteaOwner(strp(giteaReposPageWithCounters), nil))

	res, err := g.Paginate("octo", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := res.Repositories[0].Stats
	if s.Issues.Open == nil || *s.Issues.Open != 1552 {
		t.Errorf("open issues = %v, want 1552", s.Issues.Open)
	}
	if s.PullRequests.Open == nil || *s.PullRequests.Open != 143 {
		t.Errorf("open prs = %v, want 143", s.PullRequests.Open)
	}
	if s.Releases == nil || *s.Releases != 119 {
		t.Errorf("releases = %v, want 119", s.Releases)
	}
	if s.Commits != nil || s.Issues.Closed != nil || s.PullRequests.Closed != nil {
		t.Errorf("expected counts the listing lacks to stay unknown, got %+v", s)
	}
}

// giteaStatsServer answers the per-repository count endpoints with X-Total-Count headers.
func giteaStatsServer(emptyRepo bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case strings.HasSuffix(r.URL.Path, "/repos/octo/repo/issues") && q.Get("type") == "issues":
			w.Header().Set("X-Total-Count", map[string]string{"open": "1552", "closed": "2863"}[q.Get("state")])
		case strings.HasSuffix(r.URL.Path, "/repos/octo/repo/issues") && q.Get("type") == "pulls":
			w.Header().Set("X-Total-Count", map[string]string{"open": "143", "closed": "10073"}[q.Get("state")])
		case strings.HasSuffix(r.URL.Path, "/repos/octo/repo/commits"):
			if emptyRepo {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.Header().Set("X-Total-Count", "25951")
		case strings.HasSuffix(r.URL.Path, "/repos/octo/repo/releases"):
			w.Header().Set("X-Total-Count", "119")
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}
}

func TestGitea_FetchStats_CompletesTheListingCountersWithOneRequestEach(t *testing.T) {
	g, requests := newGiteaSource(t, "tok", giteaStatsServer(false))
	open := func(n int) *int { return &n }
	repo := SourceRepository{Name: "repo", Stats: RepositoryStats{
		Releases:     open(119),
		Issues:       StateCounts{Open: open(1552)},
		PullRequests: StateCounts{Open: open(143)},
	}}

	stats, err := g.FetchStats("octo", repo, allNeeds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// closed issues, closed pulls, commits: the rest was already known.
	if len(*requests) != 3 {
		t.Fatalf("expected 3 requests, got %d: %+v", len(*requests), *requests)
	}
	for _, req := range *requests {
		if req.Query.Get("limit") != "1" {
			t.Errorf("%s: expected limit=1 to keep the body tiny, got %v", req.Path, req.Query)
		}
		if got := req.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("%s: expected the token header, got %q", req.Path, got)
		}
	}
	want := map[string]struct {
		got  *int
		want int
	}{
		"releases":      {stats.Releases, 119},
		"commits":       {stats.Commits, 25951},
		"issues open":   {stats.Issues.Open, 1552},
		"issues closed": {stats.Issues.Closed, 2863},
		"issues all":    {stats.Issues.All, 1552 + 2863},
		"prs open":      {stats.PullRequests.Open, 143},
		"prs closed":    {stats.PullRequests.Closed, 10073},
		"prs all":       {stats.PullRequests.All, 143 + 10073},
	}
	for name, c := range want {
		if c.got == nil || *c.got != c.want {
			t.Errorf("%s = %v, want %d", name, c.got, c.want)
		}
	}
}

func TestGitea_FetchStats_FetchesOpenCountsWhenTheListingDidNotProvideThem(t *testing.T) {
	g, requests := newGiteaSource(t, "tok", giteaStatsServer(false))

	stats, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Issues: StateNeeds{All: true}, Releases: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*requests) != 3 { // open issues, closed issues, releases
		t.Fatalf("expected 3 requests, got %d: %+v", len(*requests), *requests)
	}
	if stats.Issues.All == nil || *stats.Issues.All != 1552+2863 || stats.Releases == nil || *stats.Releases != 119 {
		t.Fatalf("unexpected stats %+v", stats)
	}
	if stats.PullRequests.Closed != nil || stats.Commits != nil {
		t.Fatalf("expected unrequested counts to stay unknown, got %+v", stats)
	}
}

func TestGitea_FetchStats_EmptyRepositoryHasZeroCommits(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", giteaStatsServer(true))

	stats, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Commits: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Commits == nil || *stats.Commits != 0 {
		t.Fatalf("expected 0 commits for a 409 (empty repository), got %v", stats.Commits)
	}
}

func TestGitea_FetchStats_NonOKStatusIsError(t *testing.T) {
	g, _ := newGiteaSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := g.FetchStats("octo", SourceRepository{Name: "repo"}, StatsNeeds{Commits: true}); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

package sources

import (
	"main/src/configuration"
	"net/http"
	"strings"
	"testing"
)

// gitlabStatsServer answers the endpoints FetchStats uses for project 42.
func gitlabStatsServer(withTotals, withStatistics bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects/42/issues_statistics"):
			_, _ = w.Write([]byte(`{"statistics":{"counts":{"all":9469,"closed":6889,"opened":2580}}}`))
		case strings.HasSuffix(r.URL.Path, "/projects/42/merge_requests"):
			if withTotals {
				if r.URL.Query().Get("state") == "opened" {
					w.Header().Set("X-Total", "129")
				} else {
					w.Header().Set("X-Total", "7321")
				}
			}
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/projects/42/releases"):
			if withTotals {
				w.Header().Set("X-Total", "285")
			}
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/projects/42"):
			if withStatistics {
				_, _ = w.Write([]byte(`{"id":42,"statistics":{"commit_count":12345}}`))
			} else {
				_, _ = w.Write([]byte(`{"id":42}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func allNeeds() StatsNeeds {
	return StatsNeeds{
		Releases:     true,
		Commits:      true,
		Issues:       StateNeeds{All: true, Open: true, Closed: true},
		PullRequests: StateNeeds{All: true, Open: true, Closed: true},
	}
}

func TestGitlab_FetchStats_ReadsEveryCount(t *testing.T) {
	g, requests := newGitlabSource(t, "tok", gitlabStatsServer(true, true))

	stats, err := g.FetchStats("grp", SourceRepository{ID: 42, Name: "repo"}, allNeeds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, req := range *requests {
		if got := req.Header.Get("Private-Token"); got != "tok" {
			t.Errorf("%s: expected the private token header, got %q", req.Path, got)
		}
	}
	want := map[string]struct {
		got  *int
		want int
	}{
		"releases":      {stats.Releases, 285},
		"commits":       {stats.Commits, 12345},
		"issues all":    {stats.Issues.All, 9469},
		"issues open":   {stats.Issues.Open, 2580},
		"issues closed": {stats.Issues.Closed, 6889},
		"mrs all":       {stats.PullRequests.All, 7321},
		"mrs open":      {stats.PullRequests.Open, 129},
		"mrs closed":    {stats.PullRequests.Closed, 7321 - 129},
	}
	for name, c := range want {
		if c.got == nil || *c.got != c.want {
			t.Errorf("%s = %v, want %d", name, c.got, c.want)
		}
	}
}

func TestGitlab_FetchStats_OnlyRequestsWhatIsNeeded(t *testing.T) {
	g, requests := newGitlabSource(t, "tok", gitlabStatsServer(true, true))

	stats, err := g.FetchStats("grp", SourceRepository{ID: 42, Name: "repo"}, StatsNeeds{Releases: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*requests) != 1 || !strings.HasSuffix((*requests)[0].Path, "/projects/42/releases") {
		t.Fatalf("expected a single releases request, got %+v", *requests)
	}
	if stats.Releases == nil || *stats.Releases != 285 || stats.Commits != nil || stats.Issues.All != nil {
		t.Fatalf("expected only releases to be filled, got %+v", stats)
	}
}

func TestGitlab_FetchStats_MissingTotalsAndStatisticsStayUnknown(t *testing.T) {
	g, _ := newGitlabSource(t, "tok", gitlabStatsServer(false, false))

	stats, err := g.FetchStats("grp", SourceRepository{ID: 42, Name: "repo"}, allNeeds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Releases != nil || stats.PullRequests.All != nil || stats.PullRequests.Open != nil || stats.PullRequests.Closed != nil {
		t.Errorf("expected releases and merge requests to stay unknown without X-Total, got %+v", stats)
	}
	if stats.Commits != nil {
		t.Errorf("expected commits to stay unknown without statistics, got %v", *stats.Commits)
	}
	if stats.Issues.All == nil || *stats.Issues.All != 9469 {
		t.Errorf("expected issue statistics to still be read, got %+v", stats.Issues)
	}
}

func TestGitlab_FetchStats_NonOKStatusIsError(t *testing.T) {
	g, _ := newGitlabSource(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := g.FetchStats("grp", SourceRepository{ID: 42, Name: "repo"}, StatsNeeds{Releases: true}); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestGitlab_Paginate_PrefillsOpenIssuesFromTheListing(t *testing.T) {
	g, _ := newGitlabSource(t, "tok", gitlabTree("grp", 10, map[int]string{}, map[int]string{
		10: `[{"id": 42, "name": "repo", "http_url_to_repo": "https://gitlab.local/grp/repo.git", "visibility": "public", "open_issues_count": 17}]`,
	}))

	res, err := g.Paginate("grp", configuration.ConfigGroup{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(res.Repositories))
	}
	if got := res.Repositories[0].Stats.Issues.Open; got == nil || *got != 17 {
		t.Fatalf("expected open issues 17 from the listing, got %v", got)
	}
}

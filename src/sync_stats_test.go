package main

import (
	"errors"
	"main/src/configuration"
	"main/src/sources"
	"reflect"
	"strings"
	"testing"
)

func bounds(min, max *int) configuration.ConfigFilterMinMax[int] {
	return configuration.ConfigFilterMinMax[int]{Min: min, Max: max}
}

func TestStatsNeeded_BoundsThatCannotExcludeNeedNothing(t *testing.T) {
	var f configuration.ConfigFilter
	if statsNeeded(f).Any() {
		t.Error("an empty filter should need nothing")
	}
	f.Releases = bounds(ptr(0), nil)
	f.Issues.Open = bounds(ptr(0), nil)
	if statsNeeded(f).Any() {
		t.Error("min 0 without max cannot exclude anything and should need nothing")
	}
}

func TestStatsNeeded_MapsEveryFilterToItsStatistic(t *testing.T) {
	cases := []struct {
		name string
		set  func(f *configuration.ConfigFilter)
		want sources.StatsNeeds
	}{
		{"releases min", func(f *configuration.ConfigFilter) { f.Releases = bounds(ptr(1), nil) }, sources.StatsNeeds{Releases: true}},
		{"commits max", func(f *configuration.ConfigFilter) { f.Commits = bounds(nil, ptr(10)) }, sources.StatsNeeds{Commits: true}},
		{"issues all", func(f *configuration.ConfigFilter) { f.Issues.All = bounds(ptr(2), nil) }, sources.StatsNeeds{Issues: sources.StateNeeds{All: true}}},
		{"issues open", func(f *configuration.ConfigFilter) { f.Issues.Open = bounds(nil, ptr(0)) }, sources.StatsNeeds{Issues: sources.StateNeeds{Open: true}}},
		{"issues closed", func(f *configuration.ConfigFilter) { f.Issues.Closed = bounds(ptr(5), ptr(9)) }, sources.StatsNeeds{Issues: sources.StateNeeds{Closed: true}}},
		{"prs all", func(f *configuration.ConfigFilter) { f.PullRequests.All = bounds(ptr(1), nil) }, sources.StatsNeeds{PullRequests: sources.StateNeeds{All: true}}},
		{"prs open", func(f *configuration.ConfigFilter) { f.PullRequests.Open = bounds(nil, ptr(3)) }, sources.StatsNeeds{PullRequests: sources.StateNeeds{Open: true}}},
		{"prs closed", func(f *configuration.ConfigFilter) { f.PullRequests.Closed = bounds(ptr(1), nil) }, sources.StatsNeeds{PullRequests: sources.StateNeeds{Closed: true}}},
	}
	for _, tc := range cases {
		var f configuration.ConfigFilter
		tc.set(&f)
		if got := statsNeeded(f); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestStatsFilterReason(t *testing.T) {
	n := func(v int) *int { return &v }
	cases := []struct {
		name   string
		stats  sources.RepositoryStats
		filter func(f *configuration.ConfigFilter)
		want   string // substring of the reason, "" means the repository passes
	}{
		{"within bounds passes", sources.RepositoryStats{Releases: n(5)}, func(f *configuration.ConfigFilter) { f.Releases = bounds(n(1), n(10)) }, ""},
		{"on the bounds passes", sources.RepositoryStats{Releases: n(10)}, func(f *configuration.ConfigFilter) { f.Releases = bounds(n(10), n(10)) }, ""},
		{"below min", sources.RepositoryStats{Releases: n(0)}, func(f *configuration.ConfigFilter) { f.Releases = bounds(n(1), nil) }, "releases 0 below min 1"},
		{"above max", sources.RepositoryStats{Commits: n(5000)}, func(f *configuration.ConfigFilter) { f.Commits = bounds(nil, n(100)) }, "commits 5000 above max 100"},
		{"unknown count never excludes", sources.RepositoryStats{}, func(f *configuration.ConfigFilter) { f.Releases = bounds(n(1), nil); f.Commits = bounds(nil, n(1)) }, ""},
		{"issues all", sources.RepositoryStats{Issues: sources.StateCounts{All: n(3)}}, func(f *configuration.ConfigFilter) { f.Issues.All = bounds(n(4), nil) }, "issues 3 below min 4"},
		{"issues open", sources.RepositoryStats{Issues: sources.StateCounts{Open: n(30)}}, func(f *configuration.ConfigFilter) { f.Issues.Open = bounds(nil, n(20)) }, "open issues 30 above max 20"},
		{"issues closed", sources.RepositoryStats{Issues: sources.StateCounts{Closed: n(0)}}, func(f *configuration.ConfigFilter) { f.Issues.Closed = bounds(n(1), nil) }, "closed issues 0 below min 1"},
		{"prs all", sources.RepositoryStats{PullRequests: sources.StateCounts{All: n(0)}}, func(f *configuration.ConfigFilter) { f.PullRequests.All = bounds(n(1), nil) }, "pull requests 0 below min 1"},
		{"prs open", sources.RepositoryStats{PullRequests: sources.StateCounts{Open: n(9)}}, func(f *configuration.ConfigFilter) { f.PullRequests.Open = bounds(nil, n(8)) }, "open pull requests 9 above max 8"},
		{"prs closed", sources.RepositoryStats{PullRequests: sources.StateCounts{Closed: n(2)}}, func(f *configuration.ConfigFilter) { f.PullRequests.Closed = bounds(n(3), nil) }, "closed pull requests 2 below min 3"},
	}
	for _, tc := range cases {
		var f configuration.ConfigFilter
		tc.filter(&f)
		got := statsFilterReason(tc.stats, f)
		if tc.want == "" && got != "" {
			t.Errorf("%s: expected to pass, got %q", tc.name, got)
		}
		if tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%s: expected reason containing %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestSyncUser_SkipsRepositoriesOutsideTheCountFilters(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta", "gamma"})
	zero, three := 0, 3
	f.source.stats = map[string]sources.RepositoryStats{
		"alpha": {Releases: &zero},
		"beta":  {Releases: &three},
		// gamma: unknown release count → not excluded
	}
	group := groupConfig()
	group.Filter.Releases = bounds(ptr(1), nil)

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"beta", "gamma"}) {
		t.Fatalf("expected alpha to be filtered out and gamma kept (unknown count), got %v", got)
	}
	if !reflect.DeepEqual(f.source.statsCalls, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("expected statistics to be fetched for every candidate, got %v", f.source.statsCalls)
	}
}

func TestSyncUser_DoesNotFetchStatsWithoutCountFilters(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha"})

	SyncUser(f.dsts, groupConfig(), f.source)

	if len(f.source.statsCalls) != 0 {
		t.Fatalf("expected no statistics lookups without count filters, got %v", f.source.statsCalls)
	}
}

func TestSyncUser_FetchesStatsAfterTheCheapFiltersRejected(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta"})
	group := groupConfig()
	group.Exclude = []string{"alpha"}
	group.Filter.Commits = bounds(ptr(1), nil)

	SyncUser(f.dsts, group, f.source)

	if !reflect.DeepEqual(f.source.statsCalls, []string{"beta"}) {
		t.Fatalf("expected statistics only for repositories that passed the cheap filters, got %v", f.source.statsCalls)
	}
}

func TestSyncUser_SyncsRepositoriesWhoseStatsCannotBeRead(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha"})
	f.source.statsErr = errors.New("api down")
	group := groupConfig()
	group.Filter.Releases = bounds(ptr(1), nil)

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("expected the repository to be synced when its statistics cannot be read, got %v", got)
	}
}

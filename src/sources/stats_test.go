package sources

import "testing"

func TestStatsNeeds_Any(t *testing.T) {
	if (StatsNeeds{}).Any() {
		t.Error("empty needs should not be any")
	}
	cases := map[string]StatsNeeds{
		"releases":   {Releases: true},
		"commits":    {Commits: true},
		"issues":     {Issues: StateNeeds{Closed: true}},
		"pull reqs":  {PullRequests: StateNeeds{Open: true}},
		"issues all": {Issues: StateNeeds{All: true}},
	}
	for name, n := range cases {
		if !n.Any() {
			t.Errorf("%s: expected Any() to be true", name)
		}
	}
}

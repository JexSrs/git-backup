package configuration

import (
	"encoding/json"
	"testing"
)

func TestCountFilters_DecodeFromJSON(t *testing.T) {
	raw := `{"filter": {
		"releases": {"min": 1, "max": 50},
		"commits": {"min": 10},
		"issues": {"all": {"max": 100}, "open": {"min": 0}, "closed": {"min": 5}},
		"pull_requests": {"all": {"min": 1}, "open": {"max": 20}, "closed": {"min": 2, "max": 300}}
	}}`

	var c Configuration
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f := c.Filter
	check := func(name string, m ConfigFilterMinMax[int], min, max *int) {
		t.Helper()
		if (m.Min == nil) != (min == nil) || (m.Min != nil && *m.Min != *min) {
			t.Errorf("%s.min = %v, want %v", name, m.Min, min)
		}
		if (m.Max == nil) != (max == nil) || (m.Max != nil && *m.Max != *max) {
			t.Errorf("%s.max = %v, want %v", name, m.Max, max)
		}
	}
	check("releases", f.Releases, intPtr(1), intPtr(50))
	check("commits", f.Commits, intPtr(10), nil)
	check("issues.all", f.Issues.All, nil, intPtr(100))
	check("issues.open", f.Issues.Open, intPtr(0), nil)
	check("issues.closed", f.Issues.Closed, intPtr(5), nil)
	check("pull_requests.all", f.PullRequests.All, intPtr(1), nil)
	check("pull_requests.open", f.PullRequests.Open, nil, intPtr(20))
	check("pull_requests.closed", f.PullRequests.Closed, intPtr(2), intPtr(300))
}

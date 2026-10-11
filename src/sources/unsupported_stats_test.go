package sources

import (
	"errors"
	"main/src/configuration"
	"testing"
)

func TestHuggingFaceAndGit_FetchStats_AreUnavailable(t *testing.T) {
	known := 3
	repo := SourceRepository{Name: "repo", Stats: RepositoryStats{Releases: &known}}

	for name, source := range map[string]Source{
		"huggingface": NewHuggingFace(configuration.ConfigSource{}),
		"git":         NewGit(configuration.ConfigSource{}),
	} {
		stats, err := source.FetchStats("octo", repo, StatsNeeds{Releases: true, Commits: true})
		if !errors.Is(err, ErrStatsUnavailable) {
			t.Errorf("%s: expected ErrStatsUnavailable, got %v", name, err)
		}
		if stats.Releases == nil || *stats.Releases != 3 {
			t.Errorf("%s: expected known stats to be returned unchanged, got %+v", name, stats)
		}
	}
}

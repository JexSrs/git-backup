package main

import (
	"errors"
	"main/src/configuration"
	"reflect"
	"testing"
	"time"
)

// recordSleeps replaces the retry sleep for the test and returns the waits it was asked for.
func recordSleeps(t *testing.T) *[]time.Duration {
	t.Helper()
	slept := []time.Duration{}
	previous := retrySleep
	retrySleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { retrySleep = previous })
	return &slept
}

func TestSyncUser_RetriesAFailedRepositorySyncAfterBackingOff(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha"})
	f.git.onceErrs["ImportRepository"] = errors.New("422 migration failed: could not connect to github.com")
	group := groupConfig()
	group.Config.Retries = ptr(2)
	slept := recordSleeps(t)

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha", "alpha"}) {
		t.Fatalf("expected the import to be attempted twice, got %v", got)
	}
	if want := []time.Duration{2 * time.Second}; !reflect.DeepEqual(*slept, want) {
		t.Fatalf("expected a single 2s backoff before the retry, slept %v", *slept)
	}
}

func TestSyncUser_RetriesAFailedPageFetchWithGrowingBackoff(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha"})
	f.source.failOnPage = 1
	f.source.failPageTimes = 2
	group := groupConfig()
	group.Config.Retries = ptr(3)
	slept := recordSleeps(t)

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("expected alpha to be synced once the page fetch succeeded, got %v", got)
	}
	if want := []time.Duration{2 * time.Second, 5 * time.Second}; !reflect.DeepEqual(*slept, want) {
		t.Fatalf("expected backoff of 2s then 5s, slept %v", *slept)
	}
}

func TestSyncUser_DoesNotRetryWhenNoRetriesAreConfigured(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha"})
	f.source.failOnPage = 1
	f.source.failPageTimes = 1
	slept := recordSleeps(t)

	SyncUser(f.dsts, groupConfig(), f.source) // groupConfig() leaves retries unset

	if len(f.git.calls) != 0 {
		t.Fatalf("expected nothing to be synced without retries, got %v", f.git.calls)
	}
	if len(*slept) != 0 {
		t.Fatalf("expected no backoff without retries, slept %v", *slept)
	}
}

func TestSyncUser_UsesTheRepositorySpecificRetries(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta"})
	f.git.onceErrs["ImportRepository"] = errors.New("transient failure")
	group := groupConfig() // no retries at group level
	alpha := repoConfig("gitea", "dufs")
	alpha.Retries = ptr(1)
	group.Repositories = []configuration.ConfigRepository{{Name: "alpha", ConfigRepo: alpha}}
	slept := recordSleeps(t)

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha", "alpha", "beta"}) {
		t.Fatalf("expected alpha to be retried once and beta to follow, got %v", got)
	}
	if want := []time.Duration{2 * time.Second}; !reflect.DeepEqual(*slept, want) {
		t.Fatalf("expected a single 2s backoff, slept %v", *slept)
	}
}

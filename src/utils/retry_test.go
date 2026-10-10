package utils

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestBackoff_StartsAtTwoThenFiveThenTenSecondsAndDoublesUpToTwoMinutes(t *testing.T) {
	want := []time.Duration{
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
		40 * time.Second,
		80 * time.Second,
		2 * time.Minute,
		2 * time.Minute,
	}
	for i, w := range want {
		attempt := i + 1
		if got := Backoff(attempt); got != w {
			t.Errorf("Backoff(%d) = %s, want %s", attempt, got, w)
		}
	}
}

func TestBackoff_TreatsNonPositiveAttemptsAsTheFirst(t *testing.T) {
	if got := Backoff(0); got != 2*time.Second {
		t.Fatalf("Backoff(0) = %s, want 2s", got)
	}
}

// retryRecorder captures what Retry does between attempts so tests never actually sleep.
type retryRecorder struct {
	slept    []time.Duration
	attempts []int
	waits    []time.Duration
	errs     []error
}

func (r *retryRecorder) sleep(d time.Duration) { r.slept = append(r.slept, d) }

func (r *retryRecorder) onRetry(attempt int, wait time.Duration, err error) {
	r.attempts = append(r.attempts, attempt)
	r.waits = append(r.waits, wait)
	r.errs = append(r.errs, err)
}

func TestRetry_SucceedingOnTheFirstAttemptRunsOnceAndNeverWaits(t *testing.T) {
	rec := &retryRecorder{}
	calls := 0

	err := Retry(3, rec.sleep, rec.onRetry, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if len(rec.slept) != 0 || len(rec.attempts) != 0 {
		t.Fatalf("expected no waiting, slept %v, retried %v", rec.slept, rec.attempts)
	}
}

func TestRetry_RetriesWithBackoffUntilTheOperationSucceeds(t *testing.T) {
	rec := &retryRecorder{}
	calls := 0
	failing := errors.New("boom")

	err := Retry(3, rec.sleep, rec.onRetry, func() error {
		calls++
		if calls <= 2 {
			return failing
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
	wantWaits := []time.Duration{2 * time.Second, 5 * time.Second}
	if !reflect.DeepEqual(rec.slept, wantWaits) {
		t.Fatalf("slept %v, want %v", rec.slept, wantWaits)
	}
	if !reflect.DeepEqual(rec.waits, wantWaits) {
		t.Fatalf("reported waits %v, want %v", rec.waits, wantWaits)
	}
	if !reflect.DeepEqual(rec.attempts, []int{1, 2}) {
		t.Fatalf("reported attempts %v, want [1 2]", rec.attempts)
	}
	for _, e := range rec.errs {
		if e != failing {
			t.Fatalf("reported error %v, want %v", e, failing)
		}
	}
}

func TestRetry_ReturnsTheLastErrorWhenRetriesAreExhausted(t *testing.T) {
	rec := &retryRecorder{}
	calls := 0

	err := Retry(2, rec.sleep, rec.onRetry, func() error {
		calls++
		return errors.New("attempt " + string(rune('0'+calls)))
	})

	if err == nil || err.Error() != "attempt 3" {
		t.Fatalf("expected the last error (attempt 3), got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 1 + 2 retries = 3 calls, got %d", calls)
	}
	if len(rec.slept) != 2 {
		t.Fatalf("expected to wait twice, slept %v", rec.slept)
	}
}

func TestRetry_ZeroRetriesRunsExactlyOnce(t *testing.T) {
	rec := &retryRecorder{}
	calls := 0

	err := Retry(0, rec.sleep, rec.onRetry, func() error {
		calls++
		return errors.New("nope")
	})

	if err == nil {
		t.Fatal("expected the error to be returned")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call, got %d", calls)
	}
	if len(rec.slept) != 0 {
		t.Fatalf("expected no waiting, slept %v", rec.slept)
	}
}

func TestRetry_NilSleepAndNilCallbackAreAllowed(t *testing.T) {
	calls := 0
	// Retries are 0 so no sleep is attempted even with a nil sleep.
	err := Retry(0, nil, nil, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

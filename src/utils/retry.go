package utils

import "time"

// backoffSchedule holds the waits before the first retries; later retries double the last entry.
var backoffSchedule = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// maxBackoff caps the wait between two attempts.
const maxBackoff = 2 * time.Minute

// Backoff returns how long to wait before retry number attempt (1-based):
// 2s, 5s, 10s, then doubling (20s, 40s, 80s) and never more than 2 minutes.
// Attempts below 1 are treated as the first.
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt <= len(backoffSchedule) {
		return backoffSchedule[attempt-1]
	}

	wait := backoffSchedule[len(backoffSchedule)-1]
	for i := len(backoffSchedule); i < attempt; i++ {
		wait *= 2
		if wait >= maxBackoff {
			return maxBackoff
		}
	}
	return wait
}

// Retry runs fn until it succeeds or it has failed 1+retries times, returning the last error.
// Before each retry it reports the failed attempt through onRetry and waits Backoff(attempt)
// through sleep. Either callback may be nil; a nil sleep defaults to time.Sleep.
func Retry(retries int, sleep func(time.Duration), onRetry func(attempt int, wait time.Duration, err error), fn func() error) error {
	if sleep == nil {
		sleep = time.Sleep
	}

	for attempt := 1; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if attempt > retries {
			return err
		}

		wait := Backoff(attempt)
		if onRetry != nil {
			onRetry(attempt, wait, err)
		}
		sleep(wait)
	}
}

package runner

import (
	"context"
	"sync"
	"time"
)

type RateLimiter interface {
	Wait(ctx context.Context, domain string) error
	Close()
}

type globalRateLimiter struct {
	ticker *time.Ticker
}

func newGlobalRateLimiter(requestsPerSecond int) RateLimiter {
	if requestsPerSecond <= 0 {
		return &noRateLimiter{}
	}

	return &globalRateLimiter{
		ticker: time.NewTicker(time.Second / time.Duration(requestsPerSecond)),
	}
}

func (r *globalRateLimiter) Wait(ctx context.Context, domain string) error {
	select {
	case <-r.ticker.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *globalRateLimiter) Close() {
	r.ticker.Stop()
}

type domainRateLimiter struct {
	maxRequests int
	window      time.Duration
	domains     map[string]*domainTracker
	mu          sync.Mutex
}

type domainTracker struct {
	timestamps []time.Time
	mu         sync.Mutex
}

func newDomainRateLimiter(maxRequests int, window time.Duration) RateLimiter {
	if maxRequests <= 0 {
		return &noRateLimiter{}
	}

	return &domainRateLimiter{
		maxRequests: maxRequests,
		window:      window,
		domains:     make(map[string]*domainTracker),
	}
}

func (r *domainRateLimiter) Wait(ctx context.Context, domain string) error {
	r.mu.Lock()
	tracker, exists := r.domains[domain]
	if !exists {
		tracker = &domainTracker{
			timestamps: make([]time.Time, 0, r.maxRequests),
		}
		r.domains[domain] = tracker
	}
	r.mu.Unlock()

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	validTimestamps := make([]time.Time, 0, len(tracker.timestamps))
	for _, ts := range tracker.timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	tracker.timestamps = validTimestamps

	if len(tracker.timestamps) >= r.maxRequests {
		oldestTimestamp := tracker.timestamps[0]
		waitUntil := oldestTimestamp.Add(r.window)
		waitDuration := time.Until(waitUntil)

		if waitDuration > 0 {
			select {
			case <-time.After(waitDuration):
				tracker.timestamps = tracker.timestamps[1:]
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			tracker.timestamps = tracker.timestamps[1:]
		}
	}

	tracker.timestamps = append(tracker.timestamps, now)
	return nil
}

func (r *domainRateLimiter) Close() {}

type noRateLimiter struct{}

func (r *noRateLimiter) Wait(ctx context.Context, domain string) error {
	return nil
}

func (r *noRateLimiter) Close() {}

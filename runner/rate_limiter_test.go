package runner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNoRateLimiter(t *testing.T) {
	limiter := &noRateLimiter{}
	ctx := context.Background()

	start := time.Now()
	for range 100 {
		if err := limiter.Wait(ctx, "example.com"); err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("noRateLimiter took %v, should be instant", elapsed)
	}

	limiter.Close()
}

func TestGlobalRateLimiter(t *testing.T) {
	t.Run("enforces rate limit", func(t *testing.T) {
		requestsPerSecond := 10
		limiter := newGlobalRateLimiter(requestsPerSecond)
		defer limiter.Close()

		ctx := context.Background()
		numRequests := 20

		start := time.Now()
		for range numRequests {
			if err := limiter.Wait(ctx, "example.com"); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
		elapsed := time.Since(start)

		expectedMin := time.Duration(numRequests-1) * time.Second / time.Duration(requestsPerSecond)
		if elapsed < expectedMin {
			t.Errorf("completed %d requests in %v, expected at least %v", numRequests, elapsed, expectedMin)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		limiter := newGlobalRateLimiter(1)
		defer limiter.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := limiter.Wait(ctx, "example.com")
		if err != context.Canceled {
			t.Errorf("Wait() error = %v, want context.Canceled", err)
		}
	})

	t.Run("applies to all domains", func(t *testing.T) {
		limiter := newGlobalRateLimiter(10)
		defer limiter.Close()

		ctx := context.Background()
		domains := []string{"example.com", "test.com", "another.com"}

		start := time.Now()
		for i := range 30 {
			domain := domains[i%len(domains)]
			if err := limiter.Wait(ctx, domain); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
		elapsed := time.Since(start)

		expectedMin := 2900 * time.Millisecond
		if elapsed < expectedMin {
			t.Errorf("global rate limiter should apply across all domains")
		}
	})

	t.Run("zero rate returns noRateLimiter", func(t *testing.T) {
		limiter := newGlobalRateLimiter(0)
		defer limiter.Close()

		ctx := context.Background()
		start := time.Now()
		for range 100 {
			if err := limiter.Wait(ctx, "example.com"); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
		elapsed := time.Since(start)

		if elapsed > 10*time.Millisecond {
			t.Errorf("zero rate should act as noRateLimiter")
		}
	})
}

func TestDomainRateLimiter(t *testing.T) {
	t.Run("enforces per-domain limit", func(t *testing.T) {
		maxRequests := 5
		window := 100 * time.Millisecond
		limiter := newDomainRateLimiter(maxRequests, window)
		defer limiter.Close()

		ctx := context.Background()
		domain := "example.com"

		start := time.Now()
		for range maxRequests {
			if err := limiter.Wait(ctx, domain); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
		elapsed := time.Since(start)

		if elapsed > 10*time.Millisecond {
			t.Errorf("first %d requests should be immediate, took %v", maxRequests, elapsed)
		}

		beforeWait := time.Now()
		if err := limiter.Wait(ctx, domain); err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		waitTime := time.Since(beforeWait)

		if waitTime < 50*time.Millisecond {
			t.Errorf("6th request should wait ~100ms, waited %v", waitTime)
		}
	})

	t.Run("independent domain limits", func(t *testing.T) {
		maxRequests := 3
		window := 200 * time.Millisecond
		limiter := newDomainRateLimiter(maxRequests, window)
		defer limiter.Close()

		ctx := context.Background()

		start := time.Now()
		for range maxRequests {
			if err := limiter.Wait(ctx, "example.com"); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
		for range maxRequests {
			if err := limiter.Wait(ctx, "test.com"); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
		elapsed := time.Since(start)

		if elapsed > 20*time.Millisecond {
			t.Errorf("domains should have independent limits, took %v", elapsed)
		}
	})

	t.Run("sliding window", func(t *testing.T) {
		maxRequests := 2
		window := 100 * time.Millisecond
		limiter := newDomainRateLimiter(maxRequests, window)
		defer limiter.Close()

		ctx := context.Background()
		domain := "example.com"

		limiter.Wait(ctx, domain)
		limiter.Wait(ctx, domain)

		time.Sleep(110 * time.Millisecond)

		start := time.Now()
		limiter.Wait(ctx, domain)
		limiter.Wait(ctx, domain)
		elapsed := time.Since(start)

		if elapsed > 10*time.Millisecond {
			t.Errorf("after window expired, requests should be immediate, took %v", elapsed)
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		maxRequests := 10
		window := 100 * time.Millisecond
		limiter := newDomainRateLimiter(maxRequests, window)
		defer limiter.Close()

		ctx := context.Background()
		domain := "example.com"
		numWorkers := 5
		requestsPerWorker := 5

		var wg sync.WaitGroup
		wg.Add(numWorkers)

		var successCount int32

		for range numWorkers {
			go func() {
				defer wg.Done()
				for range requestsPerWorker {
					if err := limiter.Wait(ctx, domain); err == nil {
						atomic.AddInt32(&successCount, 1)
					}
				}
			}()
		}

		wg.Wait()

		if successCount != int32(numWorkers*requestsPerWorker) {
			t.Errorf("successCount = %d, want %d", successCount, numWorkers*requestsPerWorker)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		limiter := newDomainRateLimiter(1, 100*time.Millisecond)
		defer limiter.Close()

		ctx := context.Background()
		domain := "example.com"

		limiter.Wait(ctx, domain)

		ctx2, cancel := context.WithCancel(context.Background())
		cancel()

		err := limiter.Wait(ctx2, domain)
		if err != context.Canceled {
			t.Errorf("Wait() error = %v, want context.Canceled", err)
		}
	})

	t.Run("zero rate returns noRateLimiter", func(t *testing.T) {
		limiter := newDomainRateLimiter(0, 100*time.Millisecond)
		defer limiter.Close()

		ctx := context.Background()
		start := time.Now()
		for range 100 {
			if err := limiter.Wait(ctx, "example.com"); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
		elapsed := time.Since(start)

		if elapsed > 10*time.Millisecond {
			t.Errorf("zero rate should act as noRateLimiter")
		}
	})
}

func TestDomainRateLimiter_CleanupOldTimestamps(t *testing.T) {
	maxRequests := 3
	window := 50 * time.Millisecond
	limiter := newDomainRateLimiter(maxRequests, window)
	defer limiter.Close()

	ctx := context.Background()
	domain := "example.com"

	for range maxRequests {
		limiter.Wait(ctx, domain)
	}

	time.Sleep(60 * time.Millisecond)

	start := time.Now()
	for range maxRequests {
		limiter.Wait(ctx, domain)
	}
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("old timestamps should be cleaned up, took %v", elapsed)
	}
}

func TestRateLimiter_Interface(t *testing.T) {
	limiters := []RateLimiter{
		&noRateLimiter{},
		newGlobalRateLimiter(10),
		newDomainRateLimiter(5, 100*time.Millisecond),
	}

	ctx := context.Background()

	for i, limiter := range limiters {
		if err := limiter.Wait(ctx, "test.com"); err != nil {
			t.Errorf("limiter %d Wait() error: %v", i, err)
		}
		limiter.Close()
	}
}

func TestGlobalRateLimiter_HighRate(t *testing.T) {
	requestsPerSecond := 1000
	limiter := newGlobalRateLimiter(requestsPerSecond)
	defer limiter.Close()

	ctx := context.Background()
	numRequests := 100

	start := time.Now()
	for range numRequests {
		if err := limiter.Wait(ctx, "example.com"); err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
	}
	elapsed := time.Since(start)

	expectedMin := time.Duration(numRequests-1) * time.Second / time.Duration(requestsPerSecond)
	expectedMax := expectedMin + 50*time.Millisecond

	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("elapsed %v not in expected range [%v, %v]", elapsed, expectedMin, expectedMax)
	}
}

func TestDomainRateLimiter_ManyDomains(t *testing.T) {
	maxRequests := 5
	window := 100 * time.Millisecond
	limiter := newDomainRateLimiter(maxRequests, window)
	defer limiter.Close()

	ctx := context.Background()
	numDomains := 10

	start := time.Now()
	for i := range numDomains {
		domain := fmt.Sprintf("domain%d.com", i)
		for range maxRequests {
			if err := limiter.Wait(ctx, domain); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
		}
	}
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("all domains should complete immediately, took %v", elapsed)
	}
}

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type TokenBucketRateLimiter struct {
	tokens     int
	capacity   int
	refillRate time.Duration
	mu         sync.Mutex
	stopCh     chan struct{}
}

func NewTokenBucketRateLimiter(capacity int, refillRate time.Duration) *TokenBucketRateLimiter {
	rl := &TokenBucketRateLimiter{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		stopCh:     make(chan struct{}),
	}

	// Start token refill goroutine
	go rl.refillTokens()

	return rl
}

func (rl *TokenBucketRateLimiter) refillTokens() {
	ticker := time.NewTicker(rl.refillRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			if rl.tokens < rl.capacity {
				rl.tokens++
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// Wait implements RateLimiter interface
func (rl *TokenBucketRateLimiter) Wait(ctx context.Context, domain string) error {
	for {
		rl.mu.Lock()
		if rl.tokens > 0 {
			rl.tokens--
			rl.mu.Unlock()
			return nil
		}
		rl.mu.Unlock()

		// Wait a bit before trying again
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Close implements RateLimiter interface
func (rl *TokenBucketRateLimiter) Close() {
	close(rl.stopCh)
}

// CustomRunner that uses our rate limiter
type CustomRateLimitRunner struct {
	maxConcurrency int
	rateLimiter    *TokenBucketRateLimiter
	logger         logger.Logger
}

func NewCustomRateLimitRunner(concurrency, bucketSize int, refillRate time.Duration, log logger.Logger) *CustomRateLimitRunner {
	return &CustomRateLimitRunner{
		maxConcurrency: concurrency,
		rateLimiter:    NewTokenBucketRateLimiter(bucketSize, refillRate),
		logger:         log,
	}
}

func (r *CustomRateLimitRunner) Run(ctx context.Context, c crawler.Crawler, q storage.Queue) error {
	defer r.rateLimiter.Close()

	var wg sync.WaitGroup
	for range r.maxConcurrency {
		wg.Add(1)
		wg.Go(func() {
			defer wg.Done()
			for {
				empty, _ := q.IsEmpty()
				if empty {
					return
				}

				req, err := q.FetchNext(ctx)
				if err != nil || req == nil {
					return
				}

				// Apply rate limiting
				if err := r.rateLimiter.Wait(ctx, req.URL); err != nil {
					return
				}

				r.logger.Info("Processing url: %s, time: %s", req.URL, time.Now().Format("15:04:05.000"))

				// Crawl logic omitted for brevity - normally would call c.Crawl here
				q.MarkHandled(req)
			}
		})
	}

	wg.Wait()
	return nil
}

type RateLimitTestHandler struct{}

func (h *RateLimitTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	for i := range 10 {
		queue.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
	}

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &RateLimitTestHandler{},
		Storage: store,
		Logger:  log,
	})

	// Custom token bucket: 3 tokens, refill 1 per second
	log.Info("Using token bucket rate limiter (3 tokens, refill 1/sec)")
	r := NewCustomRateLimitRunner(5, 3, time.Second, log)

	start := time.Now()
	if err := r.Run(ctx, c, queue); err != nil {
		log.Error("Failed", "error", err)
	}

	log.Info("\nCompleted in %v with token bucket rate limiting\n", time.Since(start))
}

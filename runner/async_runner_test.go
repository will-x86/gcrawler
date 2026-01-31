package runner

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	crawler "github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type testCrawler struct {
	handler      crawler.Handler
	crawlFunc    func(context.Context, *url.URL, func(context.Context, *url.URL) error) error
	crawlCount   int32
	crawlHistory []string
	mu           sync.Mutex
}

func (c *testCrawler) Crawl(ctx context.Context, u *url.URL, enqueueFunc func(context.Context, *url.URL) error) error {
	atomic.AddInt32(&c.crawlCount, 1)

	c.mu.Lock()
	c.crawlHistory = append(c.crawlHistory, u.String())
	c.mu.Unlock()

	if c.crawlFunc != nil {
		return c.crawlFunc(ctx, u, enqueueFunc)
	}

	if c.handler != nil {
		hctx := &crawler.HandlerContext{
			URL:     u,
			Log:     logger.NewStdLogger(),
			Storage: storage.NewMemoryStorage(),
			EnqueueLinks: func(urls ...string) error {
				for _, urlStr := range urls {
					parsed, err := url.Parse(urlStr)
					if err != nil {
						continue
					}
					if err := enqueueFunc(ctx, parsed); err != nil {
						return err
					}
				}
				return nil
			},
			Get: func(key string) any { return nil },
		}
		return c.handler.Handle(ctx, hctx)
	}

	return nil
}

type testHandler struct {
	handleFunc func(context.Context, *crawler.HandlerContext) error
}

func (h *testHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	if h.handleFunc != nil {
		return h.handleFunc(ctx, hctx)
	}
	return nil
}

func TestAsyncRunner_Run(t *testing.T) {
	t.Run("processes all URLs", func(t *testing.T) {
		q := storage.NewMemoryQueue()
		ctx := context.Background()

		urls := []string{
			"https://example.com/page1",
			"https://example.com/page2",
			"https://example.com/page3",
		}

		for _, u := range urls {
			q.Add(ctx, u)
		}

		c := &testCrawler{}
		runner := NewAsyncRunner(2)

		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}

		if atomic.LoadInt32(&c.crawlCount) != int32(len(urls)) {
			t.Errorf("crawlCount = %d, want %d", c.crawlCount, len(urls))
		}

		isEmpty, _ := q.IsEmpty()
		if !isEmpty {
			t.Error("queue should be empty after run")
		}
	})

	t.Run("respects concurrency limit", func(t *testing.T) {
		q := storage.NewMemoryQueue()
		ctx := context.Background()

		numURLs := 20
		for i := range numURLs {
			q.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
		}

		maxConcurrency := 3
		var activeCrawls int32
		var maxActiveCrawls int32

		c := &testCrawler{
			crawlFunc: func(ctx context.Context, u *url.URL, enqueue func(context.Context, *url.URL) error) error {
				active := atomic.AddInt32(&activeCrawls, 1)
				for {
					current := atomic.LoadInt32(&maxActiveCrawls)
					if active <= current || atomic.CompareAndSwapInt32(&maxActiveCrawls, current, active) {
						break
					}
				}

				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&activeCrawls, -1)
				return nil
			},
		}

		runner := NewAsyncRunner(maxConcurrency)

		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}

		if maxActiveCrawls > int32(maxConcurrency) {
			t.Errorf("maxActiveCrawls = %d, want <= %d", maxActiveCrawls, maxConcurrency)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		q := storage.NewMemoryQueue()
		ctx, cancel := context.WithCancel(context.Background())

		for i := range 100 {
			q.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
		}

		c := &testCrawler{
			crawlFunc: func(ctx context.Context, u *url.URL, enqueue func(context.Context, *url.URL) error) error {
				time.Sleep(50 * time.Millisecond)
				return nil
			},
		}

		runner := NewAsyncRunner(5)

		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		err := runner.Run(ctx, c, q)
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("link policy filtering", func(t *testing.T) {
		q := storage.NewMemoryQueue()
		ctx := context.Background()

		q.Add(ctx, "https://example.com/seed")

		handler := &testHandler{
			handleFunc: func(ctx context.Context, hctx *crawler.HandlerContext) error {
				links := []string{
					"https://example.com/allowed1",
					"https://example.com/allowed2",
					"https://blocked.com/page",
				}
				return hctx.EnqueueLinks(links...)
			},
		}

		c := &testCrawler{handler: handler}
		policy := NewGlobPolicy("example.com")
		runner := NewAsyncRunner(2, WithLinkPolicy(policy))

		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}

		stats, _ := q.GetStats()
		completed := stats["completed"]
		if completed != 3 {
			t.Errorf("completed = %d, want 3 (seed + 2 allowed links)", completed)
		}
	})

	t.Run("handles crawl errors with retry", func(t *testing.T) {
		q := storage.NewMemoryQueueWithOptions(storage.MemoryQueueOptions{MaxRetries: 2})
		ctx := context.Background()

		q.Add(ctx, "https://example.com/page")

		var attemptCount int32
		c := &testCrawler{
			crawlFunc: func(ctx context.Context, u *url.URL, enqueue func(context.Context, *url.URL) error) error {
				count := atomic.AddInt32(&attemptCount, 1)
				if count < 2 {
					return errors.New("temporary error")
				}
				return nil
			},
		}

		runner := NewAsyncRunner(1)

		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}

		if attemptCount != 2 {
			t.Errorf("attemptCount = %d, want 2 (initial + 1 retry)", attemptCount)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		q := storage.NewMemoryQueueWithOptions(storage.MemoryQueueOptions{MaxRetries: 2})
		ctx := context.Background()

		q.Add(ctx, "https://example.com/page")

		c := &testCrawler{
			crawlFunc: func(ctx context.Context, u *url.URL, enqueue func(context.Context, *url.URL) error) error {
				return errors.New("persistent error")
			},
		}

		runner := NewAsyncRunner(1)

		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}

		if atomic.LoadInt32(&c.crawlCount) != 2 {
			t.Errorf("crawlCount = %d, want 2 (exhausted retries)", c.crawlCount)
		}
	})

	t.Run("rate limiter integration", func(t *testing.T) {
		q := storage.NewMemoryQueue()
		ctx := context.Background()

		for i := range 10 {
			q.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
		}

		c := &testCrawler{}
		runner := NewAsyncRunner(5, WithGlobalRateLimit(20))

		start := time.Now()
		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		elapsed := time.Since(start)

		expectedMin := 450 * time.Millisecond
		if elapsed < expectedMin {
			t.Errorf("with rate limit of 20/s, 10 requests should take at least %v, took %v", expectedMin, elapsed)
		}
	})

	t.Run("dynamic URL enqueueing", func(t *testing.T) {
		q := storage.NewMemoryQueue()
		ctx := context.Background()

		q.Add(ctx, "https://example.com/seed")

		handler := &testHandler{
			handleFunc: func(ctx context.Context, hctx *crawler.HandlerContext) error {
				if hctx.URL.Path == "/seed" {
					return hctx.EnqueueLinks(
						"https://example.com/child1",
						"https://example.com/child2",
					)
				}
				return nil
			},
		}

		c := &testCrawler{handler: handler}
		runner := NewAsyncRunner(2)

		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}

		if atomic.LoadInt32(&c.crawlCount) != 3 {
			t.Errorf("crawlCount = %d, want 3 (seed + 2 children)", c.crawlCount)
		}
	})
}

func TestAsyncRunner_Options(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		runner := NewAsyncRunner(0)

		if runner.maxConcurrency != 10 {
			t.Errorf("default maxConcurrency = %d, want 10", runner.maxConcurrency)
		}
		if _, ok := runner.rateLimiter.(*noRateLimiter); !ok {
			t.Error("default rate limiter should be noRateLimiter")
		}
		if runner.linkPolicy != PolicyAllowAll {
			t.Error("default link policy should be PolicyAllowAll")
		}
	})

	t.Run("custom concurrency", func(t *testing.T) {
		runner := NewAsyncRunner(5)
		if runner.maxConcurrency != 5 {
			t.Errorf("maxConcurrency = %d, want 5", runner.maxConcurrency)
		}
	})

	t.Run("WithGlobalRateLimit", func(t *testing.T) {
		runner := NewAsyncRunner(2, WithGlobalRateLimit(10))
		if _, ok := runner.rateLimiter.(*globalRateLimiter); !ok {
			t.Error("should use globalRateLimiter")
		}
	})

	t.Run("WithDomainRateLimit", func(t *testing.T) {
		runner := NewAsyncRunner(2, WithDomainRateLimit(5, time.Second))
		if _, ok := runner.rateLimiter.(*domainRateLimiter); !ok {
			t.Error("should use domainRateLimiter")
		}
	})

	t.Run("WithLinkPolicy", func(t *testing.T) {
		policy := PolicyAllowNone
		runner := NewAsyncRunner(2, WithLinkPolicy(policy))
		if runner.linkPolicy != policy {
			t.Error("should use provided link policy")
		}
	})

	t.Run("WithLogger", func(t *testing.T) {
		log := logger.NewStdLogger()
		runner := NewAsyncRunner(2, WithLogger(log))
		if runner.logger != log {
			t.Error("should use provided logger")
		}
	})

	t.Run("multiple options", func(t *testing.T) {
		policy := PolicyAllowNone
		log := logger.NewStdLogger()
		runner := NewAsyncRunner(3,
			WithGlobalRateLimit(10),
			WithLinkPolicy(policy),
			WithLogger(log),
		)

		if runner.maxConcurrency != 3 {
			t.Error("maxConcurrency not set correctly")
		}
		if _, ok := runner.rateLimiter.(*globalRateLimiter); !ok {
			t.Error("rate limiter not set correctly")
		}
		if runner.linkPolicy != policy {
			t.Error("link policy not set correctly")
		}
		if runner.logger != log {
			t.Error("logger not set correctly")
		}
	})
}

func TestAsyncRunner_EmptyQueue(t *testing.T) {
	q := storage.NewMemoryQueue()
	ctx := context.Background()

	c := &testCrawler{}
	runner := NewAsyncRunner(2)

	if err := runner.Run(ctx, c, q); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if atomic.LoadInt32(&c.crawlCount) != 0 {
		t.Errorf("crawlCount = %d, want 0 for empty queue", c.crawlCount)
	}
}

func TestAsyncRunner_InvalidURL(t *testing.T) {
	q := storage.NewMemoryQueue()
	ctx := context.Background()

	q.(*storage.MemoryQueue).Add(ctx, "https://valid.com/page")

	c := &testCrawler{}
	runner := NewAsyncRunner(1)

	if err := runner.Run(ctx, c, q); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if atomic.LoadInt32(&c.crawlCount) != 1 {
		t.Errorf("crawlCount = %d, want 1", c.crawlCount)
	}
}

func TestAsyncRunner_PolicyInitialization(t *testing.T) {
	q := storage.NewMemoryQueue()
	ctx := context.Background()

	q.Add(ctx, "https://example.com/seed")
	q.Add(ctx, "https://another.com/seed")

	policy := &sameDomainPolicy{allowedHosts: make(map[string]bool)}
	runner := NewAsyncRunner(1, WithLinkPolicy(policy))

	handler := &testHandler{
		handleFunc: func(ctx context.Context, hctx *crawler.HandlerContext) error {
			return hctx.EnqueueLinks("https://example.com/child")
		},
	}
	c := &testCrawler{handler: handler}

	if err := runner.Run(ctx, c, q); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(policy.allowedHosts) != 2 {
		t.Errorf("policy should be initialized with 2 hosts, got %d", len(policy.allowedHosts))
	}
}

func TestAsyncRunner_WorkerExitConditions(t *testing.T) {
	t.Run("exits when queue empty and no active workers", func(t *testing.T) {
		q := storage.NewMemoryQueue()
		ctx := context.Background()

		for i := range 5 {
			q.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
		}

		c := &testCrawler{
			crawlFunc: func(ctx context.Context, u *url.URL, enqueue func(context.Context, *url.URL) error) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		runner := NewAsyncRunner(3)
		start := time.Now()

		if err := runner.Run(ctx, c, q); err != nil {
			t.Fatalf("Run() error: %v", err)
		}

		elapsed := time.Since(start)

		if elapsed > 500*time.Millisecond {
			t.Errorf("should exit promptly when done, took %v", elapsed)
		}
	})
}

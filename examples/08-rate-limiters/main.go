package main

import (
	"context"
	"fmt"
	"time"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

type RateLimitTestHandler struct{}

func (h *RateLimitTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	hctx.Log.Info("Crawled: %s, time: %s", hctx.URL.String(), time.Now().Format("15:04:05.000"))
	return nil
}

func testRateLimit(name string, options ...runner.RunnerOption) {
	fmt.Printf("\nTesting %s n", name)
	ctx := context.Background()
	log := logger.NewStdLogger()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com/1")
	queue.Add(ctx, "https://example.com/2")
	queue.Add(ctx, "https://other.com/1")
	queue.Add(ctx, "https://other.com/2")

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &RateLimitTestHandler{},
		Storage: storage.NewMemoryStorage(),
		Logger:  log,
	})

	start := time.Now()
	r := runner.NewAsyncRunner(4, options...)
	r.Run(ctx, c, queue)

	log.Info("Completed in: %v\n", time.Since(start))
}

func main() {
	log := logger.NewStdLogger()

	// 1. No rate limiting
	testRateLimit("No Rate Limit", runner.WithLogger(log))

	// 2. Global rate limit (2 requests/second across all domains)
	testRateLimit("Global Rate Limit (2 req/sec)",
		runner.WithLogger(log),
		runner.WithGlobalRateLimit(2),
	)

	// 3. Domain rate limit (2 requests per 2 seconds per domain)
	testRateLimit("Domain Rate Limit (2 req/2sec per domain)",
		runner.WithLogger(log),
		runner.WithDomainRateLimit(2, 2*time.Second),
	)

}

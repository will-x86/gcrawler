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

type RunnerTestHandler struct{}

func (h *RunnerTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	hctx.Log.Info("Processing: %s", hctx.URL.String())
	return nil
}

func main() {
	ctx := context.Background()
	log := logger.NewStdLogger()

	// 1. AsyncRunner - Fixed concurrency
	fmt.Println("AsyncRunner (fixed 3 workers) ===")
	queue1 := storage.NewMemoryQueue()
	for i := range 5 {
		queue1.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
	}

	c1 := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &RunnerTestHandler{},
		Storage: storage.NewMemoryStorage(),
		Logger:  log,
	})

	asyncRunner := runner.NewAsyncRunner(3,
		runner.WithLogger(log),
		runner.WithGlobalRateLimit(10), // 10 requests/second
	)
	asyncRunner.Run(ctx, c1, queue1)

	// 2. AdaptiveRunner - Dynamic scaling based on memory
	fmt.Println("\nAdaptiveRunner (1-5 workers, scales with memory) ===")
	queue2 := storage.NewMemoryQueue()
	for i := range 5 {
		queue2.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
	}

	c2 := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &RunnerTestHandler{},
		Storage: storage.NewMemoryStorage(),
		Logger:  log,
	})

	adaptiveRunner := runner.NewAdaptiveRunner(
		1,    // minWorkers
		5,    // maxWorkers
		1000, // maxMemoryMB
		runner.WithAdaptiveLogger(log),
		runner.WithAdaptiveDomainRateLimit(60, time.Minute), // 60 req/min per domain
	)
	adaptiveRunner.Run(ctx, c2, queue2)

	fmt.Println("\nBoth runners completed")
}

package main

import (
	"context"
	"fmt"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

type PolicyTestHandler struct {
	counter *int
}

func (h *PolicyTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	*h.counter++
	hctx.Log.Info("Crawled url: %s, count: %d", hctx.URL.String(), *h.counter)

	hctx.EnqueueLinks(
		"https://example.com/page1",
		"https://other.com/page2",
		"https://example.com/page3",
	)
	return nil
}

func testPolicy(name string, policy runner.LinkPolicy) {
	fmt.Printf("\nTesting %sn", name)
	ctx := context.Background()
	log := logger.NewStdLogger()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	counter := 0
	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &PolicyTestHandler{counter: &counter},
		Storage: storage.NewMemoryStorage(),
		Logger:  log,
	})

	r := runner.NewAsyncRunner(1,
		runner.WithLogger(log),
		runner.WithLinkPolicy(policy),
	)
	r.Run(ctx, c, queue)

	fmt.Printf("Total pages crawled: %d\n", counter)
}

func main() {
	// 1. PolicyAllowAll - Crawls everything
	testPolicy("PolicyAllowAll", runner.PolicyAllowAll)

	// 2. PolicyAllowNone - Only seed URLs
	testPolicy("PolicyAllowNone", runner.PolicyAllowNone)

	// 3. PolicySameDomain - Only same domain as seeds
	testPolicy("PolicySameDomain", runner.PolicySameDomain)

	// 4. GlobPolicy - Pattern matching
	testPolicy("GlobPolicy (*.example.com)", runner.NewGlobPolicy("*.example.com"))

	// 5. MaxPerDomainPolicy - Limit per domain
	testPolicy("MaxPerDomainPolicy (max 2)", runner.NewMaxPerDomainPolicy(2))

	// 6. MaxDepthPolicy - Limit crawl depth
	testPolicy("MaxDepthPolicy (depth 1)", runner.NewMaxDepthPolicy(1, runner.PolicyAllowAll))

	// 7. Compound policies
	compound := runner.NewMaxDepthPolicy(2,
		runner.NewMaxPerDomainPolicy(3))
	testPolicy("Compound (depth 2 + max 3 per domain)", compound)
}

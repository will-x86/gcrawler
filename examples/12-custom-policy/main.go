package main

import (
	"context"
	"net/url"
	"strings"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

// PathFilterPolicy only allows URLs matching a path prefix
type PathFilterPolicy struct {
	allowedPaths []string
}

func NewPathFilterPolicy(paths ...string) *PathFilterPolicy {
	return &PathFilterPolicy{allowedPaths: paths}
}

// Initialize implements LinkPolicy interface
func (p *PathFilterPolicy) Initialize(ctx context.Context, queue storage.Queue) error {
	// No init needed
	return nil
}

// ShouldEnqueue implements LinkPolicy interface
func (p *PathFilterPolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	// Check if path starts with any allowed prefix
	for _, prefix := range p.allowedPaths {
		if strings.HasPrefix(parsed.Path, prefix) {
			return true
		}
	}

	return false
}

// PolicyTestHandler enqueues test links
type PolicyTestHandler struct{}

func (h *PolicyTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	hctx.Log.Info("Crawled url: %s", hctx.URL.String())

	// Try to enqueue various paths
	hctx.EnqueueLinks(
		"https://example.com/blog/post1",
		"https://example.com/blog/post2",
		"https://example.com/about",
		"https://example.com/docs/intro",
	)

	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &PolicyTestHandler{},
		Storage: store,
		Logger:  log,
	})

	// Custom policy: only allow /blog/ and /docs/ paths
	customPolicy := NewPathFilterPolicy("/blog/", "/docs/")

	r := runner.NewAsyncRunner(2,
		runner.WithLogger(log),
		runner.WithLinkPolicy(customPolicy),
	)

	if err := r.Run(ctx, c, queue); err != nil {
		log.Error("Failed", "error", err)
	}

}

package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

// CustomCrawler simulates crawling by generating fake content
type CustomCrawler struct {
	handler crawler.Handler
	storage storage.Storage
	logger  logger.Logger
}

func NewCustomCrawler(h crawler.Handler, s storage.Storage, l logger.Logger) *CustomCrawler {
	return &CustomCrawler{
		handler: h,
		storage: s,
		logger:  l,
	}
}

// implement Crawl interface
func (c *CustomCrawler) Crawl(ctx context.Context, u *url.URL, enqueueFunc func(context.Context, *url.URL) error) error {
	c.logger.Info("Custom crawling %s", u.String())

	// Create handler context with custom data getter
	hctx := &crawler.HandlerContext{
		URL:     u,
		Storage: c.storage,
		Log:     c.logger,
		EnqueueLinks: func(links ...string) error {
			for _, link := range links {
				if parsed, err := url.Parse(link); err == nil {
					enqueueFunc(ctx, parsed)
				}
			}
			return nil
		},
		Get: func(key string) any {
			// Provide custom data based on key
			switch key {
			case "fakeContent":
				return fmt.Sprintf("Simulated content for: %s", u.String())
			case "fakeLinks":
				return []string{
					fmt.Sprintf("%s://%s/link1", u.Scheme, u.Host),
					fmt.Sprintf("%s://%s/link2", u.Scheme, u.Host),
				}
			default:
				return nil
			}
		},
	}

	// Call the handler
	return c.handler.Handle(ctx, hctx)
}

type CustomHandler struct{}

func (h *CustomHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	content := hctx.Get("fakeContent").(string)
	links := hctx.Get("fakeLinks").([]string)

	hctx.Log.Info("Processing %s", hctx.URL.String())

	key := fmt.Sprintf("page:%s", strings.ReplaceAll(hctx.URL.String(), "://", "_"))
	hctx.Storage.Set(key, content)

	hctx.EnqueueLinks(links...)

	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	// Use our custom crawler
	customCrawler := NewCustomCrawler(&CustomHandler{}, store, log)

	r := runner.NewAsyncRunner(2,
		runner.WithLogger(log),
		runner.WithLinkPolicy(runner.NewMaxDepthPolicy(1, runner.PolicySameDomain)),
	)

	if err := r.Run(ctx, customCrawler, queue); err != nil {
		log.Error("Failed", "error", err)
	}

}

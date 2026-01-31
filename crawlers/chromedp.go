package crawlers

import (
	"context"
	"fmt"
	"net/url"

	"github.com/chromedp/chromedp"
	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type ChromeDPOptions struct {
	Handler  crawler.Handler
	Storage  storage.Storage
	Logger   logger.Logger
	Headless bool
}

type ChromeDPCrawler struct {
	handler  crawler.Handler
	storage  storage.Storage
	logger   logger.Logger
	headless bool
	allocCtx context.Context
	cancel   context.CancelFunc
}

func NewChromeDPCrawler(opts ChromeDPOptions) crawler.Crawler {
	if opts.Logger == nil {
		opts.Logger = logger.NewStdLogger()
	}

	return &ChromeDPCrawler{
		handler:  opts.Handler,
		storage:  opts.Storage,
		logger:   opts.Logger,
		headless: opts.Headless,
	}
}

func (c *ChromeDPCrawler) Crawl(ctx context.Context, u *url.URL, enqueueFunc func(context.Context, *url.URL) error) error {
	c.logger.Info("Crawling: %s", u.String())

	// Init browser on
	if c.allocCtx == nil {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", c.headless),
		)
		c.allocCtx, c.cancel = chromedp.NewExecAllocator(ctx, opts...)
	}

	// Create a new tab context for this crawl
	tabCtx, cancelTab := chromedp.NewContext(c.allocCtx)
	defer cancelTab()

	// Navigate to URL
	if err := chromedp.Run(tabCtx, chromedp.Navigate(u.String())); err != nil {
		return fmt.Errorf("navigation failed: %w", err)
	}

	hctx := &crawler.HandlerContext{
		URL:     u,
		Log:     c.logger,
		Storage: c.storage,
		EnqueueLinks: func(urls ...string) error {
			for _, urlStr := range urls {
				parsed, err := url.Parse(urlStr)
				if err != nil {
					c.logger.Error("Invalid URL %q: %v", urlStr, err)
					continue
				}

				if err := enqueueFunc(ctx, parsed); err != nil {
					return fmt.Errorf("failed to enqueue %s: %w", parsed.String(), err)
				}
			}
			return nil
		},
		Get: func(key string) any {
			if key == "page" {
				return tabCtx
			}
			return nil
		},
	}

	return c.handler.Handle(ctx, hctx)
}

// Close cleans up the browser resources
func (c *ChromeDPCrawler) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

package crawlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type HTTPOptions struct {
	Handler      crawler.Handler
	Storage      storage.Storage
	Logger       logger.Logger
	Timeout      time.Duration
	MaxRedirects int
}

type HTTPCrawler struct {
	handler crawler.Handler
	storage storage.Storage
	logger  logger.Logger
	client  *http.Client
}

func NewHTTPCrawler(opts HTTPOptions) crawler.Crawler {
	if opts.Logger == nil {
		opts.Logger = logger.NewStdLogger()
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxRedirects == 0 {
		opts.MaxRedirects = 10
	}

	client := &http.Client{
		Timeout: opts.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= opts.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", opts.MaxRedirects)
			}
			return nil
		},
	}

	return &HTTPCrawler{
		handler: opts.Handler,
		storage: opts.Storage,
		logger:  opts.Logger,
		client:  client,
	}
}

// Crawl is not async, as runner handles that
func (c *HTTPCrawler) Crawl(ctx context.Context, u *url.URL, enqueueFunc func(context.Context, *url.URL) error) error {
	c.logger.Debug("Crawling: %s", u.String())

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
			switch key {
			case "sendRequestResponse":
				return func() (*http.Response, error) {
					req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
					if err != nil {
						return nil, err
					}
					return c.client.Do(req)
				}
			case "sendRequestBytes":
				return func() ([]byte, error) {
					req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
					if err != nil {
						return nil, err
					}
					resp, err := c.client.Do(req)
					if err != nil {
						return nil, err
					}
					defer resp.Body.Close()
					return io.ReadAll(resp.Body)
				}
			case "sendRequestReader":
				return func() (io.ReadCloser, error) {
					req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
					if err != nil {
						return nil, err
					}
					resp, err := c.client.Do(req)
					if err != nil {
						return nil, err
					}
					return resp.Body, nil
				}
			}
			return nil
		},
	}

	return c.handler.Handle(ctx, hctx)
}

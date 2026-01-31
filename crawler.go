package crawler

import (
	"context"
	"net/url"

	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type Crawler interface {
	// Crawl processes a single URL
	Crawl(ctx context.Context, u *url.URL, enqueueFunc func(context.Context, *url.URL) error) error
}

type Handler interface {
	Handle(ctx context.Context, hctx *HandlerContext) error
}

type HandlerContext struct {
	URL          *url.URL
	EnqueueLinks func(...string) error
	Log          logger.Logger
	Storage      storage.Storage
	Get          func(key string) any
}

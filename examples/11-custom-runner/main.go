package main

import (
	"context"
	"net/url"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type SequentialRunner struct {
	logger logger.Logger
}

func NewSequentialRunner(log logger.Logger) *SequentialRunner {
	return &SequentialRunner{logger: log}
}

func (r *SequentialRunner) Run(ctx context.Context, c crawler.Crawler, q storage.Queue) error {
	r.logger.Info("Starting sequential runner")

	processed := 0
	for {
		empty, err := q.IsEmpty()
		if err != nil {
			return err
		}
		if empty {
			r.logger.Info("Queue empty, finished", "processed", processed)
			break
		}

		req, err := q.FetchNext(ctx)
		if err != nil {
			return err
		}
		if req == nil {
			break
		}

		r.logger.Info("Processing", "url", req.URL)

		u, err := url.Parse(req.URL)
		if err != nil {
			q.MarkHandledWithError(req, err)
			continue
		}

		// Crawl the URL
		enqueueFunc := func(ctx context.Context, newURL *url.URL) error {
			return q.Add(ctx, newURL.String())
		}

		if err := c.Crawl(ctx, u, enqueueFunc); err != nil {
			r.logger.Warn("Crawl error", "url", req.URL, "error", err)
			q.MarkHandledWithError(req, err)
		} else {
			q.MarkHandled(req)
			processed++
		}
	}

	return nil
}

type SequentialHandler struct{}

func (h *SequentialHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	hctx.Log.Info("Crawled sequentially", "url", hctx.URL.String())
	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com/1")
	queue.Add(ctx, "https://example.com/2")
	queue.Add(ctx, "https://example.com/3")

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &SequentialHandler{},
		Storage: store,
		Logger:  log,
	})

	r := NewSequentialRunner(log)

	if err := r.Run(ctx, c, queue); err != nil {
		log.Error("Failed", "error", err)
	}

}

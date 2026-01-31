package main

import (
	"context"
	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

type SimpleHandler struct{}

func (h *SimpleHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	sendRequestBytes := hctx.Get("sendRequestBytes").(func() ([]byte, error))
	body, err := sendRequestBytes()
	if err != nil {
		return err
	}

	key := hctx.URL.String()
	hctx.Storage.Set(key, string(body))
	hctx.Log.Info("Crawled %s", key)

	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &SimpleHandler{},
		Storage: store,
		Logger:  log,
	})

	r := runner.NewAsyncRunner(2, runner.WithLogger(log))
	r.Run(ctx, c, queue)

	val, _ := store.Get("https://example.com")
	log.Info("\nStored: %v", val)
}

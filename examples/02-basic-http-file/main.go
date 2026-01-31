package main

import (
	"context"
	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

type FileHandler struct{}

func (h *FileHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	sendRequestBytes := hctx.Get("sendRequestBytes").(func() ([]byte, error))
	body, err := sendRequestBytes()
	if err != nil {
		return err
	}

	hctx.Storage.Set("https://example.com", string(body))
	hctx.Log.Info("Saved page")
	return nil
}

func main() {
	ctx := context.Background()

	queue, _ := storage.NewFileQueue("./data/queue")
	queue.Add(ctx, "https://example.com")

	store, _ := storage.NewFileStorage("./data/storage")
	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &FileHandler{},
		Storage: store,
		Logger:  log,
	})

	r := runner.NewAsyncRunner(2, runner.WithLogger(log))
	r.Run(ctx, c, queue)

}

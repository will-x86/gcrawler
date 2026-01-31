package main

import (
	"context"
	"github.com/chromedp/chromedp"
	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

type ChromeDPHandler struct{}

func (h *ChromeDPHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	tabCtx := hctx.Get("page").(context.Context)

	var html string
	chromedp.Run(tabCtx,
		chromedp.Navigate(hctx.URL.String()),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &html),
	)

	hctx.Storage.Set("example.com", html)
	hctx.Log.Info("Rendered page")
	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	c := crawlers.NewChromeDPCrawler(crawlers.ChromeDPOptions{
		Handler:  &ChromeDPHandler{},
		Storage:  store,
		Logger:   log,
		Headless: true,
	})

	r := runner.NewAsyncRunner(1, runner.WithLogger(log))
	r.Run(ctx, c, queue)

	log.Info("\nDone!")
	if html, err := store.Get("example.com"); err != nil {
		panic(err)
	} else {
		log.Info("Got html: %v", html)
	}

	if stats, err := queue.GetStats(); err != nil {
		panic(err)
	} else {
		log.Info("Stats: %v", stats)
	}
}

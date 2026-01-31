package main

import (
	"context"
	"fmt"
	"os"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

type SQLiteHandler struct{}

func (h *SQLiteHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	sendRequestBytes := hctx.Get("sendRequestBytes").(func() ([]byte, error))
	body, err := sendRequestBytes()
	if err != nil {
		return err
	}

	key := fmt.Sprintf("page:%s", hctx.URL.String())
	hctx.Storage.Set(key, string(body))
	hctx.Log.Info("Crawled %s", hctx.URL.String())
	return nil
}

func main() {
	ctx := context.Background()

	os.Remove("./example.db")

	// (requires -tags sqlite3)
	queue, err := storage.NewSQLiteQueue(storage.SQLiteQueueOptions{
		DBPath:     "./example.db",
		MaxRetries: 3,
	})
	if err != nil {
		panic(err)
	}
	defer queue.Close()

	queue.Add(ctx, "https://example.com")

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &SQLiteHandler{},
		Storage: store,
		Logger:  log,
	})

	r := runner.NewAsyncRunner(2, runner.WithLogger(log))
	if err := r.Run(ctx, c, queue); err != nil {
		log.Error("Failed", "error", err)
	}

	if stats, err := queue.GetStats(); err == nil {
		fmt.Printf("\nQueue stats: %+v\n", stats)
	}
}

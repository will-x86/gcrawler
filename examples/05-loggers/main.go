package main

import (
	"context"
	"fmt"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

type LoggerTestHandler struct{}

func (h *LoggerTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	hctx.Log.Debug("Processing URL: %s", hctx.URL.String())
	hctx.Log.Info("Crawled successfully: %s", hctx.URL.String())
	return nil
}

func runWithLogger(loggerName string, log logger.Logger) {
	fmt.Printf("\n=== Testing %s ===\n", loggerName)
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	store := storage.NewMemoryStorage()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &LoggerTestHandler{},
		Storage: store,
		Logger:  log,
	})

	r := runner.NewAsyncRunner(1, runner.WithLogger(log))
	r.Run(ctx, c, queue)
}

func main() {
	// Standard Logger
	stdLog := logger.NewStdLogger()
	runWithLogger("StdLogger", stdLog)

	// Slog Logger
	slogLog := logger.NewSlogLogger()
	runWithLogger("SlogLogger", slogLog)

	// Zerolog Logger (basic)
	zerologLog := logger.NewZerologLogger()
	runWithLogger("ZerologLogger (basic) - coloured", zerologLog)

	// Zerolog Logger with options
	zerologCustom := logger.NewZerologLoggerWithOptions(logger.ZerologOptions{
		UseColor:   false,
		Level:      "debug",
		TimeFormat: "15:04:05",
		OutputFile: "./logs",
	})
	runWithLogger("ZerologLogger (no-colour, debug, file-based)", zerologCustom)
}

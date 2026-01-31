package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

// CustomLogger implements logger.Logger with custom formatting
type CustomLogger struct {
	prefix string
}

func NewCustomLogger(prefix string) *CustomLogger {
	return &CustomLogger{prefix: prefix}
}

func (l *CustomLogger) log(level, msg string, args ...any) {
	timestamp := time.Now().Format("15:04:05")
	var parts []string
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			parts = append(parts, fmt.Sprintf("%v=%v", args[i], args[i+1]))
		}
	}
	extra := ""
	if len(parts) > 0 {
		extra = " | " + strings.Join(parts, ", ")
	}
	fmt.Printf("[%s] %s [%s] %s%s\n", l.prefix, timestamp, level, msg, extra)
}

func (l *CustomLogger) Debug(msg string, args ...any) {
	l.log("DEBUG", msg, args...)
}

func (l *CustomLogger) Info(msg string, args ...any) {
	l.log("INFO", msg, args...)
}

func (l *CustomLogger) Warn(msg string, args ...any) {
	l.log("WARN", msg, args...)
}

func (l *CustomLogger) Error(msg string, args ...any) {
	l.log("ERROR", msg, args...)
}

type LoggerTestHandler struct{}

func (h *LoggerTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	hctx.Log.Debug("Handler started", "url", hctx.URL.String())
	hctx.Log.Info("Successfully crawled", "url", hctx.URL.String())
	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	store := storage.NewMemoryStorage()

	// Use custom logger with prefix
	log := NewCustomLogger("CRAWLER")

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &LoggerTestHandler{},
		Storage: store,
		Logger:  log,
	})

	r := runner.NewAsyncRunner(1, runner.WithLogger(log))

	if err := r.Run(ctx, c, queue); err != nil {
		log.Error("Crawl failed", "error", err)
	}

	fmt.Println("\nCustom logger output shown above")
}

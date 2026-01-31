package main

import (
	"context"
	"io"
	"sync"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

// PriorityQueue implements a priority-based URL queue
type PriorityQueue struct {
	highPriority []string
	lowPriority  []string
	visited      map[string]bool
	mu           sync.Mutex
}

func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		highPriority: []string{},
		lowPriority:  []string{},
		visited:      make(map[string]bool),
	}
}

func (q *PriorityQueue) Add(ctx context.Context, url string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.visited[url] {
		return nil // Already seen
	}

	// Simple priority: .com domains are high priority
	if len(url) > 4 && url[len(url)-4:] == ".com" {
		q.highPriority = append(q.highPriority, url)
	} else {
		q.lowPriority = append(q.lowPriority, url)
	}

	return nil
}

func (q *PriorityQueue) FetchNext(ctx context.Context) (*storage.Request, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var url string

	// High priority first
	if len(q.highPriority) > 0 {
		url = q.highPriority[0]
		q.highPriority = q.highPriority[1:]
	} else if len(q.lowPriority) > 0 {
		url = q.lowPriority[0]
		q.lowPriority = q.lowPriority[1:]
	} else {
		return nil, io.EOF
	}

	q.visited[url] = true

	return &storage.Request{
		URL:        url,
		RetryCount: 0,
	}, nil
}

func (q *PriorityQueue) MarkHandled(req *storage.Request) error {
	return nil
}

func (q *PriorityQueue) MarkHandledWithError(req *storage.Request, err error) error {
	return nil
}

func (q *PriorityQueue) IsEmpty() (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.highPriority) == 0 && len(q.lowPriority) == 0, nil
}

func (q *PriorityQueue) GetAllHosts(ctx context.Context) ([]string, error) {
	return []string{}, nil
}

func (q *PriorityQueue) GetStats() (map[string]int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	stats := make(map[string]int)
	stats["pending"] = len(q.highPriority) + len(q.lowPriority)
	stats["completed"] = len(q.visited)

	return stats, nil
}

func (q *PriorityQueue) Close() error {
	return nil
}

type PriorityTestHandler struct{}

func (h *PriorityTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	hctx.Log.Info("Processing:%s", hctx.URL.String())
	return nil
}

func main() {
	ctx := context.Background()

	queue := NewPriorityQueue()

	// Add URLs in mixed order - .com domains should be processed first
	queue.Add(ctx, "https://example.org/page1")
	queue.Add(ctx, "https://example.com/page2") // High priority
	queue.Add(ctx, "https://example.net/page3")
	queue.Add(ctx, "https://test.com/page4") // High priority

	store := storage.NewMemoryStorage()
	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &PriorityTestHandler{},
		Storage: store,
		Logger:  log,
	})

	r := runner.NewAsyncRunner(1, runner.WithLogger(log))

	log.Info("Processing with priority queue (.com domains first):")
	if err := r.Run(ctx, c, queue); err != nil {
		log.Error("Failed: %s", err.Error())
	}

}

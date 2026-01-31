package runner

import (
	"context"
	"io"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type AsyncRunner struct {
	maxConcurrency int
	rateLimiter    RateLimiter
	linkPolicy     LinkPolicy
	logger         logger.Logger
}

type RunnerOption func(*AsyncRunner)

func WithGlobalRateLimit(requestsPerSecond int) RunnerOption {
	return func(r *AsyncRunner) {
		r.rateLimiter = newGlobalRateLimiter(requestsPerSecond)
	}
}

func WithDomainRateLimit(maxRequests int, window time.Duration) RunnerOption {
	return func(r *AsyncRunner) {
		r.rateLimiter = newDomainRateLimiter(maxRequests, window)
	}
}

func WithLinkPolicy(policy LinkPolicy) RunnerOption {
	return func(r *AsyncRunner) {
		r.linkPolicy = policy
	}
}

func WithLogger(log logger.Logger) RunnerOption {
	return func(r *AsyncRunner) {
		r.logger = log
	}
}

func NewAsyncRunner(maxConcurrency int, opts ...RunnerOption) *AsyncRunner {
	if maxConcurrency == 0 {
		maxConcurrency = 10
	}

	r := &AsyncRunner{
		maxConcurrency: maxConcurrency,
		rateLimiter:    &noRateLimiter{},
		linkPolicy:     PolicyAllowAll,
		logger:         logger.NewStdLogger(),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *AsyncRunner) Run(ctx context.Context, c crawler.Crawler, q storage.Queue) error {
	if err := r.linkPolicy.Initialize(ctx, q); err != nil {
		return err
	}
	defer r.rateLimiter.Close()

	var wg sync.WaitGroup
	var activeWorkers int32

	workersDone := make(chan struct{})
	for i := 0; i < r.maxConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				req, err := q.FetchNext(ctx)
				if err == io.EOF {
					r.logger.Debug("Worker %d: Queue empty", workerID)
					return
				}
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					r.logger.Error("Worker %d: Failed to fetch request: %v", workerID, err)
					continue
				}

				u, err := url.Parse(req.URL)
				if err != nil {
					r.logger.Error("Worker %d: Invalid URL %s: %v", workerID, req.URL, err)
					q.MarkHandled(req)
					continue
				}

				if err := r.rateLimiter.Wait(ctx, u.Host); err != nil {
					r.logger.Error("Worker %d: Rate limiter cancelled: %v", workerID, err)
					q.MarkHandled(req)
					return
				}

				active := atomic.AddInt32(&activeWorkers, 1)
				r.logger.Debug("Worker %d: Processing %s (active: %d)", workerID, req.URL, active)

				policyFilteredEnqueue := func(ctx context.Context, newURL *url.URL) error {
					if r.linkPolicy.ShouldEnqueue(u, newURL.String()) {
						return q.Add(ctx, newURL.String())
					}
					return nil
				}

				crawlErr := c.Crawl(ctx, u, policyFilteredEnqueue)
				if crawlErr != nil {
					r.logger.Error("Worker %d: Failed to crawl %s: %v", workerID, req.URL, crawlErr)
					q.MarkHandledWithError(req, crawlErr)
				} else {
					q.MarkHandled(req)
				}

				atomic.AddInt32(&activeWorkers, -1)
			}
		}(i)
	}

	// Monitor queue and exit when empty and no active workers
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				isEmpty, _ := q.IsEmpty()
				active := atomic.LoadInt32(&activeWorkers)

				if isEmpty && active == 0 {
					r.logger.Debug("Queue empty and no active workers, finishing...")
					return
				}
			case <-ctx.Done():
				r.logger.Debug("Context cancelled")
				return
			}
		}
	}()

	// Wait for all workers
	go func() {
		wg.Wait()
		close(workersDone)
	}()

	select {
	case <-workersDone:
		r.logger.Info("All workers finished")
		return nil
	case <-ctx.Done():
		<-workersDone
		return ctx.Err()
	}
}

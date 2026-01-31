package runner

import (
	"context"
	"io"
	"net/url"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/storage"
)

type AdaptiveRunner struct {
	minWorkers     int
	maxWorkers     int
	currentWorkers int32
	activeWorkers  int32
	maxMemoryMB    uint64
	checkInterval  time.Duration
	rateLimiter    RateLimiter
	linkPolicy     LinkPolicy
	logger         logger.Logger
	workersMu      sync.Mutex
	stopWorkers    chan struct{}
	workerWg       sync.WaitGroup
}

type AdaptiveRunnerOption func(*AdaptiveRunner)

func WithAdaptiveRateLimit(requestsPerSecond int) AdaptiveRunnerOption {
	return func(r *AdaptiveRunner) {
		r.rateLimiter = newGlobalRateLimiter(requestsPerSecond)
	}
}

func WithAdaptiveDomainRateLimit(maxRequests int, window time.Duration) AdaptiveRunnerOption {
	return func(r *AdaptiveRunner) {
		r.rateLimiter = newDomainRateLimiter(maxRequests, window)
	}
}

func WithAdaptiveLinkPolicy(policy LinkPolicy) AdaptiveRunnerOption {
	return func(r *AdaptiveRunner) {
		r.linkPolicy = policy
	}
}

func WithAdaptiveLogger(log logger.Logger) AdaptiveRunnerOption {
	return func(r *AdaptiveRunner) {
		r.logger = log
	}
}

func WithCheckInterval(interval time.Duration) AdaptiveRunnerOption {
	return func(r *AdaptiveRunner) {
		r.checkInterval = interval
	}
}

func NewAdaptiveRunner(minWorkers, maxWorkers int, maxMemoryMB uint64, opts ...AdaptiveRunnerOption) *AdaptiveRunner {
	if minWorkers <= 0 {
		minWorkers = 2
	}
	if maxWorkers <= minWorkers {
		maxWorkers = minWorkers + 1
	}
	if maxMemoryMB == 0 {
		maxMemoryMB = 1024
	}

	r := &AdaptiveRunner{
		minWorkers:     minWorkers,
		maxWorkers:     maxWorkers,
		currentWorkers: int32(minWorkers),
		maxMemoryMB:    maxMemoryMB,
		checkInterval:  5 * time.Second,
		rateLimiter:    &noRateLimiter{},
		linkPolicy:     PolicyAllowAll,
		logger:         logger.NewStdLogger(),
		stopWorkers:    make(chan struct{}),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *AdaptiveRunner) getCurrentMemoryMB() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024 / 1024
}

func (r *AdaptiveRunner) adjustWorkers(ctx context.Context, c crawler.Crawler, q storage.Queue, workerChan chan struct{}) {
	currentMem := r.getCurrentMemoryMB()
	currentWorkers := atomic.LoadInt32(&r.currentWorkers)

	scaleUpThreshold := uint64(float64(r.maxMemoryMB) * 0.65)
	scaleDownThreshold := uint64(float64(r.maxMemoryMB) * 0.90)

	r.logger.Debug("Memory: %d/%d MB, Workers: %d", currentMem, r.maxMemoryMB, currentWorkers)

	if currentMem < scaleUpThreshold && int(currentWorkers) < r.maxWorkers {
		newWorker := currentWorkers + 1
		r.logger.Debug("Scaling up to %d workers (memory: %d MB)", newWorker, currentMem)
		atomic.StoreInt32(&r.currentWorkers, newWorker)

		r.workerWg.Add(1)
		go r.worker(ctx, int(newWorker), c, q, workerChan)
	}

	if currentMem > scaleDownThreshold && int(currentWorkers) > r.minWorkers {
		r.logger.Debug("Scaling down from %d workers (memory: %d MB)", currentWorkers, currentMem)
		atomic.AddInt32(&r.currentWorkers, -1)

		select {
		case workerChan <- struct{}{}:
		default:
		}
	}
}

func (r *AdaptiveRunner) worker(ctx context.Context, workerID int, c crawler.Crawler, q storage.Queue, stopChan chan struct{}) {
	defer r.workerWg.Done()

	for {
		select {
		case <-stopChan:
			r.logger.Debug("Worker %d: Stopping due to scale-down", workerID)
			return
		case <-ctx.Done():
			return
		default:
		}

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

		active := atomic.AddInt32(&r.activeWorkers, 1)
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

		atomic.AddInt32(&r.activeWorkers, -1)
	}
}

func (r *AdaptiveRunner) Run(ctx context.Context, c crawler.Crawler, q storage.Queue) error {
	if err := r.linkPolicy.Initialize(ctx, q); err != nil {
		return err
	}
	defer r.rateLimiter.Close()

	workerStopChan := make(chan struct{}, r.maxWorkers)

	for i := 0; i < r.minWorkers; i++ {
		time.Sleep(1 * time.Second)
		r.workerWg.Add(1)
		go r.worker(ctx, i, c, q, workerStopChan)
	}

	scalingDone := make(chan struct{})
	go func() {
		defer close(scalingDone)
		ticker := time.NewTicker(r.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.adjustWorkers(ctx, c, q, workerStopChan)
			case <-ctx.Done():
				return
			}
		}
	}()

	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				isEmpty, _ := q.IsEmpty()
				active := atomic.LoadInt32(&r.activeWorkers)

				if isEmpty && active == 0 {
					r.logger.Info("Queue empty and no active workers, finishing...")
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	workersDone := make(chan struct{})
	go func() {
		r.workerWg.Wait()
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

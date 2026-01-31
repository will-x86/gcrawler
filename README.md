# Gcrawler 



This repo is meant to be a crawler framework where you can swapout different parts of it.


See [examples](/examples/README.md) for examples



A typical flow would be:

### Setup

1. Create a queue (Filebased, Sqlite, memory, Custom)
    The queue handles queueing of URL's.
    Seed queue with your URL's, e.g. `q.Add(ctx, "...URL")`
2. Create Logger (Zerolog,Slog,Std, Custom)
3. Create Storage - for crawled data (File,Memory,Custom)
4. Create handler(See examples)
4. Create Crawler (http/Chromedp/custom)
5. Create Runner (Async, Adaptive Async) 
6. Run.


# Concepts 
## Handler

The handle is what you do per URL, similar to a typical http handler.

You're given:
```go
func (h *SimpleHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
type HandlerContext struct {
	URL          *url.URL
	EnqueueLinks func(...string) error
	Log          logger.Logger
	Storage      storage.Storage
	Get          func(key string) any
}
```

Here you use the `Get(key string)` function to fetch your crawlers object.
For example, in the chromeDB crawler:
```go
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

```
Here with the "page" you can navigate, fetch the URL etc.

See [examples](examples/README.md) for more ( basic http crawler etc)



## Crawled data Storage


This is a fairly simple interface, the provided versions are *memory* and *file based*

In your handler, should you want to store data, you'd use your *storage* here.
The interface is as such:
```go
type Storage interface {
	Set(key string, value any) error
	Get(key string) (any, error)
	Has(key string) bool
	Delete(key string) error
}
```
And in your handler you'd do:
```go
	hctx.Storage.Set("example.com", html)
```

For the file-based version that comes provided, you can set "any", so something such as this works great.
```go
	hctx.Storage.Set(key, map[string]any{
		"url":        hctx.URL.String(),
		"html":       string(body),
		"status":     resp.StatusCode,
		"crawled_at": time.Now(),
	})

```

## Queue 

The queue is where we get our URL's from, we seed it with some initially.

You should *not* access the queue in your handler, this is the job of the *runner*

To "enqueue" links as you scrape data, use the EnqueueLinks function in the context.

E.g.:
```go
if err := hctx.EnqueueLinks(links...); err != nil {
    hctx.Log.Error("Failed to enqueue links: %v", err)
}
```

More on the queue later but, the reason for this is simple, any policies to stop a link from being enqueued are ran through this function.

This brings us nicely to our next concept:



## LinkPolicy

The runner uses policies before enqueueing links. When your handler calls `EnqueueLinks()`, the runner checks `ShouldCrawl()` first.

```go
type LinkPolicy interface {
	ShouldCrawl(ctx context.Context, u *url.URL, depth int) bool
}
```

Policies can be chained - all must return true for URL to be crawled.

Example: `PolicySameDomain` extracts host from URL and checks against initial seed domains. `MaxDepthPolicy` tracks depth from seed URLs. Combine them to stay within domain boundaries at limited depth.

The `GlobPolicy` uses pattern matching - useful for targeting specific paths like `/blog/*` or excluding with `!` prefix.

## RateLimiter

Runner calls `Wait()` before fetching each URL. This blocks until rate limit allows.

```go
type RateLimiter interface {
	Wait(ctx context.Context, host string) error
}
```

The `globalRateLimiter` uses a single ticker - all domains share same rate.

The `domainRateLimiter` maintains separate sliding windows per host. More polite for multi-domain crawls.

Runner passes host extracted from URL to limiter, letting it decide per-domain vs global.

## Logger

Every component receives logger through options. Handler gets it via `HandlerContext`.

All internal operations (worker spawn/shutdown, queue operations, errors) log through this interface. You control output format and level.

The logger interface has no concept of formatting - implementations handle that. `ZerologLogger` outputs JSON, `StdLogger` plain text.

## Crawler

The crawler sits between runner and handler. Runner calls `Crawl()`, crawler fetches page, then calls your handler with context.

```go
type Crawler interface {
	Crawl(ctx context.Context, url string, enqueueLinks func(links ...string) error) error
}
```

Crawler exposes helpers via `Get()` in handler context. This keeps handler agnostic to HTTP vs browser.

**HTTPCrawler** makes request once, provides `sendRequestResponse`/`sendRequestBytes`/`sendRequestReader`. You choose based on needs - Response for headers, bytes for parsing, reader for streaming.

**ChromeDPCrawler** launches browser context, provides `Get("page")` as `context.Context`. Your handler runs chromedp actions against this. Crawler manages browser lifecycle.

This separation means handler logic stays same - just swap crawler implementation.

## Runners

Runner orchestrates everything. It:
1. Fetches URL from queue, marks processing
2. Checks rate limiter
3. Calls crawler
4. Marks URL completed/failed
5. Repeats until queue empty and no active workers

**AsyncRunner** spawns N workers at start. Fixed parallelism.

**AdaptiveRunner** monitors memory every 5s, spawns/kills workers dynamically. Uses `runtime.ReadMemStats()` to track heap usage against limit. Scales between min/max worker bounds.

Runner never touches handler directly - always through crawler. This lets crawler prepare context (fetch page, launch browser, etc) before handler runs.

## Queue Architecture

Queue is the shared state. Multiple workers race to `FetchNext()`, queue returns different URL to each.

URLs are normalized before adding: lowercase scheme/host, remove fragments, sort query params. Hash becomes unique key.

Request lifecycle:
1. `Add()` - status: pending
2. `FetchNext()` - status: processing, assign to worker
3. `MarkHandled()` - status: completed
4. `MarkHandledWithError()` - increment retry, status: pending if < MaxRetries, else failed

Queue deduplicates by unique key. Second add of same URL is no-op if already seen (any status).

`GetStats()` aggregates by status. Runner uses `IsEmpty()` to detect completion.

The FileQueue writes pending URLs as individual files, visited URLs to single JSON. SQLiteQueue uses transactions - fetch+mark atomic.

## URL Flow

```
Seed URLs -> Queue (pending)
    ↓
Runner.FetchNext() -> marks processing
    ↓
Rate Limiter -> Wait()
    ↓
Crawler.Crawl() -> fetch page
    ↓
Handler.Handle() -> EnqueueLinks()
    ↓
LinkPolicy.ShouldCrawl() -> filter
    ↓
Queue.Add() -> new URLs pending
    ↓
Runner.MarkHandled() -> completed/failed
```

Handler never sees queue or runner. Only `EnqueueLinks()` function. This function is bound to LinkPolicy by runner - policy filtering is transparent to handler.

Storage is separate from queue. Queue tracks URLs to crawl, storage holds crawled data. They don't interact.

## Async Mechanics

Runner spawns workers as goroutines. Each loops: fetch, crawl, mark. Shared queue coordinates via locks (MemoryQueue) or DB transactions (SQLiteQueue).

Context cancellation propagates: runner creates child contexts per worker, cancels on shutdown. Handler receives this context - should check `ctx.Done()` in long operations.

Runner tracks active workers with atomic counter. Exit condition: `queue.IsEmpty() && activeWorkers == 0`. Prevents race where last worker finishes while new URLs exist.

## Browser Crawler Details

ChromeDPCrawler allocates browser context once in `NewChromeDPCrawler()`. Each `Crawl()` call creates new tab context.

Handler gets tab context via `Get("page")`. Must use `chromedp.Run()` with this context to execute actions.

## Custom Implementations

Every interface can be swapped. Want priority queue? Implement Queue interface. Want proxy rotation? Implement Crawler, wrap HTTPCrawler.

Handler context provides `Get()` for crawler-specific helpers. Your custom crawler populates this map before calling handler.

Runner interface only requires `Run()` method. Can be sequential, concurrent, distributed - handler doesn't care.


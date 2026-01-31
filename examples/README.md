# GCrawler Examples

## Overview

GCrawler is a web crawler with pluggable components. These examples show:
- How to use built-in implementations
- How to create custom implementations of each interface

## Directory Structure

### Basic Examples (Built-in Implementations)

#### 01-basic-http-memory
HTTP crawler with in-memory storage and queue. Perfect for quick testing and small crawls.

#### 02-basic-http-file
HTTP crawler with file-based persistence. Data survives process restarts.

#### 03-basic-http-sqlite
HTTP crawler using SQLite database for queue management.

#### 04-basic-chromedp
Browser automation crawler using Chrome DevTools Protocol. Handles JavaScript-rendered content.

#### 05-loggers
Comparison of all three built-in logger implementations.

#### 06-runners
Comparison of AsyncRunner vs AdaptiveRunner.

#### 07-policies
Shows all built-in link policies.

- `PolicyAllowAll` - Crawl everything
- `PolicyAllowNone` - Only seed URLs
- `PolicySameDomain` - Stay within initial domains
- `GlobPolicy` - Pattern matching with wildcards
- `MaxPerDomainPolicy` - Limit URLs per domain
- `MaxDepthPolicy` - Limit crawl depth
- Compound policies (combining multiple policies)

#### 08-rate-limiters
Comparison of rate limiting strategies with timing measurements.

- No rate limiting (baseline)
- `GlobalRateLimiter` - Same limit across all domains
- `DomainRateLimiter` - Per-domain rate limits

### Custom Implementation Examples

#### 09-custom-crawler
Implements a custom crawler that simulates fetching content.

#### 10-custom-logger
Custom logger with formatted output and prefixes.

#### 11-custom-runner
Sequential runner that processes URLs one at a time (no concurrency).

#### 12-custom-policy
Path-based link filtering policy.

#### 13-custom-rate-limiter
Token bucket rate limiter implementation.

#### 14-custom-storage
Layered storage implementations with prefixing and statistics.

#### 15-custom-queue
Priority queue that processes .com domains first.

## Common Patterns

### Basic Crawl Setup

- queue
- storage
- logger
- crawler 
- runner

```go
ctx := context.Background()

// 1. Create queue
queue := storage.NewMemoryQueue()
queue.Add(ctx, "https://example.com")

// 2. Create storage
store := storage.NewMemoryStorage()

// 3. Create logger
log := logger.NewStdLogger()

// 4. Create crawler with handler
c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
    Handler: &YourHandler{},
    Storage: store,
    Logger:  log,
})

// 5. Create runner with options
r := runner.NewAsyncRunner(5,
    runner.WithLogger(log),
    runner.WithLinkPolicy(runner.PolicySameDomain),
)

// 6. Run
if err := r.Run(ctx, c, queue); err != nil {
    log.Error("Crawl failed", "error", err)
}
```

### Handler Implementation


```go
type YourHandler struct{}

func (h *YourHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
    // 1. Fetch content
    sendRequestBytes := hctx.Get("sendRequestBytes").(func() ([]byte, error))
    body, err := sendRequestBytes()
    if err != nil {
        return err
    }

    // 2. Process content
    // logic here 

    // 3. Store results
    hctx.Storage.Set("key", data)

    // 4. Enqueue discovered links
    hctx.EnqueueLinks("https://example.com/page2")

    // 5. Log activity
    hctx.Log.Info("Processed url: %s", hctx.URL.String())

    return nil
}
```

### HTTPCrawler Helper Functions

Available via `hctx.Get()`:

- `"sendRequestResponse"` → `func() (*http.Response, error)` - Get full HTTP response
- `"sendRequestBytes"` → `func() ([]byte, error)` - Get response body as bytes
- `"sendRequestReader"` → `func() (io.ReadCloser, error)` - Get response body as reader

### ChromeDPCrawler Context

Available via `hctx.Get()`:

- `"page"` → `context.Context` - ChromeDP tab context for browser automation

---


## Dependencies

Most examples require only the base gcrawler module. Specific requirements:

- **03-basic-http-sqlite**: Requires `-tags sqlite3` build flag 
- **04-basic-chromedp**: Requires Chrome/Chromium installed


## Architecture

```
Handler (your logic)
    ↓
Crawler (HTTP or ChromeDP)
    ↓
Runner (Async or Adaptive)
    ↓
Queue (Memory, File, SQLite, Turso)
    ↓
Storage (Memory or File)
    ↓
Logger (Std, Slog, Zerolog)
    ↓
RateLimiter (Global, Domain, Custom)
    ↓
LinkPolicy (filtering)
```

All components are interfaces - you can swap any implementation or create your own


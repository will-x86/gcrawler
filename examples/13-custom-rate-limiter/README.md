# Custom rate limiter


A custom rate limiter must implement:
```go
type RateLimiter interface {
	Wait(ctx context.Context, domain string) error
	Close()
}
```

For example:
```go
type domainRateLimiter struct {
	maxRequests int
	window      time.Duration
	domains     map[string]*domainTracker
	mu          sync.Mutex
}
type domainTracker struct {
	timestamps []time.Time
	mu         sync.Mutex
}
```


The following example implements a TokenBucket Rate limiter 


If you use a custom Rate Limiter, you have to use a CustomRunner as these do not work without it.

The default runner's are meant to work with default rate limits only

# Runners




Runners as the concept are essentially your async logic + enqueue logic

## Example 1 

- Maximum of 3 concurrent workers
- Global rate limit of 10 req/s

```go
	asyncRunner := runner.NewAsyncRunner(3,
		runner.WithLogger(log),
		runner.WithGlobalRateLimit(10), 
	)
	asyncRunner.Run(ctx, c, queue)
```


## Example 2

- As many runners as 1GB of ram permits
- Min/Max runners of 1 & 5 
- Maximum of 60 requests / domain / minute.

```go
	adaptiveRunner := runner.NewAdaptiveRunner(
		1,    // minWorkers
		5,    // maxWorkers
		1000, // maxMemoryMB
		runner.WithAdaptiveLogger(log),
		runner.WithAdaptiveDomainRateLimit(60, time.Minute), // 60 req/min per domain
	)
	adaptiveRunner.Run(ctx, c, queue)
```

This runner, adaptively adjusts the workers every check interval ( use `runner.WithCheckInterval(time.Time)` for adjusting)


## Example three

You only want to enqueue links within your *seed domains* 
There is no concept of "seeding" links in this crawler, you enqueue links manually at the beginning.
When links are enqueued in the Handle func (crawl), the LinkPolicy is applied.

```

Policies include:
```go
	PolicyAllowAll   LinkPolicy = &allowAllPolicy{}
	PolicyAllowNone  LinkPolicy = &allowNonePolicy{}
	PolicySameDomain LinkPolicy = &sameDomainPolicy{allowedHosts: make(map[string]bool)}
```

You would do:
```go
	adaptiveRunner := runner.NewAdaptiveRunner(
		1,    // minWorkers
		5,    // maxWorkers
		1000, // maxMemoryMB
        runner.WithAdaptiveLinkPolicy(runner.PolicySameDomain),
		runner.WithAdaptiveDomainRateLimit(60, time.Minute), // 60 req/min per domain
	)
```


For link policy's on the basic runner:
```go
		runner.WithLinkPolicy(runner.PolicySameDomain)
```


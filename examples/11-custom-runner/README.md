# Custom Runners 



Runners use async by default, there's two provided:

- Async runner ( e.g. "5" workers
- Adaptive runner ( 1-5 runners, adapt based upon ram) 


If you wanted to change this algorithm, or implement a sequential runner you can


The basic interface is:
```go

type Runner interface {
	Run(ctx context.Context, c crawler.Crawler, q storage.Queue) error
}
```



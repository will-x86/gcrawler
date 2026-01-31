# Custom policy 




This is the same as other customs 

Implement interface:

```go
type LinkPolicy interface {
	ShouldEnqueue(currentURL *url.URL, targetURL string) bool
	Initialize(ctx context.Context, queue storage.Queue) error
}
```

You can adapt your custom LinkPolicy type to have values, say you wanted to only enqueue a link if all previous responses have been 200? 

```go
// pseudo-code
type only200ResponsePolicy struct {
    db // db here
}
func (p *only200ResponsePolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
    if db.UrlIsOk(currentURL.Host){
        return true
    }
	return false
}
func (p *only200ResponsePolicy) Initialize9ctx context.Context, queue storage.Queue) error{
    // Init DB 
    p.db = db 
    return nil
}
```




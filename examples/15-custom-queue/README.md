# Custom queue 



# Closing nicely
The runners that are provided by default rely on `io.EOF` to know that the queue is empty 


Ensure you return `io.EOF` if it's empty. 

They will continue & panic due to out-ofbounds errors if not 



Interface:
```go
type RequestStatus string
const (
	StatusPending    RequestStatus = "pending"
	StatusProcessing RequestStatus = "processing"
	StatusCompleted  RequestStatus = "completed"
	StatusFailed     RequestStatus = "failed"
)
type Request struct {
	ID         string        `json:"id"`
	URL        string        `json:"url"`
	UniqueKey  string        `json:"uniqueKey"`
	Status     RequestStatus `json:"status"`
	RetryCount int           `json:"retryCount"`
	AddedAt    time.Time     `json:"addedAt"`
	UpdatedAt  time.Time     `json:"updatedAt"`
	LastError  string        `json:"lastError,omitempty"`
}
type Queue interface {
	Add(ctx context.Context, url string) error
	FetchNext(ctx context.Context) (*Request, error)
	MarkHandled(req *Request) error
	MarkHandledWithError(req *Request, err error) error
	IsEmpty() (bool, error)
	GetAllHosts(ctx context.Context) ([]string, error)
	GetStats() (map[string]int, error)
	Close() error
}

```

Realistically GetStats() and Close() are optional should you not need them.

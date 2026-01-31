package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryQueue struct {
	pending    map[string]*Request
	visited    map[string]bool
	mu         sync.RWMutex
	maxRetries int
}

type MemoryQueueOptions struct {
	MaxRetries int
}

func NewMemoryQueue() Queue {
	return NewMemoryQueueWithOptions(MemoryQueueOptions{
		MaxRetries: 3,
	})
}

func NewMemoryQueueWithOptions(opts MemoryQueueOptions) Queue {
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}
	return &MemoryQueue{
		pending:    make(map[string]*Request),
		visited:    make(map[string]bool),
		maxRetries: opts.MaxRetries,
	}
}
func (q *MemoryQueue) Close() error {
	return nil
}
func (q *MemoryQueue) Add(ctx context.Context, urlStr string) error {
	uniqueKey, err := normalizeURL(urlStr)
	if err != nil {
		return fmt.Errorf("failed to normalize URL: %w", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.visited[uniqueKey] {
		return nil
	}

	id := generateID(uniqueKey)
	if _, exists := q.pending[id]; exists {
		return nil
	}

	now := time.Now()
	q.pending[id] = &Request{
		ID:         id,
		URL:        urlStr,
		UniqueKey:  uniqueKey,
		Status:     StatusPending,
		RetryCount: 0,
		AddedAt:    now,
		UpdatedAt:  now,
	}

	return nil
}

func (q *MemoryQueue) FetchNext(ctx context.Context) (*Request, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for id, req := range q.pending {
		if req.RetryCount >= q.maxRetries {
			continue
		}

		delete(q.pending, id)
		req.Status = StatusProcessing
		req.UpdatedAt = time.Now()
		return req, nil
	}

	return nil, io.EOF
}

func (q *MemoryQueue) MarkHandled(req *Request) error {
	return q.MarkHandledWithError(req, nil)
}

func (q *MemoryQueue) MarkHandledWithError(req *Request, handleErr error) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if handleErr == nil {
		q.visited[req.UniqueKey] = true
		return nil
	}

	req.RetryCount++
	req.LastError = handleErr.Error()
	req.UpdatedAt = time.Now()

	if req.RetryCount >= q.maxRetries {
		req.Status = StatusFailed
		q.visited[req.UniqueKey] = true
		return nil
	}

	req.Status = StatusPending
	q.pending[req.ID] = req

	return nil
}

func (q *MemoryQueue) IsEmpty() (bool, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.pending) == 0, nil
}

func (q *MemoryQueue) GetAllHosts(ctx context.Context) ([]string, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	hostMap := make(map[string]bool)
	for _, req := range q.pending {
		if host, err := extractHost(req.URL); err == nil && host != "" {
			hostMap[host] = true
		}
	}

	hosts := make([]string, 0, len(hostMap))
	for host := range hostMap {
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func (q *MemoryQueue) GetStats() (map[string]int, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := make(map[string]int)
	stats[string(StatusPending)] = len(q.pending)
	stats[string(StatusCompleted)] = len(q.visited)

	return stats, nil
}

func normalizeURL(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""

	if u.RawQuery != "" {
		query := u.Query()
		u.RawQuery = ""

		keys := make([]string, 0, len(query))
		for k := range query {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var parts []string
		for _, k := range keys {
			vals := query[k]
			sort.Strings(vals)
			for _, v := range vals {
				parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		u.RawQuery = strings.Join(parts, "&")
	}

	return u.String(), nil
}

func generateID(uniqueKey string) string {
	hash := sha256.Sum256([]byte(uniqueKey))
	return hex.EncodeToString(hash[:8])
}

func extractHost(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	return strings.ToLower(u.Host), nil
}

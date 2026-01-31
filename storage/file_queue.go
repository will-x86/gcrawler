package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileQueue struct {
	baseDir     string
	pendingDir  string
	visitedFile string
	visited     map[string]bool
	mu          sync.RWMutex
	maxRetries  int
}

type FileQueueOptions struct {
	BaseDir    string
	MaxRetries int
}

func NewFileQueue(baseDir string) (Queue, error) {
	return NewFileQueueWithOptions(FileQueueOptions{
		BaseDir:    baseDir,
		MaxRetries: 3,
	})
}

func NewFileQueueWithOptions(opts FileQueueOptions) (Queue, error) {
	fmt.Println("NewFileQueueWithOptions: start")
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}

	pendingDir := filepath.Join(opts.BaseDir, "pending")
	visitedFile := filepath.Join(opts.BaseDir, "visited.json")

	fmt.Println("NewFileQueueWithOptions: creating dir", pendingDir)
	if err := os.MkdirAll(pendingDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create pending directory: %w", err)
	}

	fmt.Println("NewFileQueueWithOptions: creating queue struct")
	q := &FileQueue{
		baseDir:     opts.BaseDir,
		pendingDir:  pendingDir,
		visitedFile: visitedFile,
		visited:     make(map[string]bool),
		maxRetries:  opts.MaxRetries,
	}

	fmt.Println("NewFileQueueWithOptions: loading visited")
	if err := q.loadVisited(); err != nil {
		return nil, err
	}

	fmt.Println("NewFileQueueWithOptions: done")
	return q, nil
}

func (q *FileQueue) loadVisited() error {
	data, err := os.ReadFile(q.visitedFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read visited file: %w", err)
	}

	var visited []string
	if err := json.Unmarshal(data, &visited); err != nil {
		return fmt.Errorf("failed to unmarshal visited: %w", err)
	}

	for _, key := range visited {
		q.visited[key] = true
	}

	return nil
}

func (q *FileQueue) saveVisited() error {
	visited := make([]string, 0, len(q.visited))
	for key := range q.visited {
		visited = append(visited, key)
	}

	data, err := json.Marshal(visited)
	if err != nil {
		return fmt.Errorf("failed to marshal visited: %w", err)
	}

	return os.WriteFile(q.visitedFile, data, 0644)
}

func (q *FileQueue) Add(ctx context.Context, urlStr string) error {
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
	reqPath := filepath.Join(q.pendingDir, id+".json")

	if _, err := os.Stat(reqPath); err == nil {
		return nil
	}

	now := time.Now()
	req := Request{
		ID:         id,
		URL:        urlStr,
		UniqueKey:  uniqueKey,
		Status:     StatusPending,
		RetryCount: 0,
		AddedAt:    now,
		UpdatedAt:  now,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	return os.WriteFile(reqPath, data, 0644)
}

func (q *FileQueue) FetchNext(ctx context.Context) (*Request, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	entries, err := os.ReadDir(q.pendingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pending directory: %w", err)
	}

	for _, entry := range entries {
		reqPath := filepath.Join(q.pendingDir, entry.Name())
		data, err := os.ReadFile(reqPath)
		if err != nil {
			continue
		}

		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}

		if req.RetryCount >= q.maxRetries {
			continue
		}

		if err := os.Remove(reqPath); err != nil && !os.IsNotExist(err) {
			continue
		}

		req.Status = StatusProcessing
		req.UpdatedAt = time.Now()
		return &req, nil
	}

	return nil, io.EOF
}

func (q *FileQueue) MarkHandled(req *Request) error {
	return q.MarkHandledWithError(req, nil)
}

func (q *FileQueue) MarkHandledWithError(req *Request, handleErr error) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if handleErr == nil {
		q.visited[req.UniqueKey] = true
		return q.saveVisited()
	}

	req.RetryCount++
	req.LastError = handleErr.Error()
	req.UpdatedAt = time.Now()

	if req.RetryCount >= q.maxRetries {
		req.Status = StatusFailed
		q.visited[req.UniqueKey] = true
		return q.saveVisited()
	}

	req.Status = StatusPending
	reqPath := filepath.Join(q.pendingDir, req.ID+".json")
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	return os.WriteFile(reqPath, data, 0644)
}

func (q *FileQueue) IsEmpty() (bool, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	entries, err := os.ReadDir(q.pendingDir)
	if err != nil {
		return false, fmt.Errorf("failed to read pending directory: %w", err)
	}

	return len(entries) == 0, nil
}

func (q *FileQueue) GetAllHosts(ctx context.Context) ([]string, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	entries, err := os.ReadDir(q.pendingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pending directory: %w", err)
	}

	hostMap := make(map[string]bool)
	for _, entry := range entries {
		reqPath := filepath.Join(q.pendingDir, entry.Name())
		data, err := os.ReadFile(reqPath)
		if err != nil {
			continue
		}

		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}

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

func (q *FileQueue) GetStats() (map[string]int, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := make(map[string]int)

	entries, err := os.ReadDir(q.pendingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pending directory: %w", err)
	}

	stats[string(StatusPending)] = len(entries)
	stats[string(StatusCompleted)] = len(q.visited)

	return stats, nil
}

func (q *FileQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.saveVisited()
}

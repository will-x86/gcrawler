package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFileQueue_Add(t *testing.T) {
	tests := []struct {
		name      string
		urls      []string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "single URL",
			urls:      []string{"https://example.com/page"},
			wantCount: 1,
		},
		{
			name:      "duplicate URLs",
			urls:      []string{"https://example.com/page", "https://example.com/page"},
			wantCount: 1,
		},
		{
			name:      "URLs with fragments (should dedupe)",
			urls:      []string{"https://example.com/page#a", "https://example.com/page#b"},
			wantCount: 1,
		},
		{
			name:    "invalid URL",
			urls:    []string{"://invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			q, err := NewFileQueue(tmpDir)
			if err != nil {
				t.Fatalf("NewFileQueue() error: %v", err)
			}
			defer q.Close()

			ctx := context.Background()
			var lastErr error
			for _, url := range tt.urls {
				if err := q.Add(ctx, url); err != nil {
					lastErr = err
				}
			}

			if tt.wantErr && lastErr == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && lastErr != nil {
				t.Fatalf("unexpected error: %v", lastErr)
			}

			if !tt.wantErr {
				stats, err := q.GetStats()
				if err != nil {
					t.Fatalf("GetStats() error: %v", err)
				}
				if got := stats["pending"]; got != tt.wantCount {
					t.Errorf("pending count = %d, want %d", got, tt.wantCount)
				}
			}
		})
	}
}

func TestFileQueue_Persistence(t *testing.T) {
	t.Run("pending URLs persist across restarts", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := context.Background()

		// Create queue and add URLs
		q1, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}

		urls := []string{
			"https://example.com/page1",
			"https://example.com/page2",
			"https://example.com/page3",
		}
		for _, url := range urls {
			if err := q1.Add(ctx, url); err != nil {
				t.Fatalf("Add() error: %v", err)
			}
		}

		q1.Close()

		// Reopen queue
		q2, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() reopen error: %v", err)
		}
		defer q2.Close()

		// Verify URLs are still there
		for i := range len(urls) {
			req, err := q2.FetchNext(ctx)
			if err != nil {
				t.Fatalf("FetchNext() error: %v", err)
			}
			if req == nil {
				t.Fatalf("FetchNext() returned nil on iteration %d", i)
			}
		}

		// Should be empty now
		_, err = q2.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("expected EOF after fetching all URLs, got: %v", err)
		}
	})

	t.Run("visited URLs persist across restarts", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := context.Background()

		// Create queue, add, fetch, and mark handled
		q1, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}

		url := "https://example.com/page"
		if err := q1.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q1.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}

		if err := q1.MarkHandled(req); err != nil {
			t.Fatalf("MarkHandled() error: %v", err)
		}

		q1.Close()

		// Reopen and try to add same URL
		q2, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() reopen error: %v", err)
		}
		defer q2.Close()

		// Should be prevented from adding (already visited)
		if err := q2.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		// Should not be fetchable
		_, err = q2.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("visited URL should not be fetchable after restart")
		}
	})

	t.Run("retry state persists", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := context.Background()

		q1, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}

		url := "https://example.com/page"
		if err := q1.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q1.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}

		// Mark with error (retry count = 1)
		if err := q1.MarkHandledWithError(req, fmt.Errorf("temporary error")); err != nil {
			t.Fatalf("MarkHandledWithError() error: %v", err)
		}

		q1.Close()

		// Reopen
		q2, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() reopen error: %v", err)
		}
		defer q2.Close()

		// Fetch again and verify retry count
		req2, err := q2.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() after reopen error: %v", err)
		}

		if req2.RetryCount != 1 {
			t.Errorf("RetryCount = %d, want 1", req2.RetryCount)
		}
		if req2.LastError != "temporary error" {
			t.Errorf("LastError = %q, want %q", req2.LastError, "temporary error")
		}
	})
}

func TestFileQueue_FetchNext(t *testing.T) {
	t.Run("fetch from empty queue", func(t *testing.T) {
		tmpDir := t.TempDir()
		q, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		req, err := q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("FetchNext() error = %v, want io.EOF", err)
		}
		if req != nil {
			t.Errorf("FetchNext() req = %v, want nil", req)
		}
	})

	t.Run("fetch deletes pending file", func(t *testing.T) {
		tmpDir := t.TempDir()
		q, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		url := "https://example.com/page"
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		// Check file exists
		pendingDir := filepath.Join(tmpDir, "pending")
		files, err := os.ReadDir(pendingDir)
		if err != nil {
			t.Fatalf("ReadDir() error: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 pending file, got %d", len(files))
		}

		// Fetch should delete file
		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}
		if req == nil {
			t.Fatal("FetchNext() returned nil")
		}

		files, err = os.ReadDir(pendingDir)
		if err != nil {
			t.Fatalf("ReadDir() error: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("expected 0 pending files after fetch, got %d", len(files))
		}
	})
}

func TestFileQueue_MarkHandled(t *testing.T) {
	t.Run("successful mark updates visited.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		q, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		url := "https://example.com/page"
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}

		if err := q.MarkHandled(req); err != nil {
			t.Fatalf("MarkHandled() error: %v", err)
		}

		// Check visited.json exists and contains URL
		visitedFile := filepath.Join(tmpDir, "visited.json")
		data, err := os.ReadFile(visitedFile)
		if err != nil {
			t.Fatalf("ReadFile(visited.json) error: %v", err)
		}

		var visited []string
		if err := json.Unmarshal(data, &visited); err != nil {
			t.Fatalf("Unmarshal visited.json error: %v", err)
		}

		if len(visited) != 1 {
			t.Errorf("visited list length = %d, want 1", len(visited))
		}
	})
}

func TestFileQueue_MarkHandledWithError(t *testing.T) {
	t.Run("retry creates new pending file", func(t *testing.T) {
		tmpDir := t.TempDir()
		q, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		url := "https://example.com/page"
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}

		// Mark with error
		if err := q.MarkHandledWithError(req, fmt.Errorf("test error")); err != nil {
			t.Fatalf("MarkHandledWithError() error: %v", err)
		}

		// Check pending file exists again
		pendingDir := filepath.Join(tmpDir, "pending")
		files, err := os.ReadDir(pendingDir)
		if err != nil {
			t.Fatalf("ReadDir() error: %v", err)
		}
		if len(files) != 1 {
			t.Errorf("expected 1 pending file after error, got %d", len(files))
		}

		// Verify retry count incremented
		req2, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() after error: %v", err)
		}
		if req2.RetryCount != 1 {
			t.Errorf("RetryCount = %d, want 1", req2.RetryCount)
		}
	})

	t.Run("max retries marks as visited", func(t *testing.T) {
		tmpDir := t.TempDir()
		q, err := NewFileQueueWithOptions(FileQueueOptions{
			BaseDir:    tmpDir,
			MaxRetries: 2,
		})
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		url := "https://example.com/page"
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		// Fail twice
		for i := range 2 {
			req, err := q.FetchNext(ctx)
			if err != nil {
				t.Fatalf("FetchNext() iteration %d error: %v", i, err)
			}
			if err := q.MarkHandledWithError(req, fmt.Errorf("error %d", i)); err != nil {
				t.Fatalf("MarkHandledWithError() error: %v", err)
			}
		}

		// Should be empty
		_, err = q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("expected EOF after max retries, got: %v", err)
		}

		// Check visited.json contains the URL
		visitedFile := filepath.Join(tmpDir, "visited.json")
		data, err := os.ReadFile(visitedFile)
		if err != nil {
			t.Fatalf("ReadFile(visited.json) error: %v", err)
		}

		var visited []string
		if err := json.Unmarshal(data, &visited); err != nil {
			t.Fatalf("Unmarshal visited.json error: %v", err)
		}

		if len(visited) != 1 {
			t.Errorf("visited list should contain failed URL")
		}
	})
}

func TestFileQueue_IsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	q, err := NewFileQueue(tmpDir)
	if err != nil {
		t.Fatalf("NewFileQueue() error: %v", err)
	}
	defer q.Close()

	ctx := context.Background()

	isEmpty, err := q.IsEmpty()
	if err != nil {
		t.Fatalf("IsEmpty() error: %v", err)
	}
	if !isEmpty {
		t.Error("new queue should be empty")
	}

	if err := q.Add(ctx, "https://example.com/page"); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	isEmpty, err = q.IsEmpty()
	if err != nil {
		t.Fatalf("IsEmpty() error: %v", err)
	}
	if isEmpty {
		t.Error("queue should not be empty after add")
	}

	req, err := q.FetchNext(ctx)
	if err != nil {
		t.Fatalf("FetchNext() error: %v", err)
	}

	isEmpty, err = q.IsEmpty()
	if err != nil {
		t.Fatalf("IsEmpty() error: %v", err)
	}
	if !isEmpty {
		t.Error("queue should be empty after fetch")
	}

	q.MarkHandled(req)

	isEmpty, err = q.IsEmpty()
	if err != nil {
		t.Fatalf("IsEmpty() error: %v", err)
	}
	if !isEmpty {
		t.Error("queue should be empty after mark")
	}
}

func TestFileQueue_GetAllHosts(t *testing.T) {
	tmpDir := t.TempDir()
	q, err := NewFileQueue(tmpDir)
	if err != nil {
		t.Fatalf("NewFileQueue() error: %v", err)
	}
	defer q.Close()

	ctx := context.Background()

	urls := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://another.com/page",
	}

	for _, url := range urls {
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}
	}

	hosts, err := q.GetAllHosts(ctx)
	if err != nil {
		t.Fatalf("GetAllHosts() error: %v", err)
	}

	expectedHosts := map[string]bool{
		"example.com": true,
		"another.com": true,
	}

	if len(hosts) != len(expectedHosts) {
		t.Errorf("GetAllHosts() returned %d hosts, want %d", len(hosts), len(expectedHosts))
	}

	for _, host := range hosts {
		if !expectedHosts[host] {
			t.Errorf("unexpected host: %q", host)
		}
	}
}

func TestFileQueue_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	q, err := NewFileQueue(tmpDir)
	if err != nil {
		t.Fatalf("NewFileQueue() error: %v", err)
	}
	defer q.Close()

	ctx := context.Background()

	// Add URLs
	for i := range 3 {
		url := fmt.Sprintf("https://example.com/page%d", i)
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}
	}

	stats, err := q.GetStats()
	if err != nil {
		t.Fatalf("GetStats() error: %v", err)
	}
	if stats["pending"] != 3 {
		t.Errorf("pending = %d, want 3", stats["pending"])
	}

	// Fetch and mark one
	req, _ := q.FetchNext(ctx)
	q.MarkHandled(req)

	stats, err = q.GetStats()
	if err != nil {
		t.Fatalf("GetStats() error: %v", err)
	}
	if stats["pending"] != 2 {
		t.Errorf("pending = %d, want 2", stats["pending"])
	}
	if stats["completed"] != 1 {
		t.Errorf("completed = %d, want 1", stats["completed"])
	}
}

func TestFileQueue_Concurrent(t *testing.T) {
	t.Run("concurrent adds", func(t *testing.T) {
		tmpDir := t.TempDir()
		q, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		const numGoroutines = 20
		const urlsPerGoroutine = 10

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := range numGoroutines {
			go func(workerID int) {
				defer wg.Done()
				for j := range urlsPerGoroutine {
					url := fmt.Sprintf("https://example.com/page-%d-%d", workerID, j)
					if err := q.Add(ctx, url); err != nil {
						t.Errorf("Add() error: %v", err)
					}
				}
			}(i)
		}

		wg.Wait()

		stats, err := q.GetStats()
		if err != nil {
			t.Fatalf("GetStats() error: %v", err)
		}

		expected := numGoroutines * urlsPerGoroutine
		if stats["pending"] != expected {
			t.Errorf("pending = %d, want %d", stats["pending"], expected)
		}
	})

	t.Run("concurrent fetch", func(t *testing.T) {
		tmpDir := t.TempDir()
		q, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()

		// Add URLs
		const numURLs = 50
		for i := range numURLs {
			url := fmt.Sprintf("https://example.com/page-%d", i)
			if err := q.Add(ctx, url); err != nil {
				t.Fatalf("Add() error: %v", err)
			}
		}

		// Fetch concurrently
		const numWorkers = 5
		var wg sync.WaitGroup
		wg.Add(numWorkers)

		var fetchedCount int32
		var duplicates int32
		seenURLs := sync.Map{}

		for range numWorkers {
			go func() {
				defer wg.Done()
				for {
					req, err := q.FetchNext(ctx)
					if err == io.EOF {
						return
					}
					if err != nil {
						t.Errorf("FetchNext() error: %v", err)
						return
					}

					if _, exists := seenURLs.LoadOrStore(req.URL, true); exists {
						atomic.AddInt32(&duplicates, 1)
					}

					atomic.AddInt32(&fetchedCount, 1)
					q.MarkHandled(req)
				}
			}()
		}

		wg.Wait()

		if duplicates > 0 {
			t.Errorf("found %d duplicate fetches (race condition)", duplicates)
		}

		if fetchedCount != numURLs {
			t.Errorf("fetched %d URLs, want %d", fetchedCount, numURLs)
		}
	})
}

func TestFileQueue_CorruptedFiles(t *testing.T) {
	t.Run("corrupted pending file is skipped", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := context.Background()

		q, err := NewFileQueue(tmpDir)
		if err != nil {
			t.Fatalf("NewFileQueue() error: %v", err)
		}

		// Add a valid URL
		validURL := "https://example.com/valid"
		if err := q.Add(ctx, validURL); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		// Create a corrupted file
		pendingDir := filepath.Join(tmpDir, "pending")
		corruptFile := filepath.Join(pendingDir, "corrupted.json")
		if err := os.WriteFile(corruptFile, []byte("not valid json{{{"), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}

		// FetchNext should skip corrupted file and return valid one
		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}
		if req == nil {
			t.Fatal("FetchNext() returned nil")
		}
		if req.URL != validURL {
			t.Errorf("FetchNext() URL = %q, want %q", req.URL, validURL)
		}

		q.Close()
	})

	t.Run("corrupted visited.json is handled", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create corrupted visited.json
		visitedFile := filepath.Join(tmpDir, "visited.json")
		if err := os.WriteFile(visitedFile, []byte("not valid json"), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}

		// Should fail to create queue
		_, err := NewFileQueue(tmpDir)
		if err == nil {
			t.Error("NewFileQueue() should fail with corrupted visited.json")
		}
	})
}

func TestFileQueue_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	queueDir := filepath.Join(tmpDir, "nested", "queue", "dir")

	q, err := NewFileQueue(queueDir)
	if err != nil {
		t.Fatalf("NewFileQueue() with nested path error: %v", err)
	}
	defer q.Close()

	// Verify directory structure was created
	pendingDir := filepath.Join(queueDir, "pending")
	if _, err := os.Stat(pendingDir); os.IsNotExist(err) {
		t.Error("pending directory was not created")
	}
}

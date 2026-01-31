package storage

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryQueue_Add(t *testing.T) {
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
			name:      "different URLs",
			urls:      []string{"https://example.com/page1", "https://example.com/page2"},
			wantCount: 2,
		},
		{
			name:      "URLs with different fragments (should dedupe)",
			urls:      []string{"https://example.com/page#section1", "https://example.com/page#section2"},
			wantCount: 1,
		},
		{
			name:      "URLs with unsorted query params (should dedupe)",
			urls:      []string{"https://example.com/page?b=2&a=1", "https://example.com/page?a=1&b=2"},
			wantCount: 1,
		},
		{
			name:      "case sensitivity in scheme and host (should dedupe)",
			urls:      []string{"HTTPS://EXAMPLE.COM/page", "https://example.com/page"},
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
			q := NewMemoryQueue()
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

func TestMemoryQueue_NormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "https://example.com/page",
			want:  "https://example.com/page",
		},
		{
			input: "HTTPS://EXAMPLE.COM/page",
			want:  "https://example.com/page",
		},
		{
			input: "https://example.com/page#fragment",
			want:  "https://example.com/page",
		},
		{
			input: "https://example.com/page?z=3&a=1&b=2",
			want:  "https://example.com/page?a=1&b=2&z=3",
		},
		{
			input: "https://example.com/page?key=val1&key=val2",
			want:  "https://example.com/page?key=val1&key=val2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeURL(tt.input)
			if err != nil {
				t.Fatalf("normalizeURL() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("normalizeURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMemoryQueue_FetchNext(t *testing.T) {
	t.Run("fetch from empty queue returns EOF", func(t *testing.T) {
		q := NewMemoryQueue()
		ctx := context.Background()

		req, err := q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("FetchNext() error = %v, want io.EOF", err)
		}
		if req != nil {
			t.Errorf("FetchNext() req = %v, want nil", req)
		}
	})

	t.Run("fetch existing request", func(t *testing.T) {
		q := NewMemoryQueue()
		ctx := context.Background()

		url := "https://example.com/page"
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}
		if req == nil {
			t.Fatal("FetchNext() returned nil request")
		}
		if req.URL != url {
			t.Errorf("FetchNext() URL = %q, want %q", req.URL, url)
		}
		if req.Status != StatusProcessing {
			t.Errorf("FetchNext() Status = %q, want %q", req.Status, StatusProcessing)
		}
		if req.RetryCount != 0 {
			t.Errorf("FetchNext() RetryCount = %d, want 0", req.RetryCount)
		}
	})

	t.Run("fetch removes from pending", func(t *testing.T) {
		q := NewMemoryQueue()
		ctx := context.Background()

		if err := q.Add(ctx, "https://example.com/page"); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		// First fetch should succeed
		req1, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}
		if req1 == nil {
			t.Fatal("FetchNext() returned nil")
		}

		// Second fetch should return EOF
		req2, err := q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("FetchNext() error = %v, want io.EOF", err)
		}
		if req2 != nil {
			t.Errorf("FetchNext() = %v, want nil", req2)
		}
	})
}

func TestMemoryQueue_MarkHandled(t *testing.T) {
	t.Run("successful mark", func(t *testing.T) {
		q := NewMemoryQueue()
		ctx := context.Background()

		if err := q.Add(ctx, "https://example.com/page"); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}

		if err := q.MarkHandled(req); err != nil {
			t.Fatalf("MarkHandled() error: %v", err)
		}

		stats, _ := q.GetStats()
		if stats["completed"] != 1 {
			t.Errorf("completed count = %d, want 1", stats["completed"])
		}
		if stats["pending"] != 0 {
			t.Errorf("pending count = %d, want 0", stats["pending"])
		}
	})

	t.Run("prevents re-adding marked URL", func(t *testing.T) {
		q := NewMemoryQueue()
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

		// Try to add same URL again
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() after mark error: %v", err)
		}

		// Should not be fetchable
		_, err = q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("FetchNext() after re-add should return EOF, got: %v", err)
		}
	})
}

func TestMemoryQueue_MarkHandledWithError(t *testing.T) {
	t.Run("retry under max retries", func(t *testing.T) {
		q := NewMemoryQueueWithOptions(MemoryQueueOptions{MaxRetries: 3})
		ctx := context.Background()

		if err := q.Add(ctx, "https://example.com/page"); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}

		// Mark with error (retry count becomes 1)
		if err := q.MarkHandledWithError(req, fmt.Errorf("temporary error")); err != nil {
			t.Fatalf("MarkHandledWithError() error: %v", err)
		}

		// Should be available for retry
		req2, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() after error: %v", err)
		}
		if req2.RetryCount != 1 {
			t.Errorf("RetryCount = %d, want 1", req2.RetryCount)
		}
		if req2.LastError != "temporary error" {
			t.Errorf("LastError = %q, want %q", req2.LastError, "temporary error")
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		q := NewMemoryQueueWithOptions(MemoryQueueOptions{MaxRetries: 2})
		ctx := context.Background()

		if err := q.Add(ctx, "https://example.com/page"); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		// Fail twice (retry count 1, then 2)
		for i := range 2 {
			req, err := q.FetchNext(ctx)
			if err != nil {
				t.Fatalf("FetchNext() iteration %d error: %v", i, err)
			}
			if err := q.MarkHandledWithError(req, fmt.Errorf("error %d", i)); err != nil {
				t.Fatalf("MarkHandledWithError() error: %v", err)
			}
		}

		// Should not be available anymore
		_, err := q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("FetchNext() after max retries: want io.EOF, got %v", err)
		}

		// Should be marked as visited (prevents re-add)
		if err := q.Add(ctx, "https://example.com/page"); err != nil {
			t.Fatalf("Add() error: %v", err)
		}
		_, err = q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("should not be able to add after max retries")
		}
	})
}

func TestMemoryQueue_IsEmpty(t *testing.T) {
	q := NewMemoryQueue()
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
		t.Error("queue with pending items should not be empty")
	}

	req, err := q.FetchNext(ctx)
	if err != nil {
		t.Fatalf("FetchNext() error: %v", err)
	}

	// Fetched but not marked handled - queue should be empty
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
		t.Error("queue should still be empty after mark")
	}
}

func TestMemoryQueue_GetAllHosts(t *testing.T) {
	q := NewMemoryQueue()
	ctx := context.Background()

	urls := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://another.com/page",
		"http://example.com/page3", // Different scheme, same host
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

func TestMemoryQueue_GetStats(t *testing.T) {
	q := NewMemoryQueue()
	ctx := context.Background()

	// Add multiple URLs
	urls := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://example.com/page3",
	}
	for _, url := range urls {
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

	// Fetch and mark one as handled
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

func TestMemoryQueue_Concurrent(t *testing.T) {
	t.Run("concurrent adds", func(t *testing.T) {
		q := NewMemoryQueue()
		ctx := context.Background()

		const numGoroutines = 50
		const urlsPerGoroutine = 20

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
		q := NewMemoryQueue()
		ctx := context.Background()

		// Add URLs
		const numURLs = 100
		for i := range numURLs {
			url := fmt.Sprintf("https://example.com/page-%d", i)
			if err := q.Add(ctx, url); err != nil {
				t.Fatalf("Add() error: %v", err)
			}
		}

		// Fetch concurrently
		const numWorkers = 10
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

					// Check for duplicate fetch
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

	t.Run("concurrent add and fetch", func(t *testing.T) {
		q := NewMemoryQueue()
		ctx := context.Background()

		const duration = 500 * time.Millisecond
		done := make(chan struct{})

		var addedCount int32
		var fetchedCount int32

		// Adders
		for i := range 5 {
			go func(workerID int) {
				var count int32
				for {
					select {
					case <-done:
						atomic.AddInt32(&addedCount, count)
						return
					default:
						url := fmt.Sprintf("https://example.com/worker%d-%d", workerID, count)
						q.Add(ctx, url)
						count++
						time.Sleep(time.Millisecond)
					}
				}
			}(i)
		}

		// Fetchers
		for range 3 {
			go func() {
				var count int32
				for {
					select {
					case <-done:
						atomic.AddInt32(&fetchedCount, count)
						return
					default:
						req, err := q.FetchNext(ctx)
						if err == nil && req != nil {
							q.MarkHandled(req)
							count++
						}
						time.Sleep(time.Millisecond)
					}
				}
			}()
		}

		time.Sleep(duration)
		close(done)
		time.Sleep(50 * time.Millisecond) // Let goroutines finish

		t.Logf("Added %d, Fetched %d URLs", addedCount, fetchedCount)

		// Fetch remaining
		remaining := 0
		for {
			req, err := q.FetchNext(ctx)
			if err == io.EOF {
				break
			}
			if req != nil {
				q.MarkHandled(req)
				remaining++
			}
		}

		total := int(fetchedCount) + remaining
		if total != int(addedCount) {
			t.Errorf("total fetched (%d) != added (%d)", total, addedCount)
		}
	})
}

func TestMemoryQueue_Close(t *testing.T) {
	q := NewMemoryQueue()
	if err := q.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

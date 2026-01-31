package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSQLiteQueue_Add(t *testing.T) {
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
			name:      "different URLs",
			urls:      []string{"https://example.com/page1", "https://example.com/page2"},
			wantCount: 2,
		},
		{
			name:      "URLs with query params in different order (should dedupe)",
			urls:      []string{"https://example.com/page?b=2&a=1", "https://example.com/page?a=1&b=2"},
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
			dbPath := filepath.Join(tmpDir, "test.db")

			q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
			if err != nil {
				t.Fatalf("NewSQLiteQueue() error: %v", err)
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

func TestSQLiteQueue_FetchNext(t *testing.T) {
	t.Run("fetch from empty queue", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
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

	t.Run("fetch updates status to processing", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
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
		if req == nil {
			t.Fatal("FetchNext() returned nil")
		}

		// Verify in database
		sqlQ := q.(*SQLiteQueue)
		var status string
		err = sqlQ.db.QueryRow("SELECT status FROM requests WHERE id = ?", req.ID).Scan(&status)
		if err != nil {
			t.Fatalf("QueryRow() error: %v", err)
		}
		if status != string(StatusProcessing) {
			t.Errorf("status = %q, want %q", status, StatusProcessing)
		}
	})

	t.Run("fetch is FIFO", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		urls := []string{
			"https://example.com/first",
			"https://example.com/second",
			"https://example.com/third",
		}

		for _, url := range urls {
			if err := q.Add(ctx, url); err != nil {
				t.Fatalf("Add() error: %v", err)
			}
			time.Sleep(10 * time.Millisecond) // Ensure different timestamps
		}

		// Fetch in order
		for i, expectedURL := range urls {
			req, err := q.FetchNext(ctx)
			if err != nil {
				t.Fatalf("FetchNext() iteration %d error: %v", i, err)
			}
			if req.URL != expectedURL {
				t.Errorf("FetchNext() iteration %d: URL = %q, want %q", i, req.URL, expectedURL)
			}
		}
	})
}

func TestSQLiteQueue_MarkHandled(t *testing.T) {
	t.Run("successful mark", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

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

	t.Run("prevents re-adding", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		url := "https://example.com/page"
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, _ := q.FetchNext(ctx)
		q.MarkHandled(req)

		// Try to add same URL
		if err := q.Add(ctx, url); err != nil {
			t.Fatalf("Add() after mark error: %v", err)
		}

		// Should not be fetchable
		_, err = q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("FetchNext() after re-add should return EOF")
		}
	})
}

func TestSQLiteQueue_MarkHandledWithError(t *testing.T) {
	t.Run("retry increments count", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath, MaxRetries: 3})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		if err := q.Add(ctx, "https://example.com/page"); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		req, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() error: %v", err)
		}

		// Mark with error
		testErr := fmt.Errorf("temporary error")
		if err := q.MarkHandledWithError(req, testErr); err != nil {
			t.Fatalf("MarkHandledWithError() error: %v", err)
		}

		// Fetch again
		req2, err := q.FetchNext(ctx)
		if err != nil {
			t.Fatalf("FetchNext() after error: %v", err)
		}

		sqlQ := q.(*SQLiteQueue)
		var retryCount int
		var lastError string
		err = sqlQ.db.QueryRow("SELECT retry_count, last_error FROM requests WHERE id = ?", req2.ID).Scan(&retryCount, &lastError)
		if err != nil {
			t.Fatalf("QueryRow() error: %v", err)
		}

		if retryCount != 1 {
			t.Errorf("retry_count = %d, want 1", retryCount)
		}
		if lastError != "temporary error" {
			t.Errorf("last_error = %q, want %q", lastError, "temporary error")
		}
	})

	t.Run("max retries marks as failed", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath, MaxRetries: 2})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()
		if err := q.Add(ctx, "https://example.com/page"); err != nil {
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

		// Should not be fetchable
		_, err = q.FetchNext(ctx)
		if err != io.EOF {
			t.Errorf("FetchNext() after max retries should return EOF")
		}

		stats, _ := q.GetStats()
		if stats["failed"] != 1 {
			t.Errorf("failed count = %d, want 1", stats["failed"])
		}
	})
}

func TestSQLiteQueue_IsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() error: %v", err)
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

	req, _ := q.FetchNext(ctx)

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

func TestSQLiteQueue_GetAllHosts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() error: %v", err)
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

func TestSQLiteQueue_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() error: %v", err)
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

func TestSQLiteQueue_ResetStuckRequests(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() error: %v", err)
	}
	defer q.Close()

	ctx := context.Background()

	// Add and fetch (will be in processing state)
	if err := q.Add(ctx, "https://example.com/stuck"); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	req, err := q.FetchNext(ctx)
	if err != nil {
		t.Fatalf("FetchNext() error: %v", err)
	}

	// Manually update timestamp to make it look old
	sqlQ := q.(*SQLiteQueue)
	oldTime := time.Now().Add(-10 * time.Minute)
	_, err = sqlQ.db.Exec("UPDATE requests SET updated_at = ? WHERE id = ?", oldTime, req.ID)
	if err != nil {
		t.Fatalf("manual UPDATE error: %v", err)
	}

	// Reset stuck requests
	if err := sqlQ.ResetStuckRequests(5 * time.Minute); err != nil {
		t.Fatalf("ResetStuckRequests() error: %v", err)
	}

	// Should be fetchable again
	req2, err := q.FetchNext(ctx)
	if err != nil {
		t.Fatalf("FetchNext() after reset error: %v", err)
	}
	if req2 == nil {
		t.Fatal("FetchNext() returned nil after reset")
	}
	if req2.URL != req.URL {
		t.Errorf("FetchNext() URL = %q, want %q", req2.URL, req.URL)
	}
}

func TestSQLiteQueue_Concurrent(t *testing.T) {
	t.Run("concurrent adds", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
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
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()

		const numURLs = 100
		for i := range numURLs {
			url := fmt.Sprintf("https://example.com/page-%d", i)
			if err := q.Add(ctx, url); err != nil {
				t.Fatalf("Add() error: %v", err)
			}
		}

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
			t.Errorf("found %d duplicate fetches (transaction isolation issue)", duplicates)
		}

		if fetchedCount != numURLs {
			t.Errorf("fetched %d URLs, want %d", fetchedCount, numURLs)
		}
	})

	t.Run("concurrent add and fetch", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()

		const duration = 500 * time.Millisecond
		done := make(chan struct{})

		var addedCount int32
		var fetchedCount int32

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
						if err := q.Add(ctx, url); err == nil {
							count++
						}
					}
				}
			}(i)
		}

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
					}
				}
			}()
		}

		time.Sleep(duration)
		close(done)
		time.Sleep(50 * time.Millisecond)

		t.Logf("Added %d, Fetched %d URLs", addedCount, fetchedCount)

		// Drain remaining
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

func TestSQLiteQueue_Transactions(t *testing.T) {
	t.Run("FetchNext is atomic", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
		if err != nil {
			t.Fatalf("NewSQLiteQueue() error: %v", err)
		}
		defer q.Close()

		ctx := context.Background()

		// Add a single URL
		if err := q.Add(ctx, "https://example.com/page"); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		// Try to fetch from two goroutines simultaneously
		var wg sync.WaitGroup
		wg.Add(2)

		results := make(chan *Request, 2)

		for range 2 {
			go func() {
				defer wg.Done()
				req, _ := q.FetchNext(ctx)
				results <- req
			}()
		}

		wg.Wait()
		close(results)

		var gotRequests []*Request
		for req := range results {
			if req != nil {
				gotRequests = append(gotRequests, req)
			}
		}

		if len(gotRequests) != 1 {
			t.Errorf("expected exactly 1 request to be fetched, got %d", len(gotRequests))
		}
	})
}

func TestSQLiteQueue_WALMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() error: %v", err)
	}
	defer q.Close()

	sqlQ := q.(*SQLiteQueue)

	// Verify WAL mode is enabled
	var journalMode string
	err = sqlQ.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode error: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}
}

func TestSQLiteQueue_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	// Create queue and add URLs
	q1, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() error: %v", err)
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

	// Reopen database
	q2, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() reopen error: %v", err)
	}
	defer q2.Close()

	// Verify all URLs are still there
	stats, err := q2.GetStats()
	if err != nil {
		t.Fatalf("GetStats() error: %v", err)
	}

	if stats["pending"] != len(urls) {
		t.Errorf("pending count after reopen = %d, want %d", stats["pending"], len(urls))
	}
}

func TestSQLiteQueue_ConnectionPool(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	q, err := NewSQLiteQueue(SQLiteQueueOptions{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteQueue() error: %v", err)
	}
	defer q.Close()

	sqlQ := q.(*SQLiteQueue)

	stats := sqlQ.db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("MaxOpenConnections = %d, want 25", stats.MaxOpenConnections)
	}
}

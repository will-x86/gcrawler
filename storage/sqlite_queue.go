package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteQueue struct {
	db         *sql.DB
	maxRetries int
}

type SQLiteQueueOptions struct {
	DBPath     string
	MaxRetries int
}

func NewSQLiteQueue(opts SQLiteQueueOptions) (Queue, error) {
	if opts.DBPath == "" {
		opts.DBPath = "./data/queue.db"
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}

	dbDir := filepath.Dir(opts.DBPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", opts.DBPath+"?_journal_mode=WAL&_busy_timeout=10000&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	q := &SQLiteQueue{
		db:         db,
		maxRetries: opts.MaxRetries,
	}

	if err := q.createTables(); err != nil {
		return nil, err
	}

	return q, nil
}

func (q *SQLiteQueue) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS requests (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		unique_key TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending',
		retry_count INTEGER NOT NULL DEFAULT 0,
		added_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		last_error TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_status ON requests(status);
	CREATE INDEX IF NOT EXISTS idx_unique_key ON requests(unique_key);
	CREATE INDEX IF NOT EXISTS idx_status_retry ON requests(status, retry_count);
	`

	_, err := q.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

func (q *SQLiteQueue) Add(ctx context.Context, urlStr string) error {
	uniqueKey, err := normalizeURL(urlStr)
	if err != nil {
		return fmt.Errorf("failed to normalize URL: %w", err)
	}

	var exists bool
	err = q.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM requests WHERE unique_key = ?)",
		uniqueKey,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if URL exists: %w", err)
	}

	if exists {
		return nil
	}

	id := generateID(uniqueKey)
	now := time.Now()

	_, err = q.db.ExecContext(ctx,
		`INSERT INTO requests (id, url, unique_key, status, retry_count, added_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, urlStr, uniqueKey, StatusPending, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to insert request: %w", err)
	}

	return nil
}

func (q *SQLiteQueue) FetchNext(ctx context.Context) (*Request, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var req Request
	var status string
	var retryCount int

	err = tx.QueryRowContext(ctx,
		`SELECT id, url, unique_key, status, retry_count, added_at
		 FROM requests
		 WHERE status = ? AND retry_count < ?
		 ORDER BY added_at ASC
		 LIMIT 1`,
		StatusPending, q.maxRetries,
	).Scan(&req.ID, &req.URL, &req.UniqueKey, &status, &retryCount, &req.AddedAt)

	if err == sql.ErrNoRows {
		return nil, io.EOF
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch request: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE requests
		 SET status = ?, updated_at = ?
		 WHERE id = ?`,
		StatusProcessing, time.Now(), req.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update request status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &req, nil
}

func (q *SQLiteQueue) MarkHandled(req *Request) error {
	return q.MarkHandledWithError(req, nil)
}

func (q *SQLiteQueue) MarkHandledWithError(req *Request, handleErr error) error {
	ctx := context.Background()

	if handleErr == nil {
		_, err := q.db.ExecContext(ctx,
			`UPDATE requests
			 SET status = ?, updated_at = ?
			 WHERE id = ?`,
			StatusCompleted, time.Now(), req.ID,
		)
		return err
	}

	var retryCount int
	err := q.db.QueryRowContext(ctx,
		"SELECT retry_count FROM requests WHERE id = ?",
		req.ID,
	).Scan(&retryCount)
	if err != nil {
		return fmt.Errorf("failed to get retry count: %w", err)
	}

	retryCount++
	newStatus := StatusPending
	if retryCount >= q.maxRetries {
		newStatus = StatusFailed
	}

	_, err = q.db.ExecContext(ctx,
		`UPDATE requests
		 SET status = ?, retry_count = ?, last_error = ?, updated_at = ?
		 WHERE id = ?`,
		newStatus, retryCount, handleErr.Error(), time.Now(), req.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update request: %w", err)
	}

	return nil
}

func (q *SQLiteQueue) IsEmpty() (bool, error) {
	var count int
	err := q.db.QueryRow(
		`SELECT COUNT(*) FROM requests
		 WHERE status = ? AND retry_count < ?`,
		StatusPending, q.maxRetries,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to count pending requests: %w", err)
	}

	return count == 0, nil
}

func (q *SQLiteQueue) Close() error {
	return q.db.Close()
}

func (q *SQLiteQueue) GetStats() (map[string]int, error) {
	stats := make(map[string]int)

	rows, err := q.db.Query(
		`SELECT status, COUNT(*) as count
		 FROM requests
		 GROUP BY status`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}

	return stats, nil
}

func (q *SQLiteQueue) ResetStuckRequests(timeout time.Duration) error {
	cutoff := time.Now().Add(-timeout)
	_, err := q.db.Exec(
		`UPDATE requests
		 SET status = ?, updated_at = ?
		 WHERE status = ? AND updated_at < ?`,
		StatusPending, time.Now(), StatusProcessing, cutoff,
	)
	return err
}

func (q *SQLiteQueue) GetAllHosts(ctx context.Context) ([]string, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT DISTINCT url FROM requests")
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer rows.Close()

	hostMap := make(map[string]bool)
	for rows.Next() {
		var urlStr string
		if err := rows.Scan(&urlStr); err != nil {
			continue
		}
		if host, err := extractHost(urlStr); err == nil && host != "" {
			hostMap[host] = true
		}
	}

	hosts := make([]string, 0, len(hostMap))
	for host := range hostMap {
		hosts = append(hosts, host)
	}
	return hosts, nil
}

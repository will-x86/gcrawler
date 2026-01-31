package main

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/crawlers"
	"github.com/will-x86/gcrawler/logger"
	"github.com/will-x86/gcrawler/runner"
	"github.com/will-x86/gcrawler/storage"
)

// PrefixedStorage wraps another storage and adds key prefixes
type PrefixedStorage struct {
	inner  storage.Storage
	prefix string
	mu     sync.RWMutex
}

func NewPrefixedStorage(inner storage.Storage, prefix string) *PrefixedStorage {
	return &PrefixedStorage{
		inner:  inner,
		prefix: prefix,
	}
}

func (s *PrefixedStorage) prefixKey(key string) string {
	return s.prefix + ":" + key
}

// Set implements Storage interface
func (s *PrefixedStorage) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Set(s.prefixKey(key), value)
}

// Get implements Storage interface
func (s *PrefixedStorage) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inner.Get(s.prefixKey(key))
}

// Has implements Storage interface
func (s *PrefixedStorage) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inner.Has(s.prefixKey(key))
}

// Delete implements Storage interface
func (s *PrefixedStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Delete(s.prefixKey(key))
}

// StatStorage tracks get/set statistics
type StatStorage struct {
	inner storage.Storage
	stats struct {
		sets    int
		gets    int
		deletes int
		mu      sync.Mutex
	}
}

func NewStatStorage(inner storage.Storage) *StatStorage {
	return &StatStorage{inner: inner}
}

func (s *StatStorage) Set(key string, value any) error {
	s.stats.mu.Lock()
	s.stats.sets++
	s.stats.mu.Unlock()
	return s.inner.Set(key, value)
}

func (s *StatStorage) Get(key string) (any, error) {
	s.stats.mu.Lock()
	s.stats.gets++
	s.stats.mu.Unlock()
	return s.inner.Get(key)
}

func (s *StatStorage) Has(key string) bool {
	return s.inner.Has(key)
}

func (s *StatStorage) Delete(key string) error {
	s.stats.mu.Lock()
	s.stats.deletes++
	s.stats.mu.Unlock()
	return s.inner.Delete(key)
}

func (s *StatStorage) GetStats() (sets, gets, deletes int) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	return s.stats.sets, s.stats.gets, s.stats.deletes
}

type StorageTestHandler struct{}

func (h *StorageTestHandler) Handle(ctx context.Context, hctx *crawler.HandlerContext) error {
	// Store some data
	data := map[string]string{
		"url":  hctx.URL.String(),
		"time": "now",
	}

	jsonData, _ := json.Marshal(data)
	hctx.Storage.Set("page", string(jsonData))

	hctx.Log.Info("Stored url: %s ", hctx.URL.String())
	return nil
}

func main() {
	ctx := context.Background()

	queue := storage.NewMemoryQueue()
	queue.Add(ctx, "https://example.com")

	baseStorage := storage.NewMemoryStorage()
	prefixedStorage := NewPrefixedStorage(baseStorage, "crawl-session-1")
	statStorage := NewStatStorage(prefixedStorage)

	log := logger.NewStdLogger()

	c := crawlers.NewHTTPCrawler(crawlers.HTTPOptions{
		Handler: &StorageTestHandler{},
		Storage: statStorage,
		Logger:  log,
	})

	r := runner.NewAsyncRunner(1, runner.WithLogger(log))

	if err := r.Run(ctx, c, queue); err != nil {
		log.Error("Failed", "error", err)
	}

	sets, gets, deletes := statStorage.GetStats()
	log.Info("\nStorage Statistics:\n")
	log.Info("  Sets: %d\n", sets)
	log.Info("  Gets: %d\n", gets)
	log.Info("  Deletes: %d\n", deletes)

	if val, err := baseStorage.Get("crawl-session-1:page"); err == nil {
		log.Info("\nPrefixed key 'crawl-session-1:page' contains: %v\n", val)
	}
}

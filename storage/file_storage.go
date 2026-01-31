package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type FileStorage struct {
	dir string
	mu  sync.RWMutex
}

func NewFileStorage(dir string) (Storage, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &FileStorage{dir: dir}, nil
}

func (f *FileStorage) Set(key string, value any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	key = sanitize(key, 0, "")
	path := filepath.Join(f.dir, key+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (f *FileStorage) Get(key string) (any, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	key = sanitize(key, 0, "")
	path := filepath.Join(f.dir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return value, nil
}

func (f *FileStorage) Has(key string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	key = sanitize(key, 0, "")
	path := filepath.Join(f.dir, key+".json")
	_, err := os.Stat(path)
	return err == nil
}

func (f *FileStorage) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	key = sanitize(key, 0, "")
	path := filepath.Join(f.dir, key+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

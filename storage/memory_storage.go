package storage

import (
	"encoding/json"
	"fmt"
	"sync"
)

type MemoryStorage struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func NewMemoryStorage() Storage {
	return &MemoryStorage{
		data: make(map[string][]byte),
	}
}

func (s *MemoryStorage) Set(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = data
	return nil
}

func (s *MemoryStorage) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found")
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return value, nil
}

func (s *MemoryStorage) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.data[key]
	return ok
}

func (s *MemoryStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	return nil
}

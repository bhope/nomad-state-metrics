// Package store provides a simple in-memory cache for Nomad API responses,
// intended to decouple scrape frequency from API poll frequency in future.
package store

import (
	"sync"
	"time"
)

// Entry holds a cached value with a timestamp.
type Entry[T any] struct {
	Value     T
	UpdatedAt time.Time
}

// Store is a generic thread-safe key-value cache.
type Store[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]Entry[V]
}

// New returns an empty Store.
func New[K comparable, V any]() *Store[K, V] {
	return &Store[K, V]{data: make(map[K]Entry[V])}
}

// Set stores a value for the given key.
func (s *Store[K, V]) Set(key K, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = Entry[V]{Value: value, UpdatedAt: time.Now()}
}

// Get retrieves a value and whether it existed.
func (s *Store[K, V]) Get(key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return e.Value, ok
}

// All returns a snapshot of all stored entries.
func (s *Store[K, V]) All() []Entry[V] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry[V], 0, len(s.data))
	for _, e := range s.data {
		out = append(out, e)
	}
	return out
}

// Delete removes the entry for a key.
func (s *Store[K, V]) Delete(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

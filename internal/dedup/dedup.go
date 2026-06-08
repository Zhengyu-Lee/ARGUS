package dedup

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Fingerprint computes a stable dedup key from raw data fields.
// Uses SHA256 of (normalized URL + title + first 200 chars of content).
func Fingerprint(url, title, content string) string {
	normalized := strings.TrimSpace(strings.ToLower(url)) + "|" +
		strings.TrimSpace(strings.ToLower(title)) + "|" +
		truncate(strings.TrimSpace(content), 200)

	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

type Store interface {
	// Seen returns true if the fingerprint was already recorded.
	// If not seen, records it and returns false.
	Seen(fp string) (bool, error)
	// Close cleans up resources.
	Close() error
}

// ── In-Memory Store (PoC / single instance) ──

type MemoryStore struct {
	mu   sync.RWMutex
	seen map[string]bool
	ttl  time.Duration // 0 = never expire
}

func NewMemoryStore(ttl time.Duration) *MemoryStore {
	return &MemoryStore{
		seen: make(map[string]bool),
		ttl:  ttl,
	}
}

func (s *MemoryStore) Seen(fp string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[fp] {
		return true, nil
	}
	s.seen[fp] = true
	return false, nil
}

func (s *MemoryStore) Close() error { return nil }

// ── File-Based Store (persistent across restarts) ──

type FileStore struct {
	path string
	f    *os.File
	mu   sync.RWMutex
	seen map[string]bool
}

func NewFileStore(dir string) (*FileStore, error) {
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "dedup.db")

	seen := make(map[string]bool)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("open dedup db: %w", err)
	}

	// Load existing entries
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			seen[line] = true
		}
	}

	return &FileStore{
		path: path,
		f:    f,
		seen: seen,
	}, nil
}

func (s *FileStore) Seen(fp string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.seen[fp] {
		return true, nil
	}

	// Record new fingerprint
	s.seen[fp] = true
	if _, err := s.f.WriteString(fp + "\n"); err != nil {
		return false, fmt.Errorf("write dedup: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		return false, fmt.Errorf("sync dedup: %w", err)
	}
	return false, nil
}

func (s *FileStore) Close() error {
	return s.f.Close()
}

// ── Redis Store (production distributed) ──

// RedisStore uses Redis SETNX for atomic dedup across workers.
// The bloom filter approach would be more memory-efficient for large scale,
// but for MVP a simple Redis SET with TTL is clearer.
type RedisStore struct {
	// Fields left empty; Redis support added when redis client is available.
	// For now, falls back to memory store.
	memory *MemoryStore
}

func NewRedisStore() *RedisStore {
	return &RedisStore{memory: NewMemoryStore(24 * time.Hour)}
}

func (s *RedisStore) Seen(fp string) (bool, error) {
	return s.memory.Seen(fp)
}

func (s *RedisStore) Close() error { return s.memory.Close() }

// ── Helpers ──

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

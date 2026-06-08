package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CacheKey defines what "same input" means for LLM cache.
// Three strategies in priority order:
//
//  1. URL-based: 同一 URL 直接命中（最强信号）
//  2. Content-based: 不同源的同一篇文章，内容不同但语义相同
//  3. URL+Content: URL 和内容都匹配才命中（最保守）
//
// 同一篇文章被不同源采集时的典型差异：
//   - RSS 源：摘要文本，无 HTML
//   - 网页抓取：带 HTML 标签，可能有广告/导航
//   - 群控抓取：可能含 OCR 误差
// 因此用 NormalizedContent() 做归一化后再指纹。

type CacheKey struct {
	URL         string `json:"url"`
	ContentHash string `json:"content_hash"`
}

func (k CacheKey) String() string {
	return k.URL + "|" + k.ContentHash
}

// ComputeCacheKey generates a cache key from raw text data.
// URL 优先；无 URL 时用归一化内容的 SHA256。
func ComputeCacheKey(url, content string) CacheKey {
	return CacheKey{
		URL:         normalizeURL(url),
		ContentHash: sha256Hex(normalizeContent(content)),
	}
}

// normalizeURL lowercases and strips trailing slash for consistent matching.
func normalizeURL(url string) string {
	u := strings.TrimSpace(strings.ToLower(url))
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimSuffix(u, "?utm_source=rss")
	u = strings.TrimSuffix(u, "?feed=rss")
	// Strip common tracking params
	u = stripTrackingParams(u)
	return u
}

func stripTrackingParams(url string) string {
	// Simple approach: remove common query params
	if idx := strings.Index(url, "?utm_"); idx >= 0 {
		url = url[:idx]
	}
	if idx := strings.Index(url, "?ref="); idx >= 0 {
		url = url[:idx]
	}
	return url
}

// normalizeContent strips HTML, normalizes whitespace, and truncates.
// 同一篇文章即使来自不同源，归一化后内容应高度相似。
func normalizeContent(content string) string {
	// 1. Strip HTML tags
	clean := stripHTMLTags(content)
	// 2. Normalize whitespace
	clean = normalizeWhitespace(clean)
	// 3. Truncate to 500 chars for cache key (long enough for uniqueness)
	runes := []rune(clean)
	if len(runes) > 500 {
		clean = string(runes[:500])
	}
	return strings.ToLower(clean)
}

func stripHTMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func normalizeWhitespace(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ── Cache Store ──

type CacheEntry struct {
	Result    string    `json:"result"`
	CachedAt  time.Time `json:"cached_at"`
	Key       CacheKey  `json:"key"`
}

type CacheStore interface {
	Get(key CacheKey) (*CacheEntry, bool)
	Set(key CacheKey, result string)
	Close() error
}

type MemoryCache struct {
	mu    sync.RWMutex
	store map[string]*CacheEntry
	ttl   time.Duration
}

func NewMemoryCache(ttl time.Duration) *MemoryCache {
	return &MemoryCache{
		store: make(map[string]*CacheEntry),
		ttl:   ttl,
	}
}

func (c *MemoryCache) Get(key CacheKey) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.store[key.String()]
	if !ok {
		return nil, false
	}
	if c.ttl > 0 && time.Since(entry.CachedAt) > c.ttl {
		delete(c.store, key.String())
		return nil, false
	}
	return entry, true
}

func (c *MemoryCache) Set(key CacheKey, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key.String()] = &CacheEntry{
		Result:   result,
		CachedAt: time.Now(),
		Key:      key,
	}
}

func (c *MemoryCache) Close() error { return nil }

// ── File Cache (persistent) ──

type FileCache struct {
	dir   string
	ttl   time.Duration
}

func NewFileCache(dir string, ttl time.Duration) *FileCache {
	os.MkdirAll(dir, 0755)
	return &FileCache{dir: dir, ttl: ttl}
}

func (c *FileCache) Get(key CacheKey) (*CacheEntry, bool) {
	path := filepath.Join(c.dir, key.String()+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if c.ttl > 0 && time.Since(entry.CachedAt) > c.ttl {
		os.Remove(path)
		return nil, false
	}
	return &entry, true
}

func (c *FileCache) Set(key CacheKey, result string) {
	entry := CacheEntry{
		Result:   result,
		CachedAt: time.Now(),
		Key:      key,
	}
	data, _ := json.Marshal(entry)
	path := filepath.Join(c.dir, key.String()+".json")
	os.WriteFile(path, data, 0644)
}

func (c *FileCache) Close() error { return nil }

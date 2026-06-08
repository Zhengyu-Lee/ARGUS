package llm

import (
	"fmt"
	"log"
	"strings"
	"time"
)

type LLMService struct {
	cache   CacheStore
	client  *httpClient
	enabled bool
}

type httpClient struct {
	url   string
	token string
}

func NewService(cacheType, cacheDir, llmURL, llmToken string) *LLMService {
	var cache CacheStore
	switch cacheType {
	case "file":
		cache = NewFileCache(cacheDir, 7*24*time.Hour) // 7 days TTL
		log.Printf("[llm] file cache: %s (7d TTL)", cacheDir)
	case "memory":
		cache = NewMemoryCache(24 * time.Hour) // 24h TTL
		log.Printf("[llm] memory cache (24h TTL)")
	default:
		cache = NewMemoryCache(24 * time.Hour)
	}

	svc := &LLMService{
		cache:   cache,
		enabled: llmURL != "",
	}

	if svc.enabled {
		svc.client = &httpClient{url: llmURL, token: llmToken}
		log.Printf("[llm] service enabled: %s", llmURL)
	} else {
		log.Printf("[llm] service disabled (no URL configured)")
	}

	return svc
}

// Analyze checks cache first, then calls LLM if not cached.
// Cache key 策略：
//   1. URL 优先：同一 URL 直接命中
//   2. 内容归一化：Strip HTML → 正则化空白 → SHA256
//   3. URL + 内容哈希联合作为 key
func (s *LLMService) Analyze(url, content string) (string, error) {
	key := ComputeCacheKey(url, content)

	// 1. Check cache
	if entry, found := s.cache.Get(key); found {
		log.Printf("[llm] cache HIT: url=%q, content_hash=%s", key.URL, key.ContentHash[:12])
		return entry.Result, nil
	}
	log.Printf("[llm] cache MISS: url=%q, content_hash=%s", key.URL, key.ContentHash[:12])

	if !s.enabled {
		result := fmt.Sprintf("mock: analyzed content (hash=%s)", key.ContentHash[:12])
		s.cache.Set(key, result)
		return result, nil
	}

	// 2. Call LLM
	result, err := s.callLLM(content)
	if err != nil {
		return "", fmt.Errorf("llm call: %w", err)
	}

	// 3. Cache result
	s.cache.Set(key, result)
	return result, nil
}

func (s *LLMService) callLLM(content string) (string, error) {
	// TODO: actual HTTP call to LLM API
	// For now, return a mock
	return fmt.Sprintf("entity_extraction: %d entities found", strings.Count(content, " ")), nil
}

func (s *LLMService) Close() error {
	return s.cache.Close()
}

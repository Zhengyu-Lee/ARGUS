package llm

import (
	"os"
	"testing"
)

func TestComputeCacheKeySameURL(t *testing.T) {
	// 同一 URL，内容不同 → URL 降噪仍应匹配
	k1 := ComputeCacheKey("https://example.com/news/123", "全文内容...")
	k2 := ComputeCacheKey("https://example.com/news/123?utm_source=rss", "全文内容（略有差异）")
	if k1.URL != k2.URL {
		t.Error("同URL应归一化为相同cache key")
	}
}

func TestComputeCacheKeyDifferentURL(t *testing.T) {
	// 不同 URL，相同内容 → 内容哈希应相同
	k1 := ComputeCacheKey("https://source-a.com/123", "某漏洞CVE-2026-1234的详细分析文章")
	k2 := ComputeCacheKey("https://source-b.com/456", "某漏洞CVE-2026-1234的详细分析文章")
	if k1.ContentHash != k2.ContentHash {
		t.Error("同内容应产生相同内容哈希")
	}
}

func TestCacheKeyHTMLvsPlain(t *testing.T) {
	// 同一篇文章：RSS 摘要（纯文本）vs 网页抓取（带 HTML）
	rssContent := "New vulnerability CVE-2026-1234 discovered in the wild"
	htmlContent := "<html><body><p>New vulnerability CVE-2026-1234 discovered in the wild</p></body></html>"

	k1 := ComputeCacheKey("", rssContent)
	k2 := ComputeCacheKey("", htmlContent)
	if k1.ContentHash != k2.ContentHash {
		t.Error("HTML 标签应被剥离，归一化后哈希应相同")
	}
}

func TestCacheKeyWhitespaceNormalization(t *testing.T) {
	// 空白差异
	sparse := "CVE-2026-1234    is    critical"
	dense := "CVE-2026-1234 is critical"

	k1 := ComputeCacheKey("", sparse)
	k2 := ComputeCacheKey("", dense)
	if k1.ContentHash != k2.ContentHash {
		t.Error("空白应归一化，哈希应相同")
	}
}

func TestMemoryCacheHitAndMiss(t *testing.T) {
	cache := NewMemoryCache(0)
	defer cache.Close()

	key := ComputeCacheKey("https://example.com/test", "test content")

	// Miss first
	_, found := cache.Get(key)
	if found {
		t.Error("首次查询应 miss")
	}

	// Set and get
	cache.Set(key, "analysis result")
	entry, found := cache.Get(key)
	if !found {
		t.Error("Set 后应 hit")
	}
	if entry.Result != "analysis result" {
		t.Errorf("结果不匹配: got %q", entry.Result)
	}
}

func TestFileCachePersistence(t *testing.T) {
	dir, _ := os.MkdirTemp("", "llm-cache-*")
	defer os.RemoveAll(dir)

	cache := NewFileCache(dir, 0)
	key := ComputeCacheKey("https://persist.test/1", "persistent content")
	cache.Set(key, "persisted result")
	cache.Close()

	// Re-open
	cache2 := NewFileCache(dir, 0)
	defer cache2.Close()
	entry, found := cache2.Get(key)
	if !found {
		t.Error("文件缓存应持久化")
	}
	if entry.Result != "persisted result" {
		t.Errorf("结果不匹配: got %q", entry.Result)
	}
}

func TestAnalyzeCache(t *testing.T) {
	svc := NewService("memory", "", "", "")
	defer svc.Close()

	// First call - should cache MISS then cache
	result1, err := svc.Analyze("https://example.com/1", "test article about CVE-2026-1234")
	if err != nil {
		t.Fatal(err)
	}

	// Second call with same URL - should cache HIT
	result2, _ := svc.Analyze("https://example.com/1", "test article about CVE-2026-1234 (different source)")
	if result1 != result2 {
		t.Error("同一URL应返回缓存结果")
	}

	// Different URL, same content - should cache MISS (different URL key)
	result3, _ := svc.Analyze("https://other-source.com/1", "test article about CVE-2026-1234")
	if result1 == result3 {
		t.Error("不同URL不应返回相同缓存结果")
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"HTTPS://EXAMPLE.COM/NEWS", "https://example.com/news"},
		{"https://example.com/news/", "https://example.com/news"},
		{"https://example.com/news?utm_source=rss", "https://example.com/news"},
		{"https://example.com/news?feed=rss", "https://example.com/news"},
	}
	for _, tt := range tests {
		got := normalizeURL(tt.input)
		if got != tt.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripHTMLTags(t *testing.T) {
	input := "<html><body><p>Hello <b>World</b></p><script>alert(1)</script></body></html>"
	want := "Hello Worldalert(1)"
	got := stripHTMLTags(input)
	if got != want {
		t.Errorf("stripHTML: got %q, want %q", got, want)
	}
}

// TestCrossSourceDedup 验证同一篇文章从三个不同源采集时的缓存命中
func TestCrossSourceDedup(t *testing.T) {
	// RSS 源：纯文本摘要
	rssContent := "A critical vulnerability CVE-2026-5678 has been discovered affecting major cloud providers"
	// 网页抓取：带 HTML
	webContent := "<html><body><article><h1>CVE Alert</h1><p>A critical vulnerability CVE-2026-5678 has been discovered affecting major cloud providers</p></article></body></html>"
	// 群控 OCR：可能有额外换行
	ocrContent := "A critical vulnerability\n\nCVE-2026-5678 has been\ndiscovered affecting\nmajor cloud providers"

	k1 := ComputeCacheKey("https://rss-source.com/alert1", rssContent)
	k2 := ComputeCacheKey("https://web-source.com/articles/999", webContent)
	k3 := ComputeCacheKey("https://device-ocr.local/capture/abc", ocrContent)

	// 三个不同 URL → URL key 不同
	if k1.URL == k2.URL {
		t.Error("不同源应有不同 URL")
	}

	// 但内容哈希应相同（归一化后）
	if k1.ContentHash != k2.ContentHash {
		t.Errorf("RSS 和网页的内容哈希应相同\n  RSS: %s\n  Web: %s", k1.ContentHash, k2.ContentHash)
	}
	if k1.ContentHash != k3.ContentHash {
		t.Errorf("RSS 和 OCR 的内容哈希应相同\n  RSS: %s\n  OCR: %s", k1.ContentHash, k3.ContentHash)
	}
}

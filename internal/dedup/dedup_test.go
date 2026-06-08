package dedup

import (
	"os"
	"testing"
)

func TestFingerprintDeterministic(t *testing.T) {
	a := Fingerprint("https://example.com/news", "Breaking CVE-2026", "A new vulnerability...")
	b := Fingerprint("https://example.com/news", "Breaking CVE-2026", "A new vulnerability...")
	if a != b {
		t.Error("fingerprint should be deterministic")
	}
}

func TestFingerprintDifferentURL(t *testing.T) {
	a := Fingerprint("https://example.com/news", "Breaking CVE-2026", "A new vulnerability...")
	b := Fingerprint("https://example.net/news", "Breaking CVE-2026", "A new vulnerability...")
	if a == b {
		t.Error("different URLs should produce different fingerprints")
	}
}

func TestFingerprintCaseInsensitive(t *testing.T) {
	a := Fingerprint("HTTPS://EXAMPLE.COM/NEWS", "Breaking CVE-2026", "content")
	b := Fingerprint("https://example.com/news", "Breaking CVE-2026", "content")
	if a != b {
		t.Error("fingerprint should be case-insensitive for URL")
	}
}

func TestMemoryStoreDedup(t *testing.T) {
	store := NewMemoryStore(0)
	defer store.Close()

	fp := "test-fingerprint-123"

	// First time: not seen
	seen, err := store.Seen(fp)
	if err != nil {
		t.Fatal(err)
	}
	if seen {
		t.Error("first occurrence should not be seen")
	}

	// Second time: seen
	seen, err = store.Seen(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Error("second occurrence should be seen")
	}
}

func TestFileStoreDedup(t *testing.T) {
	dir, _ := os.MkdirTemp("", "dedup-test-*")
	defer os.RemoveAll(dir)

	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	fp := "file-dedup-fingerprint"

	// First time
	seen, _ := store.Seen(fp)
	if seen {
		t.Error("first should not be seen")
	}
	store.Close()

	// Re-open from file - fingerprint should persist
	store2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	seen, _ = store2.Seen(fp)
	if !seen {
		t.Error("fingerprint should persist across restarts")
	}
}

func TestMemoryStoreNoFalsePositive(t *testing.T) {
	store := NewMemoryStore(0)
	defer store.Close()

	fp1 := "unique-fp-alpha"
	fp2 := "unique-fp-beta"

	seen1, _ := store.Seen(fp1)
	seen2, _ := store.Seen(fp2)

	if seen1 || seen2 {
		t.Error("different fingerprints should not collide")
	}
}

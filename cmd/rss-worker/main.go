package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/types"
	"github.com/google/uuid"
)

// Minimal RSS/Atom parser (no external dependency needed for basic feeds)
type RSS struct {
	Channel Channel `xml:"channel"`
}

type Channel struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Items       []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Creator     string `xml:"dc:creator"`
}

type FeedSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Tags string `json:"tags"`
}

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	interval := getEnvInt("FETCH_INTERVAL", 300) // seconds
	groupID := "rss-worker"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	// Built-in feed list (configurable via env)
	feeds := getFeeds()

	log.Printf("[rss-worker] started, %d feeds, interval=%ds", len(feeds), interval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	client := &http.Client{Timeout: 30 * time.Second}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// Initial fetch
	fetchAll(ctx, client, bus, feeds)

	for {
		select {
		case <-ticker.C:
			fetchAll(ctx, client, bus, feeds)
		case <-ctx.Done():
			log.Println("[rss-worker] stopping")
			return
		}
	}
}

func fetchAll(ctx context.Context, client *http.Client, bus *broker.MessageBus, feeds []FeedSource) {
	for _, feed := range feeds {
		select {
		case <-ctx.Done():
			return
		default:
		}

		log.Printf("[rss-worker] fetching %s (%s)", feed.Name, feed.URL)
		items, err := fetchFeed(client, feed.URL)
		if err != nil {
			log.Printf("[rss-worker] error fetching %s: %v", feed.Name, err)
			continue
		}

		for _, item := range items {
			raw := types.RawData{
				ID:        uuid.New().String(),
				Source:    "rss",
				Platform:  feed.Name,
				URL:       item.Link,
				Title:     item.Title,
				Content:   cleanHTML(item.Description),
				Author:    item.Creator,
				Published: parseTime(item.PubDate),
				Collected: time.Now(),
				Metadata: map[string]string{
					"feed_url": feed.URL,
					"tags":     feed.Tags,
				},
			}

			if err := bus.Publish("raw-data", raw.ID, raw); err != nil {
				log.Printf("[rss-worker] publish error: %v", err)
			}
		}
		log.Printf("[rss-worker] %s: published %d items", feed.Name, len(items))
	}
}

func fetchFeed(client *http.Client, url string) ([]Item, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var feed RSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse xml: %w", err)
	}

	return feed.Channel.Items, nil
}

func parseTime(s string) time.Time {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"Mon, 02 Jan 2006 15:04:05 MST",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func cleanHTML(s string) string {
	s = strings.ReplaceAll(s, "<p>", " ")
	s = strings.ReplaceAll(s, "</p>", " ")
	s = strings.ReplaceAll(s, "<br>", " ")
	s = strings.ReplaceAll(s, "<br/>", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	// Remove remaining tags
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
	return strings.TrimSpace(result.String())
}

func getFeeds() []FeedSource {
	// Default feeds - override with RSS_FEEDS env var
	env := os.Getenv("RSS_FEEDS")
	if env != "" {
		var feeds []FeedSource
		for _, line := range strings.Split(env, ",") {
			parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
			if len(parts) >= 2 {
				f := FeedSource{Name: parts[0], URL: parts[1]}
				if len(parts) >= 3 {
					f.Tags = parts[2]
				}
				feeds = append(feeds, f)
			}
		}
		if len(feeds) > 0 {
			return feeds
		}
	}

	return []FeedSource{
		{Name: "thehackernews", URL: "https://feeds.feedburner.com/TheHackerNews", Tags: "security"},
		{Name: "krebsonsecurity", URL: "https://krebsonsecurity.com/feed/", Tags: "security"},
		{Name: "bleepingcomputer", URL: "https://www.bleepingcomputer.com/feed/", Tags: "security"},
		{Name: "schneier", URL: "https://www.schneier.com/feed/", Tags: "security"},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return fallback
}

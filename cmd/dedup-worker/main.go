package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/dedup"
	"github.com/argus-platform/argus/internal/types"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	storeType := getEnv("DEDUP_STORE", "memory")    // memory, file, redis
	storeDir := getEnv("DEDUP_DIR", "./data/dedup") // for file store
	groupID := "dedup-worker"

	var store dedup.Store
	switch storeType {
	case "file":
		var err error
		store, err = dedup.NewFileStore(storeDir)
		if err != nil {
			log.Fatalf("create file store: %v", err)
		}
		log.Printf("[dedup-worker] using file store: %s", storeDir)
	case "redis":
		store = dedup.NewRedisStore()
		log.Printf("[dedup-worker] using redis store (memory fallback)")
	default:
		store = dedup.NewMemoryStore(24 * time.Hour)
		log.Printf("[dedup-worker] using memory store (24h TTL)")
	}
	defer store.Close()

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	log.Println("[dedup-worker] started, consuming from raw-data...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	stats := struct {
		total   int
		dup     int
		passed  int
		errored int
	}{}

	err = bus.Consume(ctx, []string{"raw-data"}, func(msg []byte) error {
		var raw types.RawData
		if err := json.Unmarshal(msg, &raw); err != nil {
			log.Printf("unmarshal error: %v", err)
			stats.errored++
			return nil // skip bad messages
		}
		stats.total++

		// Compute fingerprint from URL + title + content
		fp := dedup.Fingerprint(raw.URL, raw.Title, raw.Content)

		seen, err := store.Seen(fp)
		if err != nil {
			log.Printf("dedup check error: %v", err)
			stats.errored++
			// On error, let it through (fail open)
			return bus.Publish("enriched", raw.ID, types.EnrichedData{RawData: raw})
		}

		if seen {
			stats.dup++
			if stats.dup%100 == 1 {
				log.Printf("[dedup-worker] %s: duplicate (total=%d, dup=%d, pass=%d)",
					raw.ID, stats.total, stats.dup, stats.passed)
			}
			return nil // drop duplicate
		}

		stats.passed++
		return bus.Publish("enriched", raw.ID, types.EnrichedData{RawData: raw})
	})

	log.Printf("[dedup-worker] stopped: %v", err)
	log.Printf("[dedup-worker] final stats: total=%d, dup=%d, passed=%d, errors=%d",
		stats.total, stats.dup, stats.passed, stats.errored)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

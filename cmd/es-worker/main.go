package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/types"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	esURL := getEnv("ES_URL", "")          // empty = file fallback
	dataDir := getEnv("DATA_DIR", "./data") // file fallback directory
	groupID := "es-worker"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	os.MkdirAll(dataDir, 0755)
	log.Printf("[es-worker] started, consuming from es-store (es_url=%q, fallback_dir=%q)", esURL, dataDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	err = bus.Consume(ctx, []string{"es-store"}, func(msg []byte) error {
		var data types.EnrichedData
		if err := json.Unmarshal(msg, &data); err != nil {
			log.Printf("unmarshal error: %v", err)
			return nil
		}

		if esURL != "" {
			// Future: send to Elasticsearch bulk API
			log.Printf("[es-worker] would index to ES: %s (confidence=%d)", data.ID, data.Confidence)
		} else {
			// Fallback: write to JSON file
			date := data.Collected.Format("2006-01-02")
			dir := filepath.Join(dataDir, date)
			os.MkdirAll(dir, 0755)

			path := filepath.Join(dir, data.ID+".json")
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()

			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			if err := enc.Encode(data); err != nil {
				return err
			}
			log.Printf("[es-worker] saved %s (confidence=%d, iocs=%d)", path, data.Confidence, len(data.IOCs))
		}
		return nil
	})
	log.Printf("[es-worker] stopped: %v", err)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

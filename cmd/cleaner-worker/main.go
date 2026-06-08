package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/extractor"
	"github.com/argus-platform/argus/internal/types"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	groupID := "cleaner-worker"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	log.Println("[cleaner-worker] started, consuming from enriched...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	err = bus.Consume(ctx, []string{"enriched"}, func(msg []byte) error {
		var data types.EnrichedData
		if err := json.Unmarshal(msg, &data); err != nil {
			log.Printf("unmarshal error: %v", err)
			return nil // skip bad messages
		}

		// Extract IOCs
		iocs := extractor.ExtractIOCs(data.Content)
		if len(iocs) > 0 {
			data.IOCs = append(data.IOCs, iocs...)
			data.Tags = append(data.Tags, "has-ioc")
			data.Confidence += 15
		}

		return bus.Publish("ioc-candidate", data.ID, data)
	})
	log.Printf("[cleaner-worker] stopped: %v", err)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

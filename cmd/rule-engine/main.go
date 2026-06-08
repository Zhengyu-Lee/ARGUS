package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/rules"
	"github.com/argus-platform/argus/internal/types"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	groupID := "rule-engine"
	rulesPath := getEnv("RULES_PATH", "/etc/argus/rules.yaml")

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	engine, err := rules.NewEngine(rulesPath)
	if err != nil {
		log.Fatalf("load rules: %v", err)
	}

	log.Printf("[rule-engine] started, consuming from ioc-candidate...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	err = bus.Consume(ctx, []string{"ioc-candidate"}, func(msg []byte) error {
		var data types.EnrichedData
		if err := json.Unmarshal(msg, &data); err != nil {
			log.Printf("unmarshal error: %v", err)
			return nil
		}

		results := engine.Evaluate(&data)
		if len(results) == 0 {
			log.Printf("[rule-engine] no rules matched for %s, storing to ES only", data.ID)
			return bus.Publish("es-store", data.ID, data)
		}

		// Collect all target topics and tags from matched rules
		publishTopics := make(map[string]bool)
		hasAlert := false

		for _, r := range results {
			data.Tags = append(data.Tags, r.RuleName)
			data.Confidence += r.ConfidenceBoost

			for _, topic := range r.TargetTopics {
				publishTopics[topic] = true
			}
			if r.TriggerAlert {
				hasAlert = true
			}
		}

		log.Printf("[rule-engine] %s: confidence=%d, topics=%v, tags=%v",
			data.ID, data.Confidence, mapKeys(publishTopics), data.Tags)

		// Publish to each target topic
		for topic := range publishTopics {
			if err := bus.Publish(topic, data.ID, data); err != nil {
				log.Printf("[rule-engine] publish to %s error: %v", topic, err)
			}
		}

		// Trigger alert if any rule requested it
		if hasAlert {
			bus.Publish("alert", data.ID, data)
		}

		return nil
	})
	log.Printf("[rule-engine] stopped: %v", err)
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

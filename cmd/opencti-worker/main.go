package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/types"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	openctiURL := getEnv("OPENCTI_URL", "http://localhost:4000")
	openctiToken := getEnv("OPENCTI_TOKEN", "")
	groupID := "opencti-worker"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	client := &http.Client{}
	log.Printf("[opencti-worker] started, pushing to OpenCTI at %s", openctiURL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	err = bus.Consume(ctx, []string{"ioc-confirmed"}, func(msg []byte) error {
		var data types.EnrichedData
		if err := json.Unmarshal(msg, &data); err != nil {
			return err
		}

		// Create STIX2 Report via OpenCTI GraphQL API
		query := map[string]any{
			"query": `
				mutation CreateReport($input: ReportAddInput!) {
					reportAdd(input: $input) {
						id
						name
					}
				}
			`,
			"variables": map[string]any{
				"input": map[string]any{
					"name":        data.Title,
					"description": data.Content,
					"published":   data.Published,
					"objects":     buildObservables(data.IOCs),
				},
			},
		}

		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", openctiURL+"/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+openctiToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("opencti push error: %v", err)
			return err
		}
		defer resp.Body.Close()

		log.Printf("[opencti-worker] pushed report: %s", data.ID)
		return nil
	})
	log.Printf("[opencti-worker] stopped: %v", err)
}

func buildObservables(iocs []types.IOC) []map[string]any {
	obs := make([]map[string]any, 0, len(iocs))
	for _, ioc := range iocs {
		obs = append(obs, map[string]any{
			"observable_value": ioc.Value,
			"observable_type":  ioc.Type,
		})
	}
	return obs
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

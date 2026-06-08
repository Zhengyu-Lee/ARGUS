package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/types"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	mispURL := getEnv("MISP_URL", "http://localhost:8080")
	mispKey := getEnv("MISP_KEY", "")
	groupID := "misp-worker"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	client := &http.Client{}
	log.Printf("[misp-worker] started, pushing to MISP at %s", mispURL)

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

		// Build MISP event payload
		event := map[string]any{
			"Event": map[string]any{
				"info":       data.Title,
				"distribution": "3",
				"analysis":   "2",
				"Attribute":  buildAttributes(data.IOCs),
			},
		}

		body, _ := json.Marshal(event)
		req, _ := http.NewRequest("POST", mispURL+"/events", strings.NewReader(string(body)))
		req.Header.Set("Authorization", mispKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("misp push error: %v", err)
			return err
		}
		defer resp.Body.Close()

		log.Printf("[misp-worker] pushed event: %s", data.ID)
		return nil
	})
	log.Printf("[misp-worker] stopped: %v", err)
}

func buildAttributes(iocs []types.IOC) []map[string]any {
	attrs := make([]map[string]any, 0, len(iocs))
	for _, ioc := range iocs {
		attrs = append(attrs, map[string]any{
			"type":  iocTypeToMISP(ioc.Type),
			"value": ioc.Value,
		})
	}
	return attrs
}

func iocTypeToMISP(t string) string {
	switch t {
	case "ip":
		return "ip-dst"
	case "domain":
		return "domain"
	case "url":
		return "url"
	case "hash":
		return "md5"
	case "cve":
		return "vulnerability"
	case "email":
		return "email-src"
	default:
		return "text"
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

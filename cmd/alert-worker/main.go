package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/types"
)

type Alert struct {
	ID        string    `json:"id"`
	RuleName  string    `json:"rule_name"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Confidence int      `json:"confidence"`
	IOCs      []types.IOC `json:"iocs"`
	Triggered time.Time `json:"triggered"`
}

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	slackWebhook := getEnv("SLACK_WEBHOOK", "")
	groupID := "alert-worker"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	log.Printf("[alert-worker] started, consuming from alert (slack=%t)", slackWebhook != "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	err = bus.Consume(ctx, []string{"alert"}, func(msg []byte) error {
		var data types.EnrichedData
		if err := json.Unmarshal(msg, &data); err != nil {
			log.Printf("unmarshal error: %v", err)
			return nil
		}

		alert := Alert{
			ID:         data.ID,
			Title:      data.Title,
			Content:    truncate(data.Content, 200),
			Confidence: data.Confidence,
			IOCs:       data.IOCs,
			Triggered:  time.Now(),
		}

		// Console alert
		log.Printf("=== ALERT === %s (confidence=%d)", alert.Title, alert.Confidence)
		for _, ioc := range alert.IOCs {
			log.Printf("  IOC [%s]: %s", ioc.Type, ioc.Value)
		}

		// Slack webhook (if configured)
		if slackWebhook != "" {
			payload := map[string]string{
				"text": fmt.Sprintf(":rotating_light: *ARGUS Alert*\n*%s*\nConfidence: %d\nIOCs: %v",
					alert.Title, alert.Confidence, formatIOCs(alert.IOCs)),
			}
			body, _ := json.Marshal(payload)
			log.Printf("[alert-worker] would send to Slack: %s", string(body))
			// TODO: actual HTTP POST to slackWebhook
		}

		return nil
	})
	log.Printf("[alert-worker] stopped: %v", err)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func formatIOCs(iocs []types.IOC) string {
	s := ""
	for i, ioc := range iocs {
		if i > 5 {
			s += fmt.Sprintf(" ... +%d more", len(iocs)-5)
			break
		}
		s += fmt.Sprintf("%s:%s ", ioc.Type, ioc.Value)
	}
	return s
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

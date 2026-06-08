package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"

	"github.com/argus-platform/argus/internal/activity"
	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/types"
	"github.com/argus-platform/argus/internal/workflow"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	temporalHost := getEnv("TEMPORAL_HOST", "localhost:7233")
	groupID := "temporal-worker"

	// 1. Connect to Temporal
	tc, err := client.Dial(client.Options{
		HostPort: temporalHost,
	})
	if err != nil {
		log.Fatalf("connect temporal: %v", err)
	}
	defer tc.Close()

	// 2. Register workflow + activities
	w := worker.New(tc, workflow.ReviewQueue, worker.Options{})
	w.RegisterWorkflow(workflow.IoCReviewWorkflow)
	w.RegisterActivity(activity.NotifyReviewerActivity)
	w.RegisterActivity(activity.PushToMISPActivity)
	w.RegisterActivity(activity.PushToOpenCTIActivity)
	w.RegisterActivity(activity.PushToMISPWithTagActivity)
	w.RegisterActivity(activity.EscalateReviewActivity)

	if err := w.Start(); err != nil {
		log.Fatalf("start temporal worker: %v", err)
	}
	log.Printf("[temporal-worker] started, queue=%s, temporal=%s", workflow.ReviewQueue, temporalHost)

	// 3. Connect to Kafka
	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
		w.Stop()
	}()

	// 4. Consume human-review topic and start workflows
	err = bus.Consume(ctx, []string{"human-review"}, func(msg []byte) error {
		var data types.EnrichedData
		if err := json.Unmarshal(msg, &data); err != nil {
			log.Printf("unmarshal error: %v", err)
			return nil
		}

		_, err := tc.ExecuteWorkflow(ctx,
			client.StartWorkflowOptions{
				TaskQueue: workflow.ReviewQueue,
				ID:        "review-" + data.ID,
			},
			workflow.IoCReviewWorkflow,
			data,
		)
		if err != nil {
			log.Printf("start workflow error: %v", err)
			return nil
		}
		log.Printf("[temporal-worker] started workflow review-%s (confidence=%d)", data.ID, data.Confidence)
		return nil
	})
	log.Printf("[temporal-worker] stopped: %v", err)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
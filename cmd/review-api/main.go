package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/types"
	"github.com/argus-platform/argus/internal/workflow"
	"go.temporal.io/sdk/client"
)

type ReviewItem struct {
	ID         string            `json:"id"`
	Data       types.EnrichedData `json:"data"`
	Received   time.Time         `json:"received"`
	Reviewed   bool              `json:"reviewed"`
	Decision   string            `json:"decision"`
	Reviewer   string            `json:"reviewer"`
	ReviewedAt *time.Time        `json:"reviewed_at,omitempty"`
}

type ReviewStore struct {
	mu    sync.RWMutex
	items map[string]*ReviewItem
}

func NewReviewStore() *ReviewStore {
	return &ReviewStore{items: make(map[string]*ReviewItem)}
}

func (s *ReviewStore) Add(item *ReviewItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[item.ID] = item
}

func (s *ReviewStore) List() []*ReviewItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*ReviewItem, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result
}

func (s *ReviewStore) Get(id string) *ReviewItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items[id]
}

func (s *ReviewStore) Approve(id, reviewer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	item.Reviewed = true
	item.Decision = "approved"
	item.Reviewer = reviewer
	now := time.Now()
	item.ReviewedAt = &now
	return nil
}

func (s *ReviewStore) Reject(id, reviewer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	item.Reviewed = true
	item.Decision = "rejected"
	item.Reviewer = reviewer
	now := time.Now()
	item.ReviewedAt = &now
	return nil
}

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	listenAddr := getEnv("LISTEN_ADDR", ":8090")
	temporalHost := getEnv("TEMPORAL_HOST", "localhost:7233")
	groupID := "review-api"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	temporalClient, err := client.Dial(client.Options{
		HostPort: temporalHost,
	})
	if err != nil {
		log.Fatalf("connect temporal: %v", err)
	}
	defer temporalClient.Close()

	store := NewReviewStore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := bus.Consume(ctx, []string{"human-review"}, func(msg []byte) error {
			var data types.EnrichedData
			if err := json.Unmarshal(msg, &data); err != nil {
				log.Printf("unmarshal error: %v", err)
				return nil
			}
			store.Add(&ReviewItem{
				ID:       data.ID,
				Data:     data,
				Received: time.Now(),
			})
			log.Printf("[review-api] new review item: %s (confidence=%d)", data.ID, data.Confidence)
			return nil
		})
		if err != nil && err != context.Canceled {
			log.Printf("consume error: %v", err)
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(store.List())
	})

	mux.HandleFunc("/api/review/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/api/review/"):]
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			item := store.Get(id)
			if item == nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(item)

		case http.MethodPost:
			var req struct {
				Action       string `json:"action"`
				Reviewer     string `json:"reviewer"`
				RejectReason string `json:"reject_reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var err error
			switch req.Action {
			case "approve":
				err = store.Approve(id, req.Reviewer)
				if err == nil {
					item := store.Get(id)
					bus.Publish("ioc-confirmed", id, item.Data)
					temporalClient.SignalWorkflow(ctx, id, "", workflow.SignalApprove, req.Reviewer)
					log.Printf("[review-api] signaled approve: %s", id)
				}
			case "reject":
				err = store.Reject(id, req.Reviewer)
				if err == nil {
					temporalClient.SignalWorkflow(ctx, id, "", workflow.SignalReject, req.RejectReason)
					log.Printf("[review-api] signaled reject: %s", id)
				}
			default:
				http.Error(w, "invalid action", http.StatusBadRequest)
				return
			}

			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
		server.Shutdown(context.Background())
	}()

	log.Printf("[review-api] HTTP server listening on %s", listenAddr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

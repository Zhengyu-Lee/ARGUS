package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/IBM/sarama"
)

type MessageBus struct {
	producer sarama.SyncProducer
	consumer sarama.ConsumerGroup
	brokers  []string
	groupID  string
}

func New(brokers []string, groupID string) (*MessageBus, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("create producer: %w", err)
	}

	consumer, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("create consumer: %w", err)
	}

	return &MessageBus{
		producer: producer,
		consumer: consumer,
		brokers:  brokers,
		groupID:  groupID,
	}, nil
}

func (mb *MessageBus) Publish(topic string, key string, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	_, _, err = mb.producer.SendMessage(&sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(data),
	})
	return err
}

func (mb *MessageBus) Consume(ctx context.Context, topics []string, handler func(msg []byte) error) error {
	ch := make(chan error, 1)

	go func() {
		for {
			err := mb.consumer.Consume(ctx, topics, &consumerHandler{handler: handler})
			if err != nil {
				ch <- fmt.Errorf("consume: %w", err)
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type consumerHandler struct {
	handler func([]byte) error
}

func (h *consumerHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }
func (h *consumerHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		if err := h.handler(msg.Value); err != nil {
			log.Printf("process message error: %v", err)
		}
		sess.MarkMessage(msg, "")
	}
	return nil
}

func (mb *MessageBus) Close() error {
	if err := mb.producer.Close(); err != nil {
		return err
	}
	return mb.consumer.Close()
}

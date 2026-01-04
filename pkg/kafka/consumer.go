package kafka

import (
	"context"
	"fmt"
	"sync"

	kafkago "github.com/segmentio/kafka-go"
)

type KafkaConsumer struct {
	reader *kafkago.Reader
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewKafkaConsumer(brokers []string, groupID, topic string) *KafkaConsumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  brokers,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})

	return &KafkaConsumer{reader: reader}
}

func (c *KafkaConsumer) Subscribe(ctx context.Context, topics []string, handler ConsumerHandler) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := c.reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					continue
				}

				headers := make(map[string]string)
				for _, h := range msg.Headers {
					headers[h.Key] = string(h.Value)
				}

				message := Message{
					Topic:   msg.Topic,
					Key:     msg.Key,
					Value:   msg.Value,
					Headers: headers,
				}

				if err := handler(message); err != nil {
					fmt.Printf("Error handling message: %v\n", err)
				}
			}
		}
	}()

	return nil
}

func (c *KafkaConsumer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}

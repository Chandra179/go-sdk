package kafka

import "errors"

var (
	ErrProducerNotInitialized = errors.New("producer not initialized")
	ErrConsumerNotInitialized = errors.New("consumer not initialized")
	ErrInvalidMessage         = errors.New("invalid message")
	ErrTopicNotFound          = errors.New("topic not found")
)

type Message struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers map[string]string
}

type ConsumerHandler func(msg Message) error

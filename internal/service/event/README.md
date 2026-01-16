# Message Broker Service

This service provides a simple REST API for publishing and consuming messages using Kafka.

## API Endpoints

### Publish Message

`POST /messages/publish`

Publishes a message to a Kafka topic.

**Request Body:**
```json
{
  "topic": "my-topic",
  "key": "optional-key",
  "value": "message content"
}
```

**Response:**
```json
{
  "message": "message published successfully"
}
```

### Subscribe to Topic

`POST /messages/subscribe`

Subscribes to a Kafka topic.

**Request Body:**
```json
{
  "topic": "my-topic",
  "group_id": "my-consumer-group"
}
```

**Response:**
```json
{
  "message": "subscribed successfully",
  "subscription_id": "uuid-here"
}
```

### Unsubscribe

`DELETE /messages/subscribe/:id`

Unsubscribes from a topic using the subscription ID.

**Response:**
```json
{
  "message": "unsubscribed successfully"
}
```

## Usage

The service is automatically initialized when the server starts and registers the following routes:

- `/messages/publish` - Publish messages to Kafka topics
- `/messages/subscribe` - Subscribe to Kafka topics
- `/messages/subscribe/:id` - Unsubscribe from a topic

Messages are logged to the console when received by the consumer.

package messagebroker

type PublishRequest struct {
	Topic string `json:"topic" binding:"required"`
	Key   string `json:"key"`
	Value string `json:"value" binding:"required"`
}

type SubscribeRequest struct {
	Topic   string `json:"topic" binding:"required"`
	GroupID string `json:"group_id" binding:"required"`
}

type MessageResponse struct {
	Topic   string            `json:"topic"`
	Key     string            `json:"key"`
	Value   string            `json:"value"`
	Headers map[string]string `json:"headers"`
}

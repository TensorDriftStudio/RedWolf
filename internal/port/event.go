package port

// EventBroadcaster delivers real-time telemetry updates to connected dashboard clients.
type EventBroadcaster interface {
	Publish(event string, payload any)
	Subscribe() (<-chan []byte, func())
}

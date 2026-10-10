package event

import (
	"encoding/json"
	"sync"

	"github.com/tensordriftstudio/redwolf/internal/port"
)

var _ port.EventBroadcaster = (*Broadcaster)(nil)

// Message defines the structure of real-time events.
type Message struct {
	Event   string `json:"event"`
	Payload any    `json:"payload"`
}

// Broadcaster manages in-memory event distribution to concurrent dashboard clients.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[chan []byte]struct{}
}

// NewBroadcaster creates an initialized event broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[chan []byte]struct{}),
	}
}

// Publish serializes and fans out an event payload to all active subscribers.
func (b *Broadcaster) Publish(event string, payload any) {
	msg := Message{
		Event:   event,
		Payload: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers {
		select {
		case ch <- data:
		default:
			// Non-blocking drop for slow consumers to prevent pipeline stall
		}
	}
}

// Subscribe registers a new event listener and returns a read-only channel and unsubscribe function.
func (b *Broadcaster) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 32)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, ch)
			close(ch)
			b.mu.Unlock()
		})
	}

	return ch, unsubscribe
}

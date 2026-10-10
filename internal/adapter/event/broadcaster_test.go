package event

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBroadcaster_PublishSubscribe(t *testing.T) {
	b := NewBroadcaster()

	ch, unsubscribe := b.Subscribe()
	defer unsubscribe()

	testData := map[string]string{"node_id": "test-123"}
	b.Publish("node:updated", testData)

	select {
	case msg := <-ch:
		var parsed Message
		if err := json.Unmarshal(msg, &parsed); err != nil {
			t.Fatalf("failed unmarshaling message: %v", err)
		}
		if parsed.Event != "node:updated" {
			t.Fatalf("expected 'node:updated', got '%s'", parsed.Event)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroadcaster_UnsubscribeIdempotent(t *testing.T) {
	b := NewBroadcaster()

	_, unsubscribe := b.Subscribe()

	// Calling unsubscribe multiple times must not panic
	unsubscribe()
	unsubscribe()
	unsubscribe()
}

func TestBroadcaster_SlowConsumerDrop(t *testing.T) {
	b := NewBroadcaster()

	ch, unsubscribe := b.Subscribe()
	defer unsubscribe()

	// Fill subscriber channel (capacity 32)
	for i := 0; i < 40; i++ {
		b.Publish("tick", map[string]int{"seq": i})
	}

	// Verify channel didn't block and received messages
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != 32 {
		t.Fatalf("expected 32 buffered messages, got %d", count)
	}
}

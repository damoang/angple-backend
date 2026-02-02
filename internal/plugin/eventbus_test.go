package plugin

import (
	"sync"
	"testing"
	"time"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	logger := NewDefaultLogger("test")
	eb := NewEventBus(logger)

	var received Event
	eb.Subscribe("plugin-a", "user.created", func(e Event) {
		received = e
	})

	eb.Publish("plugin-b", "user.created", map[string]interface{}{"user_id": "123"})

	if received.Topic != "user.created" {
		t.Fatalf("expected topic user.created, got %s", received.Topic)
	}
	if received.Source != "plugin-b" {
		t.Fatalf("expected source plugin-b, got %s", received.Source)
	}
	if received.Payload["user_id"] != "123" {
		t.Fatalf("expected user_id 123, got %v", received.Payload["user_id"])
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	logger := NewDefaultLogger("test")
	eb := NewEventBus(logger)

	var count int
	var mu sync.Mutex

	handler := func(_ Event) {
		mu.Lock()
		count++
		mu.Unlock()
	}

	eb.Subscribe("plugin-a", "order.placed", handler)
	eb.Subscribe("plugin-b", "order.placed", handler)
	eb.Subscribe("plugin-c", "order.placed", handler)

	eb.Publish("shop", "order.placed", nil)

	if count != 3 {
		t.Fatalf("expected 3 handlers called, got %d", count)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	logger := NewDefaultLogger("test")
	eb := NewEventBus(logger)

	var called bool
	eb.Subscribe("plugin-a", "test.event", func(_ Event) {
		called = true
	})

	eb.Unsubscribe("plugin-a")
	eb.Publish("source", "test.event", nil)

	if called {
		t.Fatal("expected handler NOT to be called after unsubscribe")
	}
}

func TestEventBus_NoSubscribers(t *testing.T) {
	logger := NewDefaultLogger("test")
	eb := NewEventBus(logger)

	// Should not panic
	eb.Publish("source", "no.subscribers", nil)
}

func TestEventBus_HandlerPanic(t *testing.T) {
	logger := NewDefaultLogger("test")
	eb := NewEventBus(logger)

	var secondCalled bool
	eb.Subscribe("bad-plugin", "test", func(_ Event) {
		panic("handler crash")
	})
	eb.Subscribe("good-plugin", "test", func(_ Event) {
		secondCalled = true
	})

	// Should not panic, and second handler should still run
	eb.Publish("source", "test", nil)

	if !secondCalled {
		t.Fatal("expected second handler to be called despite first panic")
	}
}

func TestEventBus_GetSubscriptions(t *testing.T) {
	logger := NewDefaultLogger("test")
	eb := NewEventBus(logger)

	eb.Subscribe("a", "topic1", func(_ Event) {})
	eb.Subscribe("b", "topic1", func(_ Event) {})
	eb.Subscribe("c", "topic2", func(_ Event) {})

	subs := eb.GetSubscriptions()
	if len(subs["topic1"]) != 2 {
		t.Fatalf("expected 2 subscribers for topic1, got %d", len(subs["topic1"]))
	}
	if len(subs["topic2"]) != 1 {
		t.Fatalf("expected 1 subscriber for topic2, got %d", len(subs["topic2"]))
	}
}

func TestEventBus_PublishAsync(t *testing.T) {
	logger := NewDefaultLogger("test")
	eb := NewEventBus(logger)

	var received bool
	var mu sync.Mutex
	eb.Subscribe("plugin-a", "async.test", func(_ Event) {
		mu.Lock()
		received = true
		mu.Unlock()
	})

	eb.PublishAsync("source", "async.test", nil)

	// Wait for async handler
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Fatal("expected async handler to be called")
	}
}

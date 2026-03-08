package event

import (
	"context"
	"log/slog"
	"sync"
)

// InMemoryBus is a synchronous, in-process event bus. Handlers are called in
// subscription order. A handler error is logged but does not block subsequent
// handlers or the caller.
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[Type][]Handler
}

// NewInMemoryBus creates a new in-memory event bus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[Type][]Handler),
	}
}

func (b *InMemoryBus) Subscribe(eventType Type, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *InMemoryBus) Publish(ctx context.Context, evt Event) {
	b.mu.RLock()
	handlers := b.handlers[evt.EventType()]
	b.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, evt); err != nil {
			slog.Error("event handler failed",
				"event_type", string(evt.EventType()),
				"error", err,
			)
		}
	}
}

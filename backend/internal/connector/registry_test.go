package connector

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeConnector struct {
	kind      string
	singleton bool
}

func (f fakeConnector) Kind() string                         { return f.kind }
func (f fakeConnector) Singleton() bool                      { return f.singleton }
func (f fakeConnector) SecretFields() []string               { return nil }
func (f fakeConnector) Coalescable() bool                    { return true }
func (f fakeConnector) ValidateConfig(json.RawMessage) error { return nil }
func (f fakeConnector) Deliver(context.Context, Instance, Announcement) error {
	return nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeConnector{kind: "webhook"})
	r.Register(fakeConnector{kind: "chat", singleton: true})

	c, ok := r.Get("chat")
	if !ok {
		t.Fatal("expected chat to be registered")
	}
	if !c.Singleton() {
		t.Fatal("expected chat to report itself as a singleton")
	}

	if _, ok := r.Get("nope"); ok {
		t.Fatal("expected an unregistered kind to be absent")
	}
}

func TestRegistryKindsAreSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeConnector{kind: "webhook"})
	r.Register(fakeConnector{kind: "chat"})

	kinds := r.Kinds()
	if len(kinds) != 2 || kinds[0] != "chat" || kinds[1] != "webhook" {
		t.Fatalf("Kinds() = %v, want [chat webhook]", kinds)
	}
}

// Registering the same kind twice can only be a bootstrap bug, and silently
// keeping one of the two would route deliveries to whichever won.
func TestRegistryPanicsOnDuplicateKind(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a duplicate registration to panic")
		}
	}()

	r := NewRegistry()
	r.Register(fakeConnector{kind: "webhook"})
	r.Register(fakeConnector{kind: "webhook"})
}

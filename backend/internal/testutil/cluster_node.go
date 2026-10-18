package testutil

import (
	"encoding/json"
	"sync"

	"github.com/williamokano/go-torrent-trader/backend/internal/cluster"
)

// ClusterNode is an in-memory cluster.Node: it records what a component
// publishes, and lets a test deliver messages as if another replica had sent
// them.
type ClusterNode struct {
	mu        sync.Mutex
	handlers  map[string][]cluster.Handler
	resync    []func()
	published []ClusterMessage
}

// ClusterMessage is one recorded Publish.
type ClusterMessage struct {
	Topic   string
	Payload json.RawMessage
}

// NewClusterNode creates an empty ClusterNode.
func NewClusterNode() *ClusterNode {
	return &ClusterNode{handlers: map[string][]cluster.Handler{}}
}

// Publish records the message, marshalled as the real bus would send it.
func (n *ClusterNode) Publish(topic string, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.published = append(n.published, ClusterMessage{Topic: topic, Payload: payload})
}

// Subscribe registers a handler, as the real bus does.
func (n *ClusterNode) Subscribe(topic string, h cluster.Handler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers[topic] = append(n.handlers[topic], h)
}

// OnResync registers a resync callback, as the real bus does.
func (n *ClusterNode) OnResync(f func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.resync = append(n.resync, f)
}

// Receive delivers v on topic as if another replica had published it.
func (n *ClusterNode) Receive(topic string, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	n.mu.Lock()
	handlers := append([]cluster.Handler(nil), n.handlers[topic]...)
	n.mu.Unlock()
	for _, h := range handlers {
		h(payload)
	}
}

// Resync fires the resync callbacks, as a reconnected listener does.
func (n *ClusterNode) Resync() {
	n.mu.Lock()
	callbacks := append([]func(){}, n.resync...)
	n.mu.Unlock()
	for _, f := range callbacks {
		f()
	}
}

// Published returns the payloads published on topic, in order.
func (n *ClusterNode) Published(topic string) []json.RawMessage {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []json.RawMessage
	for _, m := range n.published {
		if m.Topic == topic {
			out = append(out, m.Payload)
		}
	}
	return out
}

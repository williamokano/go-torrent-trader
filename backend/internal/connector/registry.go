package connector

import (
	"fmt"
	"sort"
	"sync"
)

// Registry maps a kind string to its compile-time Connector implementation.
// It is populated once during bootstrap and read-only thereafter.
type Registry struct {
	mu    sync.RWMutex
	kinds map[string]Connector
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{kinds: make(map[string]Connector)}
}

// Register adds a connector. Registering the same kind twice panics: it can only
// happen from a bootstrap-time coding mistake, and silently keeping one of the
// two would route deliveries to whichever won the race.
func (r *Registry) Register(c Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.kinds[c.Kind()]; exists {
		panic(fmt.Sprintf("connector: duplicate registration for kind %q", c.Kind()))
	}
	r.kinds[c.Kind()] = c
}

// Get returns the connector registered for kind.
func (r *Registry) Get(kind string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.kinds[kind]
	return c, ok
}

// Kinds returns every registered kind, sorted, for the admin UI's kind picker.
func (r *Registry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]string, 0, len(r.kinds))
	for k := range r.kinds {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

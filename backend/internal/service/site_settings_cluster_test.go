package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/cluster"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
)

// settingsNode is a minimal in-memory cluster.Node. (testutil has a fuller one,
// but importing it here would be an import cycle.)
type settingsNode struct {
	mu        sync.Mutex
	handlers  map[string][]cluster.Handler
	resync    []func()
	published []json.RawMessage
}

func (n *settingsNode) Publish(_ string, v any) {
	payload, _ := json.Marshal(v)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.published = append(n.published, payload)
}

func (n *settingsNode) Subscribe(topic string, h cluster.Handler) {
	if n.handlers == nil {
		n.handlers = map[string][]cluster.Handler{}
	}
	n.handlers[topic] = append(n.handlers[topic], h)
}

func (n *settingsNode) OnResync(f func()) { n.resync = append(n.resync, f) }

func (n *settingsNode) receive(topic string, v any) {
	payload, _ := json.Marshal(v)
	for _, h := range n.handlers[topic] {
		h(payload)
	}
}

func TestSiteSettingsSetTellsOtherNodesWhichKeyChanged(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	node := &settingsNode{}
	svc.UseCluster(node)

	if err := svc.Set(context.Background(), SettingHnREnabled, "true", event.Actor{ID: 1}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if len(node.published) != 1 || string(node.published[0]) != `{"key":"`+SettingHnREnabled+`"}` {
		t.Fatalf("published %s, want the changed key", node.published)
	}
}

// Without this another node keeps serving the old value for the cache's whole
// TTL — five minutes of hit-and-run enforcement after it was switched off.
func TestSiteSettingsEvictsAKeyChangedOnAnotherNode(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	node := &settingsNode{}
	svc.UseCluster(node)
	ctx := context.Background()

	_ = repo.Set(ctx, SettingHnREnabled, "true")
	if !svc.HnREnabled(ctx) {
		t.Fatal("expected true")
	}
	// Another node switches it off: the row changes, this node's cache does not.
	_ = repo.Set(ctx, SettingHnREnabled, "false")
	if !svc.HnREnabled(ctx) {
		t.Fatal("the cached value should still be served before any broadcast")
	}

	node.receive(TopicSiteSettingChanged, SiteSettingChange{Key: SettingHnREnabled})

	if svc.HnREnabled(ctx) {
		t.Fatal("still serving the old value after another node changed it")
	}
	if len(node.published) != 0 {
		t.Fatal("applying another node's change published it again")
	}
}

func TestSiteSettingsResyncEmptiesTheCache(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	node := &settingsNode{}
	svc.UseCluster(node)
	ctx := context.Background()

	_ = repo.Set(ctx, SettingChatSystemDisplayName, "Old Bot")
	_ = repo.Set(ctx, SettingHnREnabled, "true")
	svc.SystemChatName(ctx)
	svc.HnREnabled(ctx)
	_ = repo.Set(ctx, SettingChatSystemDisplayName, "New Bot")
	_ = repo.Set(ctx, SettingHnREnabled, "false")

	for _, f := range node.resync {
		f()
	}

	if got := svc.SystemChatName(ctx); got != "New Bot" {
		t.Errorf("SystemChatName = %q after resync, want the stored value", got)
	}
	if svc.HnREnabled(ctx) {
		t.Error("HnREnabled still cached after resync")
	}
}

func TestSiteSettingsIgnoresAMalformedBroadcast(t *testing.T) {
	svc := NewSiteSettingsService(newMockSiteSettingsRepo(), event.NewInMemoryBus())
	node := &settingsNode{}
	svc.UseCluster(node)
	node.receive(TopicSiteSettingChanged, []int{1})
}

package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// The live feed hub across replicas (#295): a delivery runs on one node, and a
// disconnect is decided on one node, but the browsers are spread over all of
// them.

func newClusteredStreamHarness(t *testing.T) (*streamHarness, *testutil.ClusterNode) {
	t.Helper()
	h := newStreamHarness(t)
	node := testutil.NewClusterNode()
	h.hub.resyncSpread = 0
	h.hub.UseCluster(node)
	return h, node
}

// nextAnnouncement reads frames until an announcement, skipping heartbeats.
func nextAnnouncement(read func() string) string {
	for {
		frame := read()
		if strings.HasPrefix(frame, ": ping") || strings.HasPrefix(frame, "retry:") {
			continue
		}
		return frame
	}
}

func TestFeedBroadcastIsPublishedForOtherNodes(t *testing.T) {
	h, node := newClusteredStreamHarness(t)

	h.hub.Broadcast("default", "torrent.published:1", []byte(`{"name":"x"}`))

	got := node.Published(topicFeedBroadcast)
	if len(got) != 1 {
		t.Fatalf("published %d, want 1", len(got))
	}
	var msg feedBroadcast
	if err := json.Unmarshal(got[0], &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Feed != "default" || msg.ID != "torrent.published:1" || string(msg.Payload) != `{"name":"x"}` {
		t.Fatalf("published %+v", msg)
	}
}

func TestFeedAppliesAnotherNodesBroadcastToItsOwnFeedOnly(t *testing.T) {
	h, node := newClusteredStreamHarness(t)
	watcher, closeWatcher := h.connectFeed(t, "default", h.login(t, 1))
	defer closeWatcher()
	readSSEFrame(t, watcher)
	other, closeOther := h.connectFeed(t, "no-adult", h.login(t, 2))
	defer closeOther()
	readSSEFrame(t, other)
	h.waitForClients(t, 2)

	node.Receive(topicFeedBroadcast, feedBroadcast{Feed: "no-adult", ID: "torrent.published:9", Payload: json.RawMessage(`{"n":9}`)})

	frame := nextAnnouncement(func() string { return readSSEFrame(t, other) })
	if !strings.Contains(frame, "id: torrent.published:9") || !strings.Contains(frame, `data: {"n":9}`) {
		t.Fatalf("frame = %q", frame)
	}
	if n := len(node.Published(topicFeedBroadcast)); n != 0 {
		t.Fatal("applying another node's broadcast published it again")
	}
}

func TestFeedDisconnectReachesOtherNodes(t *testing.T) {
	h, node := newClusteredStreamHarness(t)
	victim, closeVictim := h.connect(t, h.login(t, 1))
	defer closeVictim()
	readSSEFrame(t, victim)
	bystander, closeBystander := h.connect(t, h.login(t, 2))
	defer closeBystander()
	readSSEFrame(t, bystander)
	h.waitForClients(t, 2)

	node.Receive(topicFeedDisconnect, feedUser{UserID: 1})
	waitFor(t, "the member's stream to close", func() bool { return h.hub.clientCount() == 1 })
	if n := len(node.Published(topicFeedDisconnect)); n != 0 {
		t.Fatal("applying another node's disconnect published it again")
	}

	h.hub.DisconnectUser(2)
	if got := node.Published(topicFeedDisconnect); len(got) != 1 || string(got[0]) != `{"user_id":2}` {
		t.Fatalf("published %s", got)
	}
}

// A renamed feed is disconnected everywhere, or browsers on other nodes keep
// watching under a slug that may now be another feed's.
func TestFeedDisconnectAllReachesOtherNodes(t *testing.T) {
	h, node := newClusteredStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))
	defer closeStream()
	readSSEFrame(t, reader)
	h.waitForClients(t, 1)

	node.Receive(topicFeedDisconnectAll, struct{}{})
	waitFor(t, "every stream to close", func() bool { return h.hub.clientCount() == 0 })
	if n := len(node.Published(topicFeedDisconnectAll)); n != 0 {
		t.Fatal("applying another node's disconnect published it again")
	}

	h.hub.DisconnectAll()
	if n := len(node.Published(topicFeedDisconnectAll)); n != 1 {
		t.Fatalf("published %d disconnect-alls, want 1", n)
	}
}

func TestFeedResyncClosesEveryStream(t *testing.T) {
	h, node := newClusteredStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))
	defer closeStream()
	readSSEFrame(t, reader)
	h.waitForClients(t, 1)

	node.Resync()

	waitFor(t, "every stream to close", func() bool { return h.hub.clientCount() == 0 })
}

func TestFeedIgnoresMalformedClusterMessages(t *testing.T) {
	h, node := newClusteredStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))
	defer closeStream()
	readSSEFrame(t, reader)
	h.waitForClients(t, 1)

	node.Receive(topicFeedDisconnect, "nope")
	node.Receive(topicFeedBroadcast, 3)

	if h.hub.clientCount() != 1 {
		t.Fatal("a malformed message closed a stream")
	}
}

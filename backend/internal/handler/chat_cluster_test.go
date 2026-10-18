package handler

import (
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// These cover the chat hub across replicas (#295): what one node does to its
// clients is published, and what another node publishes reaches the clients
// connected here — without being published again.

func newClusteredWSHarness(t *testing.T, users ...*model.User) (*wsHarness, *testutil.ClusterNode) {
	t.Helper()
	h := newWSHarness(t, nil, users...)
	node := testutil.NewClusterNode()
	h.hub.resyncSpread = 0
	h.hub.UseCluster(node)
	return h, node
}

func waitForClients(t *testing.T, h *wsHarness, userID int64, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for h.clientsOf(userID) != want {
		if time.Now().After(deadline) {
			t.Fatalf("user %d has %d clients, want %d", userID, h.clientsOf(userID), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// expectClosed fails unless the server closes conn.
func expectClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				t.Fatal("the connection stayed open")
			}
			return
		}
	}
}

func TestChatMessageIsPublishedForOtherNodes(t *testing.T) {
	h, node := newClusteredWSHarness(t)
	h.session(t, "tok", 7, model.Permissions{Level: 1})
	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	sendText(t, conn, "hello cluster")
	readFrameOfType(t, conn, "message")

	published := node.Published(topicChatBroadcast)
	if len(published) != 1 {
		t.Fatalf("published %d chat broadcasts, want 1", len(published))
	}
	var frame map[string]any
	if err := json.Unmarshal(published[0], &frame); err != nil || frame["message"] != "hello cluster" {
		t.Fatalf("published %s, want the message frame", published[0])
	}
}

func TestChatAppliesAnotherNodesBroadcastWithoutRepublishing(t *testing.T) {
	h, node := newClusteredWSHarness(t)
	h.session(t, "tok", 7, model.Permissions{Level: 1})
	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	node.Receive(topicChatBroadcast, json.RawMessage(`{"type":"delete","id":5}`))

	if frame := readFrameOfType(t, conn, "delete"); frame["id"] != float64(5) {
		t.Fatalf("frame = %v", frame)
	}
	if n := len(node.Published(topicChatBroadcast)); n != 0 {
		t.Fatalf("re-published %d broadcasts from another node", n)
	}
}

func TestChatLocalBroadcastsArePublished(t *testing.T) {
	h, node := newClusteredWSHarness(t)

	h.hub.BroadcastDelete(1)
	h.hub.BroadcastDeleteUser(2)
	h.hub.Broadcast([]byte(`{"type":"message","id":3}`))

	if n := len(node.Published(topicChatBroadcast)); n != 3 {
		t.Fatalf("published %d broadcasts, want 3", n)
	}
}

// A member with two tabs may be connected to two replicas.
func TestChatSendToUserReachesOtherNodes(t *testing.T) {
	h, node := newClusteredWSHarness(t,
		&model.User{ID: 7, Username: "target", CanChat: true},
		&model.User{ID: 8, Username: "bystander", CanChat: true})
	h.session(t, "target", 7, model.Permissions{Level: 1})
	h.session(t, "bystander", 8, model.Permissions{Level: 1})
	target := h.mustDial(t, "target")
	bystander := h.mustDial(t, "bystander")
	readFrameOfType(t, target, "backfill")
	readFrameOfType(t, bystander, "backfill")

	h.hub.SendToUser(7, []byte(`{"type":"notification","id":1}`))
	readFrameOfType(t, target, "notification")
	if got := node.Published(topicChatToUser); len(got) != 1 ||
		string(got[0]) != `{"user_id":7,"data":{"type":"notification","id":1}}` {
		t.Fatalf("published %s", got)
	}

	node.Receive(topicChatToUser, chatToUser{UserID: 7, Data: json.RawMessage(`{"type":"notification","id":2}`)})
	if frame := readFrameOfType(t, target, "notification"); frame["id"] != float64(2) {
		t.Fatalf("frame = %v", frame)
	}
	_ = bystander.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, raw, err := bystander.ReadMessage(); err == nil {
		t.Fatalf("another member received %s", raw)
	}
	if n := len(node.Published(topicChatToUser)); n != 1 {
		t.Fatal("applying another node's message published it again")
	}
}

// A ban happens on one node; the member's connections on the others have to
// close too (#291, #295).
func TestChatDisconnectReachesOtherNodes(t *testing.T) {
	h, node := newClusteredWSHarness(t,
		&model.User{ID: 7, Username: "banned", CanChat: true},
		&model.User{ID: 8, Username: "bystander", CanChat: true})
	h.session(t, "banned", 7, model.Permissions{})
	h.session(t, "bystander", 8, model.Permissions{})
	victim := h.mustDial(t, "banned")
	h.mustDial(t, "bystander")
	waitForClients(t, h, 7, 1)
	waitForClients(t, h, 8, 1)

	node.Receive(topicChatDisconnect, chatUser{UserID: 7})

	expectClosed(t, victim)
	waitForClients(t, h, 7, 0)
	if h.clientsOf(8) != 1 {
		t.Error("another member's connection was closed")
	}
	if n := len(node.Published(topicChatDisconnect)); n != 0 {
		t.Fatal("applying another node's disconnect published it again")
	}

	h.hub.DisconnectUser(8)
	if got := node.Published(topicChatDisconnect); len(got) != 1 || string(got[0]) != `{"user_id":8}` {
		t.Fatalf("published %s, want the local disconnect", got)
	}
}

func TestChatReloadsSpamSettingsSavedOnAnotherNode(t *testing.T) {
	h, node := newClusteredWSHarness(t)
	h.settings.values[service.SettingChatRateLimitMax] = "42"

	node.Receive(service.TopicSiteSettingChanged, service.SiteSettingChange{Key: service.SettingChatRateLimitMax})

	deadline := time.Now().Add(3 * time.Second)
	for h.hub.getSpamSettings().rateLimitMaxMsgs != 42 {
		if time.Now().After(deadline) {
			t.Fatal("the anti-spam limit saved on another node was never loaded")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// After a gap in the broadcast, every client reconnects: that is what fetches
// the messages missed in it, and re-checks every session.
func TestChatResyncClosesEveryConnection(t *testing.T) {
	h, node := newClusteredWSHarness(t,
		&model.User{ID: 7, Username: "a", CanChat: true},
		&model.User{ID: 8, Username: "b", CanChat: true})
	h.session(t, "a", 7, model.Permissions{})
	h.session(t, "b", 8, model.Permissions{})
	first := h.mustDial(t, "a")
	second := h.mustDial(t, "b")
	waitForClients(t, h, 7, 1)
	waitForClients(t, h, 8, 1)

	node.Resync()

	expectClosed(t, first)
	expectClosed(t, second)
}

func TestChatIgnoresMalformedClusterMessages(t *testing.T) {
	h, node := newClusteredWSHarness(t)
	h.session(t, "tok", 7, model.Permissions{})
	h.mustDial(t, "tok")
	waitForClients(t, h, 7, 1)

	node.Receive(topicChatDisconnect, "not an object")
	node.Receive(topicChatToUser, 12)

	if h.clientsOf(7) != 1 {
		t.Fatal("a malformed message closed a connection")
	}
}

// A change missed while the broadcast was down would otherwise never load: the
// limits are only reloaded when a change is announced.
func TestChatResyncReloadsSpamSettings(t *testing.T) {
	h, node := newClusteredWSHarness(t)
	h.settings.values[service.SettingChatSpamStrikeCount] = "9"

	node.Resync()

	deadline := time.Now().Add(3 * time.Second)
	for h.hub.getSpamSettings().spamStrikeCount != 9 {
		if time.Now().After(deadline) {
			t.Fatal("resync did not reload the anti-spam settings")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

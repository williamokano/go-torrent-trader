package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func (h *ChatHub) localClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// A member whose client stops reading is dropped by the hub once its send
// queue fills — but the connection's reader keeps running until the writer
// gives up. Anything the reader then queues for that client (here, the
// rate-limit warning) must be discarded, not panic: a send on a closed
// channel would take the whole server process down.
func TestChatHub_DroppedSlowClientCanStillBeAnsweredSafely(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "slow", 7, model.Permissions{})
	conn := h.mustDial(t, "slow") // never read from: the client is "slow"

	deadline := time.Now().Add(5 * time.Second)
	for h.hub.localClientCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("client never registered")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Large frames fill the socket buffers so the writer blocks, then the
	// queue fills behind it and the hub drops the client.
	payload := make([]byte, 64<<10)
	for i := range payload {
		payload[i] = 'x'
	}
	for i := 0; h.hub.localClientCount() > 0; i++ {
		if i > 20000 {
			t.Fatal("the hub never dropped the slow client")
		}
		h.hub.broadcast <- ChatBroadcast{Data: payload}
	}

	// If the reader is still alive, flood past the rate limit so it answers.
	// The server may already have closed the connection, which is fine.
	raw, _ := json.Marshal(map[string]string{"type": "message", "text": "flood"})
	for i := 0; i < 20; i++ {
		if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
			break
		}
	}

	// Give the reader time to process the flood; a panic would abort the
	// test binary here.
	time.Sleep(500 * time.Millisecond)
}

// Every producer can still offer a message to a client the hub has stopped;
// it is dropped rather than sent on a closed channel.
func TestChatClient_TrySendAfterStop(t *testing.T) {
	c := &ChatClient{send: make(chan []byte, 1), done: make(chan struct{})}

	if !c.trySend([]byte("a")) {
		t.Fatal("trySend on a live client with room failed")
	}
	if c.trySend([]byte("b")) {
		t.Fatal("trySend on a full queue reported success")
	}

	c.stop()
	c.stop() // idempotent
	<-c.send // make room, so only done can refuse
	if c.trySend([]byte("c")) {
		t.Fatal("trySend after stop reported success")
	}
}

package handler

import (
	"bytes"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// A member whose client stops reading is dropped by the hub once its send
// queue fills — but the connection's reader can keep running until the
// writer gives up. Anything then queued for that client (here, the
// rate-limit warning) must be discarded, not panic: a send on a closed
// channel would take the whole server process down.
func TestChatHub_DroppedSlowClientCanStillBeAnsweredSafely(t *testing.T) {
	if testing.Short() {
		t.Skip("fills a socket's buffers; slow under -short")
	}
	h := newWSHarness(t, nil)
	h.session(t, "slow", 7, model.Permissions{})
	conn := h.mustDial(t, "slow") // never read from: the client is "slow"
	waitForClients(t, h, 7, 1)

	// Large frames fill the socket buffers so the writer blocks, then the
	// queue fills behind it and the hub drops the client.
	payload := bytes.Repeat([]byte("x"), 64<<10)
	for i := 0; h.clientsOf(7) > 0; i++ {
		if i > 20000 {
			t.Fatal("the hub never dropped the slow client")
		}
		h.hub.broadcast <- ChatBroadcast{Data: payload}
	}

	// Flood past the rate limit so a still-running reader answers. The server
	// may already have closed the connection, which is fine.
	for i := 0; i < 20; i++ {
		if writeText(conn, "flood") != nil {
			break
		}
	}

	// The server closing the connection means the reader has finished with
	// the flood; a panic would have aborted the test binary before this.
	expectClosed(t, conn)
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

// A panic while handling one member's message ends that member's connection
// and nothing else: the read loop runs outside the router's Recoverer.
func TestChatHub_PanicInMessageHandlingClosesOnlyThatConnection(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "boom", 7, model.Permissions{})
	conn := h.mustDial(t, "boom")
	waitForClients(t, h, 7, 1)

	h.messages.createPanic = true
	if err := writeText(conn, "hello"); err != nil {
		t.Fatalf("write: %v", err)
	}
	expectClosed(t, conn)
	waitForClients(t, h, 7, 0)

	// The process — and the hub — are still serving.
	h.messages.createPanic = false
	conn2 := h.mustDial(t, "boom")
	waitForClients(t, h, 7, 1)
	if err := writeText(conn2, "still here"); err != nil {
		t.Fatalf("write after recovery: %v", err)
	}
}

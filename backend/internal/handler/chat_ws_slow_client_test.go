package handler

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// A member whose client stops reading is dropped by the hub once its send
// queue fills — but the connection's reader can keep running until the
// writer gives up. Anything then queued for that client (here, the
// rate-limit warning) must be discarded, not panic: a send on a closed
// channel would take the whole server process down.
func TestChatHub_DroppedSlowClientCanStillBeAnsweredSafely(t *testing.T) {
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
	// the flood; a panic would have aborted the test binary before this. The
	// writer may sit out a full write deadline on the stalled socket first,
	// hence a longer wait than expectClosed's.
	_ = conn.SetReadDeadline(time.Now().Add(3 * writeWait))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				t.Fatal("the dropped client's connection stayed open")
			}
			return
		}
	}
}

// The hub drops a client whose queue is full by stopping it, never by
// closing its queue, so the client's own goroutines can still offer it
// messages afterwards.
func TestChatHub_DropStopsClientWithoutClosingQueue(t *testing.T) {
	h := newWSHarness(t, nil)
	c := &ChatClient{hub: h.hub, userID: 9, send: make(chan []byte, 1), done: make(chan struct{})}
	h.hub.register <- c
	waitForClients(t, h, 9, 1)

	c.send <- []byte("fills the queue")
	h.hub.broadcast <- ChatBroadcast{Data: []byte("overflows it")}
	waitForClients(t, h, 9, 0)

	select {
	case <-c.done:
	default:
		t.Fatal("a dropped client was not stopped")
	}
	<-c.send // the queued frame is still there: the queue was not closed
	if c.trySend([]byte("late reply")) {
		t.Fatal("a stopped client accepted a message")
	}
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

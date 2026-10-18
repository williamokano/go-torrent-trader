package cluster

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const waitFor = 5 * time.Second

// startNode runs a bus whose listener identifies itself as name in
// pg_stat_activity, so a test can find and kill that one connection.
func startNode(t *testing.T, name string) *Bus {
	t.Helper()
	db := requireDB(t)

	dsn, err := url.Parse(testDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	q := dsn.Query()
	q.Set("application_name", name)
	dsn.RawQuery = q.Encode()

	bus := New(db, dsn.String())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		bus.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	select {
	case <-bus.Connected():
	case <-time.After(waitFor):
		t.Fatalf("%s: listener never connected", name)
	}
	return bus
}

// collector records what a subscriber received.
type collector struct {
	mu   sync.Mutex
	got  []json.RawMessage
	seen chan struct{}
}

func newCollector() *collector { return &collector{seen: make(chan struct{}, 1024)} }

func (c *collector) handle(p json.RawMessage) {
	c.mu.Lock()
	c.got = append(c.got, append(json.RawMessage(nil), p...))
	c.mu.Unlock()
	c.seen <- struct{}{}
}

func (c *collector) waitN(t *testing.T, n int) []json.RawMessage {
	t.Helper()
	deadline := time.After(waitFor)
	for {
		c.mu.Lock()
		if len(c.got) >= n {
			out := append([]json.RawMessage(nil), c.got...)
			c.mu.Unlock()
			return out
		}
		c.mu.Unlock()
		select {
		case <-c.seen:
		case <-deadline:
			c.mu.Lock()
			defer c.mu.Unlock()
			t.Fatalf("got %d messages, want %d", len(c.got), n)
			return nil
		}
	}
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.got)
}

func TestBus_DeliversToOtherNodesOnly(t *testing.T) {
	a := startNode(t, "node-a")
	b := startNode(t, "node-b")

	onA, onB := newCollector(), newCollector()
	a.Subscribe("greet", onA.handle)
	b.Subscribe("greet", onB.handle)

	a.Publish("greet", map[string]string{"from": "a"})

	got := onB.waitN(t, 1)
	if string(got[0]) != `{"from":"a"}` {
		t.Errorf("payload = %s", got[0])
	}

	// A publishes after its own message would have arrived; if A heard
	// itself, it would by now.
	b.Publish("greet", map[string]string{"from": "b"})
	onA.waitN(t, 1)
	if n := onB.count(); n != 1 {
		t.Errorf("node b heard its own message: %d messages", n)
	}
	if got := onA.waitN(t, 1); string(got[0]) != `{"from":"b"}` {
		t.Errorf("node a got %s, want only b's message", got[0])
	}
}

func TestBus_IgnoresOtherTopics(t *testing.T) {
	a := startNode(t, "node-a")
	b := startNode(t, "node-b")

	wanted := newCollector()
	b.Subscribe("wanted", wanted.handle)

	a.Publish("unrelated", 1)
	a.Publish("wanted", 2)

	got := wanted.waitN(t, 1)
	if string(got[0]) != "2" {
		t.Errorf("payload = %s, want 2", got[0])
	}
}

func TestBus_KeepsPublishOrder(t *testing.T) {
	a := startNode(t, "node-a")
	b := startNode(t, "node-b")

	got := newCollector()
	b.Subscribe("seq", got.handle)

	const n = 50
	for i := range n {
		a.Publish("seq", i)
	}
	msgs := got.waitN(t, n)
	for i, m := range msgs {
		var v int
		if err := json.Unmarshal(m, &v); err != nil || v != i {
			t.Fatalf("message %d = %s, want %d", i, m, i)
		}
	}
}

func TestBus_SpillsOversizedPayloads(t *testing.T) {
	db := requireDB(t)
	a := startNode(t, "node-a")
	b := startNode(t, "node-b")

	// A leftover from long ago, which the next spill should clear.
	if _, err := db.Exec(`INSERT INTO cluster_messages (body, created_at) VALUES ('"stale"', now() - interval '1 hour')`); err != nil {
		t.Fatalf("seed stale row: %v", err)
	}

	got := newCollector()
	b.Subscribe("big", got.handle)

	big := strings.Repeat("x", 3*maxInlineBytes)
	a.Publish("big", big)

	msgs := got.waitN(t, 1)
	var decoded string
	if err := json.Unmarshal(msgs[0], &decoded); err != nil || decoded != big {
		t.Fatalf("oversized payload did not survive the round trip (%d bytes, err %v)", len(decoded), err)
	}

	var stale int
	if err := db.QueryRow(`SELECT count(*) FROM cluster_messages WHERE body = '"stale"'`).Scan(&stale); err != nil {
		t.Fatalf("count stale: %v", err)
	}
	if stale != 0 {
		t.Error("an old spilled message was not purged")
	}
}

func TestBus_SurvivesAPanickingHandler(t *testing.T) {
	a := startNode(t, "node-a")
	b := startNode(t, "node-b")

	b.Subscribe("boom", func(json.RawMessage) { panic("handler bug") })
	after := newCollector()
	b.Subscribe("after", after.handle)

	a.Publish("boom", 1)
	a.Publish("after", 2)

	after.waitN(t, 1)
}

func TestBus_ResyncsAfterLosingTheListener(t *testing.T) {
	db := requireDB(t)
	a := startNode(t, "node-a")
	b := startNode(t, "node-b-resync")

	resynced := make(chan struct{}, 1)
	b.OnResync(func() { resynced <- struct{}{} })
	got := newCollector()
	b.Subscribe("after", got.handle)

	// What a failover, a restart of the database or an idle timeout does to
	// the listener's session.
	var killed int
	if err := db.QueryRow(
		`SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE application_name = 'node-b-resync'`,
	).Scan(&killed); err != nil || killed == 0 {
		t.Fatalf("terminate listener: killed %d, err %v", killed, err)
	}

	select {
	case <-resynced:
	case <-time.After(waitFor):
		t.Fatal("no resync after the listener reconnected")
	}

	a.Publish("after", "back")
	if msgs := got.waitN(t, 1); string(msgs[0]) != `"back"` {
		t.Errorf("payload after reconnect = %s", msgs[0])
	}
}

func TestBus_PublishNeverBlocks(t *testing.T) {
	// No Run: nothing drains the buffer, so it fills and the rest are dropped.
	bus := New(nil, "")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range sendBuffer + 10 {
			bus.Publish("t", 1)
		}
	}()
	select {
	case <-done:
	case <-time.After(waitFor):
		t.Fatal("Publish blocked on a full buffer")
	}
	if len(bus.out) != sendBuffer {
		t.Errorf("buffered %d, want %d", len(bus.out), sendBuffer)
	}
}

func TestBus_PublishRejectsUnmarshallable(t *testing.T) {
	bus := New(nil, "")
	bus.Publish("t", make(chan int))
	if len(bus.out) != 0 {
		t.Error("an unmarshallable value was queued")
	}
}

func TestDecode(t *testing.T) {
	type msg struct {
		ID int64 `json:"id"`
	}
	if v, ok := Decode[msg]("t", json.RawMessage(`{"id":7}`)); !ok || v.ID != 7 {
		t.Errorf("Decode = %+v, %v", v, ok)
	}
	if _, ok := Decode[msg]("t", json.RawMessage(`not json`)); ok {
		t.Error("Decode accepted malformed input")
	}
}

func TestDispatch_IgnoresGarbageAndOwnOrigin(t *testing.T) {
	bus := New(nil, "")
	got := newCollector()
	bus.Subscribe("t", got.handle)

	bus.dispatch(context.Background(), "not json")
	own, _ := json.Marshal(envelope{Origin: bus.node, Topic: "t", Payload: json.RawMessage("1")})
	bus.dispatch(context.Background(), string(own))
	other, _ := json.Marshal(envelope{Origin: "elsewhere", Topic: "t", Payload: json.RawMessage("2")})
	bus.dispatch(context.Background(), string(other))

	if msgs := got.waitN(t, 1); len(msgs) != 1 || string(msgs[0]) != "2" {
		t.Errorf("dispatched %v, want only the other node's message", msgs)
	}
}

// Whatever a node cached before its first LISTEN went unannounced to it, so the
// first connect resyncs too.
func TestBus_ResyncsOnFirstConnect(t *testing.T) {
	db := requireDB(t)
	bus := New(db, testDSN)
	resynced := make(chan struct{}, 1)
	bus.OnResync(func() { resynced <- struct{}{} })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		bus.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	select {
	case <-resynced:
	case <-time.After(waitFor):
		t.Fatal("no resync on the first connect")
	}
}

// shortProbes makes the listener probe every interval and give up after
// timeout, for the length of one test.
func shortProbes(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	oldEvery, oldTimeout := probeEvery, probeTimeout
	probeEvery, probeTimeout = interval, timeout
	t.Cleanup(func() { probeEvery, probeTimeout = oldEvery, oldTimeout })
}

// A quiet channel is not a dead one: the probes keep coming back, so the
// listener stays up and does not resync — which would disconnect every member.
func TestBus_ProbesKeepAQuietListenerUp(t *testing.T) {
	shortProbes(t, 50*time.Millisecond, time.Second)
	a := startNode(t, "node-a")
	b := startNode(t, "node-b")

	resyncs := make(chan struct{}, 16)
	b.OnResync(func() { resyncs <- struct{}{} })
	got := newCollector()
	b.Subscribe("later", got.handle)

	time.Sleep(400 * time.Millisecond)
	if n := len(resyncs); n != 0 {
		t.Fatalf("a quiet listener resynced %d times", n)
	}
	a.Publish("later", 1)
	got.waitN(t, 1)
}

// A listener that hears nothing on its own connection — a pooler in
// transaction mode, a session cut off without a reset — must not count as
// live, or the node would serve stale state believing it was in step.
func TestBus_ListenerThatHearsNothingIsNeverLive(t *testing.T) {
	db := requireDB(t)
	shortProbes(t, 50*time.Millisecond, 100*time.Millisecond)
	probeChannel = "tt_cluster_nowhere"
	t.Cleanup(func() { probeChannel = Channel })

	bus := New(db, testDSN)
	resynced := make(chan struct{}, 1)
	bus.OnResync(func() { resynced <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		bus.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	select {
	case <-bus.Connected():
		t.Fatal("a listener whose probes never came back was reported connected")
	case <-resynced:
		t.Fatal("a deaf listener fired a resync")
	case <-time.After(500 * time.Millisecond):
	}
}

// Messages a sender could not get out are invisible to the receivers, whose
// listeners never dropped; the sender owes them a resync.
func TestBus_LostMessagesMakeOtherNodesResync(t *testing.T) {
	a := startNode(t, "node-a")
	b := startNode(t, "node-b")
	resynced := make(chan struct{}, 4)
	b.OnResync(func() { resynced <- struct{}{} })
	got := newCollector()
	b.Subscribe("after", got.handle)

	a.lost.Store(true) // as after a failed send or a full buffer
	a.Publish("after", 1)

	select {
	case <-resynced:
	case <-time.After(waitFor):
		t.Fatal("the receiver was never asked to resync")
	}
	got.waitN(t, 1)
	if a.lost.Load() {
		t.Error("the sender still believes it owes a resync")
	}
}

func TestBus_AFailedSendIsRemembered(t *testing.T) {
	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = db.Close() // every send now fails
	bus := New(db, "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		bus.send(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	bus.Publish("t", 1)
	deadline := time.After(waitFor)
	for !bus.lost.Load() {
		select {
		case <-deadline:
			t.Fatal("a send that failed every attempt was not remembered")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestBus_FullBufferIsRemembered(t *testing.T) {
	bus := New(nil, "")
	for range sendBuffer + 1 {
		bus.Publish("t", 1)
	}
	if !bus.lost.Load() {
		t.Fatal("a dropped broadcast was not remembered")
	}
}

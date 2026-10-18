// Package cluster tells the other replicas about changes that live in one
// process: a chat line for the browsers connected elsewhere, a stream to close
// after a ban, a cached setting to forget.
//
// It is PostgreSQL LISTEN/NOTIFY, so it needs nothing the deployment does not
// already run. Delivery is best-effort and at-most-once, which is the right
// shape for what it carries: every message describes something whose
// authoritative state is already in the database, and a node that may have
// missed messages is told so (OnResync) and can fall back to re-reading it.
//
// A publisher never hears its own messages. Callers apply a change locally
// first and then publish it for everyone else, so a single-process deployment
// behaves exactly as it did before this package existed — even with the
// listening connection down.
package cluster

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// Channel is the one NOTIFY channel every replica listens on. Topics are
// multiplexed inside the payload rather than spread over channels, so adding a
// topic never means another LISTEN.
const Channel = "tt_cluster"

const (
	// maxInlineBytes keeps a notification under PostgreSQL's 8000-byte payload
	// limit with room for the envelope. Anything larger is spilled to
	// cluster_messages and the notification carries its id.
	maxInlineBytes = 7900
	// sendBuffer is how many messages may wait for the sender. Publish never
	// blocks its caller — a worker slot or an HTTP request — so a database that
	// cannot keep up costs dropped broadcasts, not stalled requests.
	sendBuffer = 1024
	// sendTimeout bounds one NOTIFY (and its spill insert).
	sendTimeout = 5 * time.Second
	// spillRetention is how long a spilled message is kept. Listeners read it
	// within milliseconds of the notification; the margin is for a listener
	// that is slow, not for one that is down — that one resyncs instead.
	spillRetention = 10 * time.Minute
	// fetchTimeout bounds reading a spilled message back.
	fetchTimeout = 5 * time.Second
	// reconnectMin/Max bound the listener's backoff after losing its connection.
	reconnectMin = time.Second
	reconnectMax = 30 * time.Second
	// sendAttempts and sendRetry bound how hard the sender tries one message
	// before giving it up (and owing everyone a resync, see send).
	sendAttempts = 3
	sendRetry    = 500 * time.Millisecond
)

// Topics the bus uses itself.
const (
	// topicProbe is the listener's heartbeat: a notification it sends on its
	// own connection and must hear back. Never dispatched to subscribers.
	topicProbe = "cluster.probe"
	// topicResync asks every other node to resync. A node sends it once its
	// sender recovers from losing messages, because the receivers' listeners
	// never dropped and so have no way to know they missed anything.
	topicResync = "cluster.resync"
)

// Listener health, as variables so a test can shorten them. Read only by the
// listener goroutine, and written only before Run.
var (
	// probeEvery is how long the listener waits in silence before probing.
	// It also keeps the session from looking idle to a proxy or NAT that
	// would otherwise drop it.
	probeEvery = 30 * time.Second //nolint:gochecknoglobals // see above
	// probeTimeout is how long a probe may take to come back before the
	// connection is treated as dead.
	probeTimeout = 10 * time.Second //nolint:gochecknoglobals // see above
	// probeChannel is where probes are sent. Always Channel; a test points it
	// elsewhere to make a listener that hears nothing.
	probeChannel = Channel //nolint:gochecknoglobals // see above
)

// Handler receives one message published by another node.
//
// Handlers run on the listener's goroutine, one at a time and in arrival order,
// so they must not block: a hub handler should do what its local method does
// (queue onto a buffered channel, close a connection) and return.
type Handler func(payload json.RawMessage)

// Publisher is the side of the bus a node-local component needs. Every
// consumer takes this interface, and treats a nil one as "single node".
type Publisher interface {
	Publish(topic string, v any)
}

// Node is the whole bus as a node-local component sees it: publish its own
// changes, apply everyone else's, and rebuild after a gap. *Bus implements it.
type Node interface {
	Publisher
	Subscribe(topic string, h Handler)
	OnResync(f func())
}

// envelope is what travels over NOTIFY.
type envelope struct {
	Origin  string          `json:"o"`
	Topic   string          `json:"t"`
	Payload json.RawMessage `json:"p,omitempty"`
	// Ref points at a cluster_messages row holding the payload, for a message
	// too large to send inline.
	Ref int64 `json:"r,omitempty"`
}

type outgoing struct {
	topic   string
	payload json.RawMessage
}

// Bus is one node's connection to the cluster broadcast.
type Bus struct {
	db  *sql.DB
	dsn string
	// node identifies this process, so it can recognise and skip its own
	// notifications. Random per start: a restarted node is a new node.
	node string

	out chan outgoing

	mu       sync.RWMutex
	handlers map[string][]Handler
	resync   []func()

	// connected is closed the first time the listener is live. Tests wait on it
	// rather than sleeping.
	connectedOnce sync.Once
	connected     chan struct{}

	// lost is set when a message could not be sent or was dropped; see send.
	// Touched only by Publish (set) and the sender goroutine.
	lost atomic.Bool
}

// New creates a bus. dsn is the database URL the listener dials; it gets its
// own connection rather than one borrowed from a pool, because LISTEN belongs
// to the session that issued it and must hold that session for as long as the
// process runs.
func New(db *sql.DB, dsn string) *Bus {
	return &Bus{
		db:        db,
		dsn:       dsn,
		node:      newNodeID(),
		out:       make(chan outgoing, sendBuffer),
		handlers:  make(map[string][]Handler),
		connected: make(chan struct{}),
	}
}

func newNodeID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on any platform Go supports; a fixed id
		// would only make this node ignore another node's messages, so fall
		// back to the clock rather than refuse to start.
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Subscribe registers a handler for messages other nodes publish on topic.
func (b *Bus) Subscribe(topic string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], h)
}

// OnResync registers a callback for when this node may have missed messages:
// each time the listener goes live, the first time included. Before the first
// LISTEN, this node was already serving and caching but hearing nothing; after
// a lost connection, whatever was published in between is gone. Every
// node-local view built from broadcasts should treat it as "rebuild from the
// database".
func (b *Bus) OnResync(f func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resync = append(b.resync, f)
}

// Publish queues v, marshalled as JSON, for every other node. It never blocks.
func (b *Bus) Publish(topic string, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		slog.Error("cluster: cannot marshal broadcast", "topic", topic, "error", err)
		return
	}
	select {
	case b.out <- outgoing{topic: topic, payload: payload}:
	default:
		// Logged once per outage rather than per message: while the database
		// is unreachable every chat line would land here.
		if !b.lost.Swap(true) {
			slog.Warn("cluster: broadcasts are being dropped, send buffer full", "topic", topic)
		}
	}
}

// Run sends and receives until ctx is cancelled.
func (b *Bus) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		b.send(ctx)
	}()
	go func() {
		defer wg.Done()
		b.listen(ctx)
	}()
	wg.Wait()
}

// Connected is closed once the listener has been live at least once.
func (b *Bus) Connected() <-chan struct{} { return b.connected }

// send delivers queued messages one at a time, which keeps them in the order
// they were published.
//
// A message that cannot be sent after a few tries is given up — but the other
// nodes cannot tell they missed it, since their listeners are fine. So the
// sender remembers, and the next time a message does go out it is preceded by
// a resync request, which makes every other node rebuild what it may have got
// wrong: a ban that closed no connections, a setting that was never evicted.
func (b *Bus) send(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-b.out:
			if b.lost.Load() {
				if b.notifyWithRetry(ctx, outgoing{topic: topicResync, payload: json.RawMessage("{}")}) {
					b.lost.Store(false)
				}
			}
			if !b.notifyWithRetry(ctx, msg) {
				b.lost.Store(true)
			}
		}
	}
}

// notifyWithRetry sends msg, retrying briefly, and reports whether it went.
func (b *Bus) notifyWithRetry(ctx context.Context, msg outgoing) bool {
	var err error
	for attempt := range sendAttempts {
		if attempt > 0 && !sleep(ctx, sendRetry*time.Duration(attempt)) {
			return false
		}
		if err = b.notify(ctx, msg); err == nil {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
	}
	slog.Warn("cluster: broadcast not sent, other nodes will be asked to resync",
		"topic", msg.topic, "error", err)
	return false
}

func (b *Bus) notify(ctx context.Context, msg outgoing) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	env := envelope{Origin: b.node, Topic: msg.topic, Payload: msg.payload}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if len(body) > maxInlineBytes {
		ref, err := b.spill(ctx, msg.payload)
		if err != nil {
			return fmt.Errorf("spill oversized broadcast: %w", err)
		}
		env.Payload, env.Ref = nil, ref
		if body, err = json.Marshal(env); err != nil {
			return err
		}
	}
	_, err = b.db.ExecContext(ctx, `SELECT pg_notify($1, $2)`, Channel, string(body))
	return err
}

// spill stores a payload too large for NOTIFY and returns its id. Old rows are
// cleared on the same path: spills are rare, so this is the only maintenance
// the table needs.
func (b *Bus) spill(ctx context.Context, payload json.RawMessage) (int64, error) {
	if _, err := b.db.ExecContext(ctx,
		`DELETE FROM cluster_messages WHERE created_at < now() - make_interval(secs => $1)`,
		spillRetention.Seconds(),
	); err != nil {
		// A failed purge only leaves rows for the next one.
		slog.Warn("cluster: could not purge old spilled broadcasts", "error", err)
	}
	var id int64
	err := b.db.QueryRowContext(ctx,
		`INSERT INTO cluster_messages (body) VALUES ($1) RETURNING id`, string(payload),
	).Scan(&id)
	return id, err
}

// listen holds the LISTEN connection, reconnecting with backoff when it drops.
func (b *Bus) listen(ctx context.Context) {
	backoff := reconnectMin
	for ctx.Err() == nil {
		err := b.listenOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errWasLive) {
			// It worked until now, so this is a fresh outage, not the next
			// failure of a long one.
			backoff = reconnectMin
		}
		slog.Warn("cluster: listener disconnected, reconnecting", "error", err, "in", backoff)
		if !sleep(ctx, backoff) {
			return
		}
		backoff = min(backoff*2, reconnectMax)
	}
}

// errWasLive marks a listener that was connected before it failed.
var errWasLive = errors.New("listener was live")

// errProbeLost means a heartbeat never came back on a connection that
// otherwise looked fine.
var errProbeLost = errors.New("heartbeat did not come back")

// listenOnce connects, listens until the connection fails, and returns why.
//
// A connection is not trusted on LISTEN succeeding. A session cut off without
// a reset — a failover, a NAT dropping state — blocks a reader for as long as
// TCP keepalive takes to notice, and a pooler in transaction mode accepts
// LISTEN and then never delivers anything. Either would leave this node deaf
// and unaware of it. So the listener probes: it notifies itself on its own
// connection and must hear that back, once on connecting and again after every
// probeEvery of silence. It only counts as live — firing the resync callbacks
// — once the first probe has returned.
func (b *Bus) listenOnce(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, b.dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	live := false
	probe, err := b.probe(ctx, conn)
	if err != nil {
		return err
	}
	probeSent := time.Now()

	for {
		wait := probeEvery
		if probe != "" {
			wait = max(time.Until(probeSent.Add(probeTimeout)), 0)
		}
		waitCtx, cancel := context.WithTimeout(ctx, wait)
		n, err := conn.WaitForNotification(waitCtx)
		cancel()

		switch {
		case err == nil:
		case ctx.Err() != nil:
			return ctx.Err()
		case !errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return b.dropped(live, err)
		case probe != "":
			if !live {
				slog.Error("cluster: listener hears nothing on its own connection; " +
					"is the database behind a pooler in transaction mode? It must be session mode")
			}
			return b.dropped(live, errProbeLost)
		default:
			// probeEvery of silence: check the connection is still there.
			if probe, err = b.probe(ctx, conn); err != nil {
				return b.dropped(live, err)
			}
			probeSent = time.Now()
			continue
		}

		if probe != "" && b.isProbe(n.Payload, probe) {
			probe = ""
			if !live {
				live = true
				b.fireResync()
				b.connectedOnce.Do(func() { close(b.connected) })
			}
			continue
		}
		b.dispatch(ctx, n.Payload)
	}
}

// dropped wraps why a connection ended, marking whether it had been live.
func (b *Bus) dropped(live bool, err error) error {
	if live {
		return fmt.Errorf("%w: %w", errWasLive, err)
	}
	return err
}

// probe sends a heartbeat on the listener's own connection and returns its
// nonce.
func (b *Bus) probe(ctx context.Context, conn *pgx.Conn) (string, error) {
	nonce := newNodeID()
	body, err := json.Marshal(envelope{Origin: b.node, Topic: topicProbe, Payload: json.RawMessage(strconv.Quote(nonce))})
	if err != nil {
		return "", err
	}
	probeCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	if _, err := conn.Exec(probeCtx, "SELECT pg_notify($1, $2)", probeChannel, string(body)); err != nil {
		return "", fmt.Errorf("probe: %w", err)
	}
	return nonce, nil
}

// isProbe reports whether a notification is this listener's pending probe.
func (b *Bus) isProbe(raw, nonce string) bool {
	var env envelope
	if json.Unmarshal([]byte(raw), &env) != nil || env.Origin != b.node || env.Topic != topicProbe {
		return false
	}
	var got string
	return json.Unmarshal(env.Payload, &got) == nil && got == nonce
}

func (b *Bus) dispatch(ctx context.Context, raw string) {
	var env envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		slog.Warn("cluster: ignoring an unreadable broadcast", "error", err)
		return
	}
	if env.Origin == b.node || env.Topic == topicProbe {
		return
	}
	if env.Topic == topicResync {
		b.fireResync()
		return
	}

	b.mu.RLock()
	handlers := b.handlers[env.Topic]
	b.mu.RUnlock()
	if len(handlers) == 0 {
		return
	}

	payload := env.Payload
	if env.Ref != 0 {
		fetched, err := b.fetch(ctx, env.Ref)
		if err != nil {
			slog.Warn("cluster: could not read a spilled broadcast", "topic", env.Topic, "error", err)
			return
		}
		payload = fetched
	}

	for _, h := range handlers {
		b.call(env.Topic, h, payload)
	}
}

func (b *Bus) fetch(ctx context.Context, id int64) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	var body string
	if err := b.db.QueryRowContext(ctx, `SELECT body FROM cluster_messages WHERE id = $1`, id).Scan(&body); err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// call runs one handler, so a panic in one consumer cannot take the listener —
// and with it every other node-local view — down.
func (b *Bus) call(topic string, h Handler, payload json.RawMessage) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("cluster: broadcast handler panicked", "topic", topic, "panic", r)
		}
	}()
	h(payload)
}

func (b *Bus) fireResync() {
	b.mu.RLock()
	callbacks := append([]func(){}, b.resync...)
	b.mu.RUnlock()
	slog.Info("cluster: resyncing node-local state")
	for _, f := range callbacks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("cluster: resync callback panicked", "panic", r)
				}
			}()
			f()
		}()
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Decode unmarshals a payload into T, logging and reporting false on failure,
// so a handler can be written as a two-line guard.
func Decode[T any](topic string, payload json.RawMessage) (T, bool) {
	var v T
	if err := json.Unmarshal(payload, &v); err != nil {
		slog.Warn("cluster: ignoring a malformed broadcast", "topic", topic, "error", err)
		return v, false
	}
	return v, true
}

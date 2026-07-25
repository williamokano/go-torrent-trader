package irc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/ergochat/irc-go/ircevent"
	"github.com/ergochat/irc-go/ircmsg"
)

// ircConn is the slice of an IRC connection this package uses.
//
// It exists so the lifecycle — connect, register, join, announce, quit — can be
// tested without a real server. The one implementation that talks to the network
// is ergoConn below; everything else in this package is written against this
// interface.
type ircConn interface {
	Connect() error
	// Loop runs the read/reconnect loop until Quit is called.
	Loop()
	Join(channel string) error
	Privmsg(target, message string) error
	Connected() bool
	Quit()
	// OnConnect registers a callback fired once *registration* completes —
	// after the MOTD, not merely when the socket opens — and again after every
	// reconnect. Channels are joined from here so a reconnect re-joins.
	OnConnect(func())
	// OnDisconnect registers a callback fired when the connection drops, so the
	// instance stops being considered ready to deliver.
	OnDisconnect(func())
}

// dialer builds a connection from a validated config. Swapped in tests.
type dialer func(cfg Config) ircConn

// ergoConn adapts ircevent.Connection to ircConn.
type ergoConn struct {
	conn *ircevent.Connection
}

func (c *ergoConn) Connect() error                   { return c.conn.Connect() }
func (c *ergoConn) Loop()                            { c.conn.Loop() }
func (c *ergoConn) Privmsg(target, msg string) error { return c.conn.Privmsg(target, msg) }
func (c *ergoConn) Join(channel string) error        { return c.conn.Join(channel) }
func (c *ergoConn) Connected() bool                  { return c.conn.Connected() }
func (c *ergoConn) Quit()                            { c.conn.Quit() }

// OnConnect uses AddConnectCallback, which the library fires on RPL_ENDOFMOTD /
// ERR_NOMOTD — i.e. once registration is genuinely complete. That distinction
// matters: Connection.Connected() reports the socket, and is already true during
// the CAP/SASL handshake, when a PRIVMSG would earn ERR_NOTREGISTERED and be
// thrown away by the server.
func (c *ergoConn) OnConnect(fn func()) {
	c.conn.AddConnectCallback(func(ircmsg.Message) { fn() })
}

func (c *ergoConn) OnDisconnect(fn func()) {
	c.conn.AddDisconnectCallback(func(ircmsg.Message) { fn() })
}

// newErgoConn configures a real IRC connection.
//
// Reconnection is the library's job, not ours: ircevent's Loop already retries
// with backoff and re-runs the connect callbacks, which is the main reason this
// library was chosen over one where we would hand-roll it.
func newErgoConn(cfg Config) ircConn {
	conn := &ircevent.Connection{
		Server:      fmt.Sprintf("%s:%d", cfg.Server, cfg.Port),
		Nick:        cfg.Nick,
		User:        cfg.Nick,
		RealName:    cfg.Nick,
		UseTLS:      cfg.TLS,
		QuitMessage: "shutting down",
		Timeout:     30 * time.Second,
		KeepAlive:   2 * time.Minute,
		// Reconnect promptly, but not so fast that a server rejecting us turns
		// into a connection flood.
		ReconnectFreq: 10 * time.Second,
		// Route the library's own diagnostics through the app's logger instead
		// of its default bare os.Stdout writer.
		Log: newSlogBridge(),
	}
	if cfg.TLS {
		conn.TLSConfig = &tls.Config{ServerName: cfg.Server, MinVersion: tls.VersionTLS12}
	}
	if cfg.SASLUser != "" && cfg.SASLPass != "" {
		conn.UseSASL = true
		conn.SASLLogin = cfg.SASLUser
		conn.SASLPassword = cfg.SASLPass
	}
	return &ergoConn{conn: conn}
}

// run owns one IRC client for the lifetime of ctx.
//
// publish hands the connection to Deliver; setReady tracks whether it is
// registered and therefore safe to write to. They are separate because the two
// change at different moments: the connection exists for the whole run, but is
// only ready between registration and disconnect.
func run(ctx context.Context, cfg Config, dial dialer, publish func(ircConn), setReady func(bool)) error {
	conn := dial(cfg)

	conn.OnConnect(func() {
		// Registration has completed. Identify first — on a +r channel an
		// unidentified bot's messages are silently dropped by the server while
		// Privmsg still returns nil, which would mark every delivery 'sent'
		// while nothing arrived.
		if cfg.NickServ != "" {
			if err := conn.Privmsg("NickServ", "IDENTIFY "+sanitizeLine(cfg.NickServ)); err != nil {
				slog.Error("irc: failed to identify to NickServ", "server", cfg.Server, "error", err)
			}
		}
		for _, channel := range cfg.Channels {
			if err := conn.Join(channel.Name); err != nil {
				slog.Error("irc: failed to join channel", "channel", channel.Name, "error", err)
			}
		}
		setReady(true)
	})

	conn.OnDisconnect(func() { setReady(false) })

	// Published before connecting, so there is no window in which a callback
	// fires against an unpublished client. Deliver reports ErrNotReady until
	// the connect callback flips the flag.
	publish(conn)

	if err := conn.Connect(); err != nil {
		return fmt.Errorf("connect to irc: %w", err)
	}

	done := make(chan struct{})
	go func() {
		conn.Loop()
		close(done)
	}()

	select {
	case <-ctx.Done():
		setReady(false)
		conn.Quit()
		// Quit asks the loop to wind down; wait so the socket is actually closed
		// before the manager considers this instance stopped. If it overruns,
		// say so — the goroutine and its socket outlive us, and the nick will
		// still be taken when the replacement connects.
		select {
		case <-done:
		case <-time.After(quitGrace):
			slog.Warn("irc: client did not stop within the grace period; its socket may linger",
				"server", cfg.Server)
		}
		return ctx.Err()
	case <-done:
		// Loop only returns once Quit has been called, so reaching here means
		// something else stopped it.
		setReady(false)
		return fmt.Errorf("irc connection to %s ended unexpectedly", cfg.Server)
	}
}

// quitGrace is how long to wait for the client to finish disconnecting. It is
// deliberately shorter than the manager's own stop grace, so the manager is not
// the one that times out first.
const quitGrace = 3 * time.Second

// newSlogBridge builds the *log.Logger the library expects, writing through to
// slog so its diagnostics land in the app's log pipeline rather than on a bare
// os.Stdout.
func newSlogBridge() *log.Logger {
	return log.New(slogWriter{}, "", 0)
}

type slogWriter struct{}

func (slogWriter) Write(p []byte) (int, error) {
	// Sanitised: these lines can quote server output, which is remote input.
	slog.Debug("irc client", "message", sanitizeLine(strings.TrimRight(string(p), "\n")))
	return len(p), nil
}

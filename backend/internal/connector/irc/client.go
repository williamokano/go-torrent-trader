package irc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"github.com/ergochat/irc-go/ircevent"
	"github.com/ergochat/irc-go/ircmsg"
)

// ircConn is the slice of an IRC connection this package uses.
//
// It exists so the lifecycle — connect, join, announce, quit — can be tested
// without a real server. The one implementation that talks to the network is
// ergoConn below; everything else in this package is written against this
// interface.
type ircConn interface {
	Connect() error
	// Loop runs the read/reconnect loop until Quit is called.
	Loop()
	Join(channel string) error
	Privmsg(target, message string) error
	Connected() bool
	Quit()
	// OnConnect registers a callback fired after each successful registration —
	// including after a reconnect, which is why channels are joined from here
	// rather than once at startup.
	OnConnect(func())
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
func (c *ergoConn) OnConnect(fn func())              { c.conn.AddConnectCallback(func(ircmsg.Message) { fn() }) }

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

// run owns one IRC client for the lifetime of ctx: connect, join, then block
// until cancelled. It is the PersistentConnector.Start body.
func run(ctx context.Context, cfg Config, dial dialer, onReady func(ircConn)) error {
	conn := dial(cfg)

	// Joins happen in the connect callback so a reconnect re-joins too.
	conn.OnConnect(func() {
		for _, channel := range cfg.Channels {
			if err := conn.Join(channel.Name); err != nil {
				slog.Error("irc: failed to join channel", "channel", channel.Name, "error", err)
			}
		}
	})

	if err := conn.Connect(); err != nil {
		return fmt.Errorf("connect to irc: %w", err)
	}

	// Publish the live client only once the connection is up, so Deliver
	// reports ErrNotReady rather than writing into a half-built connection.
	onReady(conn)

	done := make(chan struct{})
	go func() {
		conn.Loop()
		close(done)
	}()

	select {
	case <-ctx.Done():
		conn.Quit()
		// Quit asks the loop to wind down; wait so the socket is actually closed
		// before the manager considers this instance stopped.
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			slog.Warn("irc: client did not stop within the grace period", "server", cfg.Server)
		}
		return ctx.Err()
	case <-done:
		// The library gave up reconnecting.
		return fmt.Errorf("irc connection to %s ended", cfg.Server)
	}
}

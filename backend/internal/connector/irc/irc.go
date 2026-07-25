// Package irc announces new torrents to IRC channels.
//
// Unlike every other connector this one holds a long-lived connection, which
// makes it the only kind with a lifecycle: a ConnectorManager starts it, and
// across a multi-node deployment a Postgres advisory lock ensures exactly one
// node runs it (see connector/leader) — two would join the channel twice and
// announce everything twice.
package irc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
)

// DefaultTemplate is the line sent when an instance sets no template.
const DefaultTemplate = "[{{.Category}}] {{.Name}} [{{.SizeHuman}}]{{if .Freeleech}} [FL]{{end}} — {{.URL}}"

// CoalescedTemplate stands in for a batch the rate limit could not send
// individually.
const CoalescedTemplate = "{{.Coalesced}} new torrents published"

const (
	// maxLineBytes keeps a rendered line inside what an IRC server will accept
	// once the ":nick!user@host PRIVMSG #channel :" prefix is added. The RFC
	// limit is 512 including CR/LF; 400 leaves comfortable room for a long
	// prefix without needing to know the server's idea of our hostmask.
	maxLineBytes = 400
	// channelSendDelay paces multi-channel announcements. The pipeline's
	// rate_per_min already bounds the overall rate; this is politeness towards
	// servers that throttle bursts from one client.
	channelSendDelay = time.Second
)

// Channel is one target channel, optionally narrowed to some categories.
type Channel struct {
	Name string `json:"name"`
	// Categories, when non-empty, limits this channel to those category IDs —
	// so one connection can feed #movies and #tv from the same instance.
	Categories []int64 `json:"categories"`
}

// Config is the IRC instance's admin-editable settings.
type Config struct {
	Server   string    `json:"server"`
	Port     int       `json:"port"`
	TLS      bool      `json:"tls"`
	Nick     string    `json:"nick"`
	SASLUser string    `json:"sasl_user"`
	SASLPass string    `json:"sasl_pass"`
	NickServ string    `json:"nickserv_pass"`
	Channels []Channel `json:"channels"`
	Template string    `json:"template"`
}

// Connector implements connector.PersistentConnector for IRC.
//
// It holds the running clients keyed by instance ID: Deliver has to find the
// specific connection that belongs to the instance a delivery is for, which is
// why the Connector interface passes the whole Instance rather than just its
// config.
type Connector struct {
	mu      sync.RWMutex
	clients map[int64]ircConn
	dial    dialer
	// sendDelay is the pause between channels, lowered in tests.
	sendDelay time.Duration
}

// New creates the IRC connector.
func New() *Connector {
	return &Connector{
		clients:   make(map[int64]ircConn),
		dial:      newErgoConn,
		sendDelay: channelSendDelay,
	}
}

func (c *Connector) Kind() string    { return "irc" }
func (c *Connector) Singleton() bool { return false }

// SecretFields covers both authentication paths; either may be unused.
func (c *Connector) SecretFields() []string { return []string{"sasl_pass", "nickserv_pass"} }

// Coalescable: an IRC channel is read by people, and twenty lines from a bulk
// import is what the "+N more" summary exists to prevent.
func (c *Connector) Coalescable() bool { return true }

// ValidateConfig checks everything that can be checked without connecting.
func (c *Connector) ValidateConfig(cfg json.RawMessage) error {
	parsed, err := parseConfig(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(parsed.Server) == "" {
		return fmt.Errorf("server is required")
	}
	if parsed.Port < 1 || parsed.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", parsed.Port)
	}
	if err := validateNick(parsed.Nick); err != nil {
		return err
	}
	if len(parsed.Channels) == 0 {
		return fmt.Errorf("at least one channel is required")
	}
	seen := make(map[string]bool, len(parsed.Channels))
	for _, channel := range parsed.Channels {
		if err := validateChannelName(channel.Name); err != nil {
			return err
		}
		if seen[channel.Name] {
			return fmt.Errorf("channel %q is listed twice", channel.Name)
		}
		seen[channel.Name] = true
	}
	if err := connector.ValidateTemplate(parsed.Template); err != nil {
		return err
	}
	return connector.ValidateRatePerMin(cfg)
}

// Start opens the connection and holds it until ctx is cancelled. The manager
// guarantees only one node calls this for a given instance.
func (c *Connector) Start(ctx context.Context, inst connector.Instance) error {
	cfg, err := parseConfig(inst.Config)
	if err != nil {
		return err
	}

	defer c.forget(inst.ID)
	return run(ctx, cfg, c.dial, func(conn ircConn) {
		c.mu.Lock()
		c.clients[inst.ID] = conn
		c.mu.Unlock()
	})
}

// Deliver writes one line per matching channel on the instance's live client.
//
// A client that is missing or still connecting yields ErrNotReady rather than an
// error: a reconnect is routine, and burning retry attempts on it would
// dead-letter a queue of perfectly good announcements.
func (c *Connector) Deliver(ctx context.Context, inst connector.Instance, a connector.Announcement) error {
	cfg, err := parseConfig(inst.Config)
	if err != nil {
		return err
	}

	c.mu.RLock()
	conn := c.clients[inst.ID]
	c.mu.RUnlock()

	if conn == nil || !conn.Connected() {
		return fmt.Errorf("%w: irc client for instance %d is not connected", connector.ErrNotReady, inst.ID)
	}

	tmpl := cfg.Template
	if a.Coalesced > 0 {
		tmpl = CoalescedTemplate
	} else if tmpl == "" {
		tmpl = DefaultTemplate
	}

	line, err := connector.RenderTemplate(tmpl, sanitizeAnnouncement(a))
	if err != nil {
		return err
	}
	line = truncateLine(sanitizeLine(line), maxLineBytes)

	targets := matchingChannels(cfg.Channels, a)
	if len(targets) == 0 {
		// Every channel filtered this announcement out. Nothing to do, and it is
		// not a failure — the instance-level filters simply were not the only
		// ones that applied.
		return nil
	}

	for i, target := range targets {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.sendDelay):
			}
		}
		if err := conn.Privmsg(target, line); err != nil {
			return fmt.Errorf("irc privmsg to %s: %w", target, err)
		}
	}
	return nil
}

// Status reports whether the instance currently has a live connection, for the
// admin panel's badge.
func (c *Connector) Connected(instanceID int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conn := c.clients[instanceID]
	return conn != nil && conn.Connected()
}

func (c *Connector) forget(instanceID int64) {
	c.mu.Lock()
	delete(c.clients, instanceID)
	c.mu.Unlock()
}

// matchingChannels picks the channels this announcement belongs in. A channel
// with no categories takes everything.
func matchingChannels(channels []Channel, a connector.Announcement) []string {
	var targets []string
	for _, channel := range channels {
		if len(channel.Categories) == 0 {
			targets = append(targets, channel.Name)
			continue
		}
		for _, id := range channel.Categories {
			if id == a.CategoryID {
				targets = append(targets, channel.Name)
				break
			}
		}
	}
	return targets
}

// sanitizeAnnouncement strips control characters from every field a template
// can interpolate.
//
// This is the injection guard. A torrent name is attacker-supplied — anyone who
// can upload chooses it — and IRC is a newline-delimited protocol, so a name
// containing CR/LF would end the PRIVMSG and let the rest be read as a fresh
// command from our authenticated bot. Sanitising the inputs rather than the
// output means a custom template cannot reintroduce the hole.
func sanitizeAnnouncement(a connector.Announcement) connector.Announcement {
	a.Name = sanitizeLine(a.Name)
	a.Category = sanitizeLine(a.Category)
	a.Uploader = sanitizeLine(a.Uploader)
	a.URL = sanitizeLine(a.URL)
	a.Title = sanitizeLine(a.Title)
	a.Body = sanitizeLine(a.Body)
	return a
}

// sanitizeLine removes CR, LF, NUL and other C0 control bytes.
func sanitizeLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		// Other C0 controls are IRC formatting codes; dropping them keeps a
		// rendered line predictable.
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// truncateLine keeps a line within the byte budget without splitting a rune.
func truncateLine(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	const ellipsis = "…"
	budget := limit - len(ellipsis)
	for budget > 0 && !isRuneBoundary(s, budget) {
		budget--
	}
	return s[:budget] + ellipsis
}

func isRuneBoundary(s string, i int) bool {
	return i >= len(s) || (s[i]&0xC0) != 0x80
}

func parseConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	if err := connector.DecodeConfig(raw, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Server = strings.TrimSpace(cfg.Server)
	cfg.Nick = strings.TrimSpace(cfg.Nick)
	return cfg, nil
}

func validateNick(nick string) error {
	if strings.TrimSpace(nick) == "" {
		return fmt.Errorf("nick is required")
	}
	if sanitizeLine(nick) != nick || strings.ContainsAny(nick, " ,*?!@") {
		return fmt.Errorf("nick %q contains invalid characters", nick)
	}
	return nil
}

func validateChannelName(name string) error {
	if name == "" {
		return fmt.Errorf("channel name is required")
	}
	if !strings.HasPrefix(name, "#") && !strings.HasPrefix(name, "&") {
		return fmt.Errorf("channel %q must start with # or &", name)
	}
	if sanitizeLine(name) != name || strings.ContainsAny(name, " ,\a") {
		return fmt.Errorf("channel %q contains invalid characters", name)
	}
	return nil
}

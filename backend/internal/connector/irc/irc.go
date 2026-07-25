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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	// maxChannels bounds one instance, because the pacing above has to fit
	// inside the pipeline's per-delivery timeout. Beyond this the delivery
	// could never finish, so it is refused at save time rather than
	// dead-lettering every announcement later. More channels than this is what
	// a second instance is for.
	maxChannels = 8
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
	clients map[int64]*client
	dial    dialer
	// sendDelay is the pause between channels, lowered in tests.
	sendDelay time.Duration
}

// client is one instance's live connection plus whether it is registered.
//
// The two are tracked separately because the library reports its socket as
// connected during the CAP/SASL handshake, when the server would answer a
// PRIVMSG with ERR_NOTREGISTERED and drop it — which would mark the delivery
// sent while nothing arrived.
type client struct {
	conn  ircConn
	ready atomic.Bool
}

// New creates the IRC connector.
func New() *Connector {
	return &Connector{
		clients:   make(map[int64]*client),
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
	if len(parsed.Channels) > maxChannels {
		return fmt.Errorf("at most %d channels per instance (add another instance for more)", maxChannels)
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

	entry := &client{}
	defer c.forget(inst.ID, entry)

	return run(ctx, cfg, c.dial,
		func(conn ircConn) {
			entry.conn = conn
			c.mu.Lock()
			c.clients[inst.ID] = entry
			c.mu.Unlock()
		},
		entry.ready.Store,
	)
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
	entry := c.clients[inst.ID]
	c.mu.RUnlock()

	// Gated on registration, not on the socket: writing during the handshake
	// would be silently discarded by the server while Privmsg reported success.
	if entry == nil || !entry.ready.Load() {
		return fmt.Errorf("%w: irc client for instance %d is not registered", connector.ErrNotReady, inst.ID)
	}
	conn := entry.conn

	tmpl := cfg.Template
	if a.Coalesced > 0 {
		tmpl = CoalescedTemplate
	} else if tmpl == "" {
		tmpl = DefaultTemplate
	}

	line, err := renderLine(tmpl, a)
	if err != nil {
		return err
	}

	targets := matchingChannels(cfg.Channels, a)
	if len(targets) == 0 {
		// Every channel filtered this announcement out. Nothing to do, and it is
		// not a failure — the instance-level filters simply were not the only
		// ones that applied.
		return nil
	}

	// A partial failure re-sends to the channels already reached when the row is
	// retried. Delivery is at-least-once throughout the pipeline, and tracking
	// per-channel progress would need per-target state on the delivery row for
	// a duplicate line in a chat channel.
	for i, target := range targets {
		if i > 0 && !pause(ctx, c.sendDelay) {
			return ctx.Err()
		}
		// The target comes from stored config, so it is sanitised here too
		// rather than trusting it was validated on the way in.
		if err := conn.Privmsg(sanitizeLine(target), line); err != nil {
			return fmt.Errorf("irc privmsg to %s: %w", target, err)
		}
	}
	return nil
}

// pause waits between channel sends, reporting false if the context ended.
func pause(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// renderLine renders and fits an announcement into one IRC line.
//
// When it does not fit, the *name* is shortened rather than the finished line:
// the default template puts the URL last, so truncating the line would drop the
// one part a reader needs.
func renderLine(tmpl string, a connector.Announcement) (string, error) {
	safe := sanitizeAnnouncement(a)

	line, err := connector.RenderTemplate(tmpl, safe)
	if err != nil {
		return "", err
	}
	line = sanitizeLine(line)
	if len(line) <= maxLineBytes {
		return line, nil
	}

	overflow := len(line) - maxLineBytes
	if len(safe.Name) > overflow {
		safe.Name = truncateText(safe.Name, len(safe.Name)-overflow)
		if line, err = connector.RenderTemplate(tmpl, safe); err != nil {
			return "", err
		}
		line = sanitizeLine(line)
	}
	// Belt and braces: a template that ignores .Name, or a name that was
	// already short, can still overrun.
	return truncateText(line, maxLineBytes), nil
}

// Connected reports whether the instance is registered and able to announce,
// which is what the admin panel's badge should reflect.
func (c *Connector) Connected(instanceID int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry := c.clients[instanceID]
	return entry != nil && entry.ready.Load()
}

// forget drops the instance's client, but only if it is still the one this run
// published. The manager stops the old client before starting a replacement, so
// this cannot normally collide — but a late-returning Start must never delete
// its successor's entry and leave every delivery reporting not-ready.
func (c *Connector) forget(instanceID int64, entry *client) {
	entry.ready.Store(false)
	c.mu.Lock()
	if c.clients[instanceID] == entry {
		delete(c.clients, instanceID)
	}
	c.mu.Unlock()
}

// matchingChannels picks the channels this announcement belongs in. A channel
// with no categories takes everything.
//
// A coalesced summary goes to every channel: it stands for a batch that may
// span several categories, and routing it by the one representative row's
// category would silently drop the announcement for all the others.
func matchingChannels(channels []Channel, a connector.Announcement) []string {
	if a.Coalesced > 0 {
		targets := make([]string, 0, len(channels))
		for _, channel := range channels {
			targets = append(targets, channel.Name)
		}
		return targets
	}

	// Matched against the whole ancestor chain, exactly like the instance-level
	// filter: routing #movies to "Movies" has to catch a torrent filed under
	// "Movies / Action", or the delivery is marked sent having gone nowhere.
	chain := a.CategoryChain()

	var targets []string
	for _, channel := range channels {
		if len(channel.Categories) == 0 {
			targets = append(targets, channel.Name)
			continue
		}
		if slices.ContainsFunc(chain, func(id int64) bool {
			return slices.Contains(channel.Categories, id)
		}) {
			targets = append(targets, channel.Name)
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
	// Only the fields RenderContext exposes to a template; sanitising the rest
	// would be work nothing can reach.
	a.Name = sanitizeLine(a.Name)
	a.Category = sanitizeLine(a.Category)
	a.Uploader = sanitizeLine(a.Uploader)
	a.URL = sanitizeLine(a.URL)
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

// truncateText keeps a string within a byte budget without splitting a rune.
func truncateText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	const ellipsis = "…"
	budget := limit - len(ellipsis)
	if budget <= 0 {
		// No room for the marker; cut cleanly instead.
		budget = limit
		for budget > 0 && !isRuneBoundary(s, budget) {
			budget--
		}
		return s[:budget]
	}
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

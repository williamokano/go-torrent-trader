// Package httpguard builds the *http.Client that outbound connectors use.
//
// A webhook URL is admin-supplied, so without a guard the admin panel is a
// server-side request forgery primitive: "http://169.254.169.254/latest/meta-data/"
// reads cloud credentials, "http://10.0.0.5:6379/" pokes an internal service.
// The check therefore happens in the dialer's Control callback, on the IP the
// connection is actually being made to — which is the only place that survives
// both DNS rebinding (the name resolves to a public IP at validation time and a
// private one at dial time) and redirect chains (each hop re-enters the dialer).
package httpguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// maxRedirects bounds a redirect chain. Each hop is still dialed through the
// guarded dialer, so this is about not following a loop, not about safety.
const maxRedirects = 3

// cgnat is the carrier-grade NAT range. net.IP has no predicate for it, but it
// is routinely used for internal networks (Tailscale, some ISPs).
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// ErrBlockedAddress is returned by the dialer when a destination resolves to an
// address connectors are not allowed to reach.
var ErrBlockedAddress = errors.New("blocked address")

// NewClient returns an HTTP client that refuses to connect to private or
// otherwise internal addresses.
//
// allowPrivate is a func rather than a bool because it reads the
// connectors_allow_private_networks site setting, which an admin can flip at
// runtime to point a webhook at a LAN receiver. A nil func means "never allow".
func NewClient(allowPrivate func() bool, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate != nil && allowPrivate() {
				return nil
			}
			return checkAddress(address)
		},
	}

	transport := &http.Transport{
		// Deliberately no proxy. With one configured, every request would dial
		// the proxy's address instead of the target's and the guard above would
		// only ever inspect the proxy — i.e. the SSRF protection would silently
		// become a no-op.
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
}

// checkAddress validates the "host:port" the dialer is about to connect to. By
// this point host is always a literal IP — resolution has already happened.
func checkAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: cannot parse %q", ErrBlockedAddress, address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: %q is not an IP address", ErrBlockedAddress, host)
	}
	return CheckIP(ip)
}

// CheckIP reports whether connectors may dial ip.
func CheckIP(ip net.IP) error {
	// Normalize IPv4-mapped IPv6 (::ffff:10.0.0.1) down to its v4 form, or the
	// v4 predicates below would all miss it.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	switch {
	case ip.IsUnspecified():
		return blocked(ip, "unspecified")
	case ip.IsLoopback():
		return blocked(ip, "loopback")
	case ip.IsPrivate():
		// RFC1918 for v4, fc00::/7 unique-local for v6.
		return blocked(ip, "private")
	case ip.IsLinkLocalUnicast():
		// 169.254.0.0/16 — this is where cloud metadata services live.
		return blocked(ip, "link-local")
	case ip.IsLinkLocalMulticast(), ip.IsInterfaceLocalMulticast(), ip.IsMulticast():
		return blocked(ip, "multicast")
	case ip.Equal(net.IPv4bcast):
		return blocked(ip, "broadcast")
	}

	if addr, ok := netip.AddrFromSlice(ip); ok && cgnat.Contains(addr.Unmap()) {
		return blocked(ip, "carrier-grade NAT")
	}

	return nil
}

func blocked(ip net.IP, why string) error {
	return fmt.Errorf("%w: %s is %s", ErrBlockedAddress, ip, why)
}

// ValidateURL is the save-time sanity check for an admin-entered destination.
// It deliberately does not resolve the host: whether the target is reachable and
// whether it is private are both decided at dial time, where the answer cannot
// go stale.
func ValidateURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("url must include a host")
	}
	if u.Hostname() == "" {
		return errors.New("url must include a host")
	}
	if u.User != nil {
		// Credentials in the URL would end up in *url.Error strings, which is
		// exactly the leak path the whole secret-redaction effort closes.
		return errors.New("url must not embed credentials")
	}
	return nil
}

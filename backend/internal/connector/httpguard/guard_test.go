package httpguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCheckIPRejectsInternalAddresses(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"127.9.9.9",       // the whole loopback /8
		"10.0.0.1",        // RFC1918
		"172.16.5.4",      // RFC1918
		"192.168.1.1",     // RFC1918
		"169.254.169.254", // cloud metadata — the reason this package exists
		"169.254.0.1",     // link-local generally
		"100.64.0.1",      // carrier-grade NAT
		"0.0.0.0",         // unspecified
		"224.0.0.1",       // multicast
		"255.255.255.255", // broadcast
		"::1",             // IPv6 loopback
		"fe80::1",         // IPv6 link-local
		"fc00::1",         // IPv6 unique-local
		"::",              // IPv6 unspecified
		"::ffff:10.0.0.1", // IPv4-mapped IPv6 must not sneak past the v4 checks
		// These embed an IPv4 address inside an IPv6 one, so To4() is nil and
		// every v4 predicate is skipped — the reason the explicit prefix list
		// exists alongside the predicates.
		"64:ff9b::a9fe:a9fe", // NAT64 wrapping 169.254.169.254
		"64:ff9b::7f00:1",    // NAT64 wrapping 127.0.0.1
		"2002:0a00:0001::",   // 6to4 wrapping 10.0.0.1
		"0.0.0.1",            // 0.0.0.0/8 — http://0/ reaches loopback on Linux
		"100.64.0.1",         // carrier-grade NAT
		"192.0.0.1",          // IETF protocol assignments
		"198.18.0.1",         // benchmarking
		"240.0.0.1",          // reserved
	}
	for _, raw := range blocked {
		t.Run(raw, func(t *testing.T) {
			ip := net.ParseIP(raw)
			if ip == nil {
				t.Fatalf("test bug: %q is not an IP", raw)
			}
			err := CheckIP(ip)
			if err == nil {
				t.Fatalf("CheckIP(%s) allowed an internal address", raw)
			}
			if !errors.Is(err, ErrBlockedAddress) {
				t.Fatalf("CheckIP(%s) = %v, want ErrBlockedAddress", raw, err)
			}
		})
	}
}

func TestCheckIPAllowsPublicAddresses(t *testing.T) {
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2001:4860:4860::8888"} {
		t.Run(raw, func(t *testing.T) {
			if err := CheckIP(net.ParseIP(raw)); err != nil {
				t.Fatalf("CheckIP(%s) = %v, want nil", raw, err)
			}
		})
	}
}

func TestClientRefusesLoopbackTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(func() bool { return false }, 5*time.Second)

	_, err := client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected the guard to refuse a loopback target")
	}
	if !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("error = %v, want it to name the blocked address", err)
	}
}

// httptest binds loopback, so the success path can only be exercised with the
// escape hatch on. That makes this the allow-private-networks test as well.
func TestClientAllowsLoopbackWhenPrivateNetworksArePermitted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(func() bool { return true }, 5*time.Second)

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected the request to succeed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// The check runs in the dialer's Control callback, on the address actually being
// connected to. That is what makes a hostname resolving to a private IP fail —
// and what defeats DNS rebinding, where the name is public at validation time
// and private at dial time.
func TestClientRejectsHostnameThatResolvesToLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}
	byName := "http://localhost:" + parsed.Port() + "/"

	client := NewClient(func() bool { return false }, 5*time.Second)

	if _, err := client.Get(byName); err == nil {
		t.Fatal("expected a hostname resolving to loopback to be refused")
	} else if !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("error = %v, want it to name the blocked address", err)
	}
}

// A receiver that 302s somewhere else would otherwise be handed the request
// body, the HMAC signature and every admin-configured header — Go strips only
// Authorization and Cookie across hosts. So a cross-host redirect is refused
// before it is even dialed.
func TestClientRefusesCrossHostRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()

	dials := 0
	client := NewClient(func() bool {
		dials++
		return true
	}, 5*time.Second)

	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("expected a cross-host redirect to be refused")
	} else if !strings.Contains(err.Error(), "cross-host redirect") {
		t.Fatalf("error = %v, want it to name the refused redirect", err)
	}
	if dials != 1 {
		t.Fatalf("dials = %d, want the redirect target never to have been contacted", dials)
	}
}

// Same-host redirects stay legal, so an http→https upgrade on the receiver's own
// host still works.
func TestClientFollowsSameHostRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, srv.URL+"/moved", http.StatusFound)
	}))
	defer srv.Close()

	client := NewClient(func() bool { return true }, 5*time.Second)

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("same-host redirect should be followed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestClientCapsRedirectChains(t *testing.T) {
	var srv *httptest.Server
	hops := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	defer srv.Close()

	client := NewClient(func() bool { return true }, 5*time.Second)

	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("expected an endless redirect chain to be stopped")
	}
	if hops > maxRedirects+1 {
		t.Fatalf("followed %d hops, want at most %d", hops, maxRedirects+1)
	}
}

// A nil allowPrivate means "never allow", so a caller that forgets to wire the
// setting fails closed.
func TestClientWithNilAllowPrivateFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	client := NewClient(nil, 5*time.Second)

	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("expected a nil allowPrivate to behave as deny")
	}
}

func TestValidateURL(t *testing.T) {
	valid := []string{
		"http://example.test/hook",
		"https://example.test/hook?x=1",
	}
	for _, raw := range valid {
		if err := ValidateURL(raw); err != nil {
			t.Errorf("ValidateURL(%q) = %v, want nil", raw, err)
		}
	}

	invalid := map[string]string{
		"empty":               "",
		"whitespace":          "   ",
		"file scheme":         "file:///etc/passwd",
		"gopher scheme":       "gopher://example.test/",
		"no scheme":           "example.test/hook",
		"scheme without host": "https://",
		// Credentials in the URL would be echoed by *url.Error, which is exactly
		// the leak path the redaction work closes.
		"embedded credentials": "https://user:pass@example.test/hook",
	}
	for name, raw := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateURL(raw); err == nil {
				t.Fatalf("ValidateURL(%q) = nil, want an error", raw)
			}
		})
	}
}

func TestCheckAddressRejectsNonIPHost(t *testing.T) {
	if err := checkAddress("example.test:80"); err == nil {
		t.Fatal("expected a non-IP dial address to be refused")
	}
	if err := checkAddress("garbage"); err == nil {
		t.Fatal("expected an unparseable dial address to be refused")
	}
}

func TestClientHasNoProxy(t *testing.T) {
	// A configured proxy would make every request dial the proxy instead of the
	// target, silently turning the whole guard into a no-op.
	client := NewClient(func() bool { return false }, time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("connector HTTP client must not use a proxy")
	}
}

var _ = context.Background

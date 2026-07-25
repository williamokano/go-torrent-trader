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

// Every redirect hop re-enters the same dialer, so an allowed host cannot bounce
// a request into the metadata service.
func TestClientGuardsRedirectTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()

	// Permit only the first dial (the test server on loopback); the redirect
	// target is then judged on its own merits, exactly as a real public host
	// redirecting inward would be.
	dials := 0
	client := NewClient(func() bool {
		dials++
		return dials == 1
	}, 5*time.Second)

	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("expected the redirect to the metadata address to be refused")
	} else if !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("error = %v, want it to name the blocked address", err)
	}
	if dials < 2 {
		t.Fatalf("dials = %d, want the redirect target to have been dialed and checked", dials)
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

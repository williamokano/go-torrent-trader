package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	realclientip "github.com/realclientip/realclientip-go"
)

func mustRanges(t *testing.T, cidrs ...string) []net.IPNet {
	t.Helper()
	if len(cidrs) == 0 {
		return nil
	}
	nets, err := realclientip.AddressesAndRangesToIPNets(cidrs...)
	if err != nil {
		t.Fatalf("bad test ranges %v: %v", cidrs, err)
	}
	return nets
}

func runRealIP(t *testing.T, trusted []net.IPNet, remote string, headers map[string]string, extraXFF []string) string {
	t.Helper()
	var got string
	h := RealIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, v := range extraXFF {
		req.Header.Add("X-Forwarded-For", v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestRealIP(t *testing.T) {
	tests := []struct {
		name     string
		trusted  []string
		remote   string
		headers  map[string]string
		extraXFF []string
		want     string
	}{
		{
			name:    "no trusted proxies leaves RemoteAddr untouched",
			remote:  "203.0.113.9:5000",
			headers: map[string]string{"X-Forwarded-For": "1.2.3.4"},
			want:    "203.0.113.9:5000",
		},
		{
			name:    "untrusted direct peer: X-Forwarded-For ignored",
			trusted: []string{"10.0.0.0/8"},
			remote:  "203.0.113.9:5000",
			headers: map[string]string{"X-Forwarded-For": "1.2.3.4"},
			want:    "203.0.113.9:5000",
		},
		{
			name:    "trusted direct peer: single X-Forwarded-For entry wins",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.7"},
			want:    "198.51.100.7",
		},
		{
			name:    "trusted direct peer: trailing trusted hops are skipped",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.7, 10.9.9.9, 10.8.8.8"},
			want:    "198.51.100.7",
		},
		{
			name:    "trusted direct peer: leftmost client-controlled entry not reached when a real hop precedes it",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1, 203.0.113.5, 10.9.9.9"},
			want:    "203.0.113.5",
		},
		{
			name:    "trusted direct peer: X-Real-IP is NOT consulted",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			headers: map[string]string{"X-Real-IP": "1.2.3.4"},
			want:    "10.1.2.3:5000",
		},
		{
			name:    "trusted direct peer: True-Client-IP is NOT consulted",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			headers: map[string]string{"True-Client-IP": "1.2.3.4"},
			want:    "10.1.2.3:5000",
		},
		{
			name:    "trusted direct peer but no X-Forwarded-For: RemoteAddr kept",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			want:    "10.1.2.3:5000",
		},
		{
			name:    "every X-Forwarded-For entry is a trusted hop: RemoteAddr kept",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "10.9.9.9, 10.8.8.8"},
			want:    "10.1.2.3:5000",
		},
		{
			name:     "multi-line X-Forwarded-For: the proxy-appended line is still walked",
			trusted:  []string{"10.0.0.0/8"},
			remote:   "10.1.2.3:5000",
			headers:  map[string]string{"X-Forwarded-For": "1.2.3.4"},
			extraXFF: []string{"203.0.113.5"},
			want:     "203.0.113.5",
		},
		{
			name:    "IPv6 trusted proxy and IPv6 client",
			trusted: []string{"2001:db8::/32"},
			remote:  "[2001:db8::1]:5000",
			headers: map[string]string{"X-Forwarded-For": "2606:4700:4700::1111"},
			want:    "2606:4700:4700::1111",
		},
		{
			name:    "IPv4-mapped IPv6 in the header normalises to IPv4",
			trusted: []string{"10.0.0.0/8"},
			remote:  "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "::ffff:203.0.113.5"},
			want:    "203.0.113.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runRealIP(t, mustRanges(t, tt.trusted...), tt.remote, tt.headers, tt.extraXFF)
			if got != tt.want {
				t.Errorf("RemoteAddr = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRealIP_PortlessDirectPeerStillGated(t *testing.T) {
	// chi's RealIP (or a prior hop) can leave RemoteAddr without a port.
	got := runRealIP(t,
		mustRanges(t, "10.0.0.0/8"),
		"10.1.2.3",
		map[string]string{"X-Forwarded-For": "203.0.113.5"},
		nil,
	)
	if got != "203.0.113.5" {
		t.Errorf("RemoteAddr = %q, want %q", got, "203.0.113.5")
	}
}

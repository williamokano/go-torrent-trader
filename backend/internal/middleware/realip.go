package middleware

import (
	"net"
	"net/http"

	realclientip "github.com/realclientip/realclientip-go"
)

// RealIP rewrites r.RemoteAddr from X-Forwarded-For, but only when the request's
// direct peer is one of the trusted proxy networks. chi's own RealIP middleware
// trusts that header (and X-Real-IP / True-Client-IP) from anyone, which turns
// `curl -H 'X-Forwarded-For: 1.2.3.4'` into a way to choose the address the
// server records and bans against.
//
// Behaviour:
//   - trusted is empty (a direct-to-internet deployment): RemoteAddr is left as
//     the socket address, headers ignored.
//   - the direct peer is not in trusted: same — its headers are attacker-supplied.
//   - the direct peer is in trusted: X-Forwarded-For is walked right-to-left,
//     skipping entries that are themselves trusted ranges, and the first
//     remaining address wins — the outermost address our own infrastructure
//     observed. The leftmost entries, which a client fully controls, are never
//     reached unless every hop in between is a trusted range.
//
// Only X-Forwarded-For is consulted: the documented reverse proxies (Caddy,
// nginx) set it, and unlike the single-value headers it carries the whole chain
// so trusted hops can be filtered out. The walk is realclientip-go's
// RightmostTrustedRangeStrategy, which also handles multi-line headers, ports
// and IPv6 zones.
//
// The rewritten RemoteAddr carries no port, matching what chi's RealIP produced;
// clientIP / announceClientIP already fall back to the bare value when
// net.SplitHostPort fails.
func RealIP(trusted []net.IPNet) func(http.Handler) http.Handler {
	if len(trusted) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	strategy := realclientip.Must(
		realclientip.NewRightmostTrustedRangeStrategy("X-Forwarded-For", trusted),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peerInRanges(r.RemoteAddr, trusted) {
				if ip := strategy.ClientIP(r.Header, r.RemoteAddr); ip != "" {
					r.RemoteAddr = ip
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func peerInRanges(remoteAddr string, ranges []net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for i := range ranges {
		if ranges[i].Contains(ip) {
			return true
		}
	}
	return false
}

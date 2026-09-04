package service_test

import (
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// The session list is only useful if a member can tell which row is their phone
// and which is not theirs at all. DeviceLabel is what puts something in that
// column, and it is fed two pieces of attacker-controlled text — so it is also
// the place where that text stops being dangerous.

func TestDeviceLabelPrefersTheNameTheClientGave(t *testing.T) {
	got := service.DeviceLabel("Work laptop", "Mozilla/5.0 (Windows NT 10.0) Firefox/128.0")
	if got != "Work laptop" {
		t.Errorf("DeviceLabel = %q, want the client's own name", got)
	}
}

func TestDeviceLabelDerivesFromTheUserAgent(t *testing.T) {
	tests := map[string]struct {
		ua   string
		want string
	}{
		"firefox on windows": {
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
			"Firefox on Windows",
		},
		"chrome on macos": {
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
			"Chrome on macOS",
		},
		// Safari is the one every Chromium browser claims to be, so it may only
		// win when nothing else matched.
		"safari on ios": {
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
			"Safari on iOS",
		},
		// And Edge and Opera both claim to be Chrome.
		"edge on windows": {
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0",
			"Edge on Windows",
		},
		"opera on linux": {
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 OPR/112.0.0.0",
			"Opera on Linux",
		},
		// Android contains "Linux", so order decides this one.
		"chrome on android": {
			"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36",
			"Chrome on Android",
		},
		"a system with no recognisable browser": {
			"curl/8.7.1 (Windows)",
			"Windows",
		},
		"nothing recognisable at all": {
			"curl/8.7.1",
			"Unknown device",
		},
		"no user agent": {
			"",
			"Unknown device",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := service.DeviceLabel("", tc.ua); got != tc.want {
				t.Errorf("DeviceLabel(%q) = %q, want %q", tc.ua, got, tc.want)
			}
		})
	}
}

// The label is stored, rendered back into a page, and logged. Everything a
// client supplies about itself is text it chose, so none of it may carry a line
// break into a log or an unbounded blob into the session list.
func TestDeviceLabelSanitizesWhatTheClientSupplies(t *testing.T) {
	got := service.DeviceLabel("Work\r\nlaptop\x00\t", "")
	if strings.ContainsAny(got, "\r\n\x00\t") {
		t.Errorf("DeviceLabel = %q, which still carries a control character", got)
	}
	if got != "Work laptop" {
		t.Errorf("DeviceLabel = %q, want %q", got, "Work laptop")
	}

	long := service.DeviceLabel(strings.Repeat("a", 500), "")
	if n := len([]rune(long)); n > 64 {
		t.Errorf("DeviceLabel kept %d runes; the column is bounded at 64", n)
	}

	// A name that is only whitespace and control characters is no name at all,
	// so it must fall through to the User-Agent rather than blanking the row.
	if got := service.DeviceLabel(" \n ", "Mozilla/5.0 (Windows NT 10.0) Firefox/128.0"); got != "Firefox on Windows" {
		t.Errorf("DeviceLabel = %q, want the derived label", got)
	}
}

// Never echo the raw User-Agent: it is the client's own text, and rendering it
// back into the member's session list is how a header ends up on a page.
func TestDeviceLabelNeverEchoesTheRawUserAgent(t *testing.T) {
	ua := `Mozilla/5.0 <script>alert(1)</script> (Windows NT 10.0) Firefox/128.0`
	got := service.DeviceLabel("", ua)
	if strings.Contains(got, "script") {
		t.Errorf("DeviceLabel = %q, which carries User-Agent text through verbatim", got)
	}
}

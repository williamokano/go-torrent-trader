package service_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// The session list is only useful if a member can tell which row is their phone
// and which is not theirs at all. These two functions are what put something in
// that column, and their input is attacker-controlled text — so they are also
// where that text stops being dangerous.

func TestDeviceLabelFromUserAgentDerivesFromTheUserAgent(t *testing.T) {
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
			if got := service.DeviceLabelFromUserAgent(tc.ua); got != tc.want {
				t.Errorf("DeviceLabelFromUserAgent(%q) = %q, want %q", tc.ua, got, tc.want)
			}
		})
	}
}

// The label is stored, rendered back into a page, and logged, so it is bounded
// and stripped on the way in — whatever produced it.
func TestDeviceLabelSanitizesItsInput(t *testing.T) {
	got := service.DeviceLabel("Work\r\nlaptop\x00\t")
	if strings.ContainsAny(got, "\r\n\x00\t") {
		t.Errorf("DeviceLabel = %q, which still carries a control character", got)
	}
	if got != "Work laptop" {
		t.Errorf("DeviceLabel = %q, want %q", got, "Work laptop")
	}

	long := service.DeviceLabel(strings.Repeat("a", 500))
	if n := len([]rune(long)); n > 64 {
		t.Errorf("DeviceLabel kept %d runes; the column is bounded at 64", n)
	}

	// Whitespace and control characters are no label at all, and a blank row
	// tells the member nothing.
	if got := service.DeviceLabel(" \n "); got != "Unknown device" {
		t.Errorf("DeviceLabel = %q, want the placeholder", got)
	}
}

// Never echo the raw User-Agent: it is the client's own text, and rendering it
// back into the member's session list is how a header ends up on a page.
func TestDeviceLabelNeverEchoesTheRawUserAgent(t *testing.T) {
	ua := `Mozilla/5.0 <script>alert(1)</script> (Windows NT 10.0) Firefox/128.0`
	got := service.DeviceLabelFromUserAgent(ua)
	if strings.Contains(got, "script") {
		t.Errorf("DeviceLabelFromUserAgent = %q, which carries User-Agent text through verbatim", got)
	}
}

// The label is derived, never accepted. A client-chosen name would be text an
// intruder writes into the very list a member reads to work out which row is
// not theirs — "Chrome on Windows" beside the genuine ones, or "This device".
func TestALoginCannotChooseItsOwnDeviceLabel(t *testing.T) {
	body := `{"username":"u","password":"p","device_name":"This device"}`

	var req service.LoginRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if req.DeviceName != "" {
		t.Errorf("DeviceName = %q — the request body set the label an intruder's "+
			"session shows up as", req.DeviceName)
	}

	var reg service.RegisterRequest
	if err := json.Unmarshal([]byte(`{"username":"u","device_name":"This device"}`), &reg); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if reg.DeviceName != "" {
		t.Errorf("DeviceName = %q on register", reg.DeviceName)
	}
}

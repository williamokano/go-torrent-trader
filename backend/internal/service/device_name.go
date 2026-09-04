package service

import (
	"strings"
	"unicode"
)

// maxDeviceNameLen caps a device label in runes. The label is member-supplied
// text that is stored and then rendered back into the session list, so it is
// bounded on the way in rather than trusted to be reasonable.
const maxDeviceNameLen = 64

// DeviceLabel decides what a session is called in "where am I logged in".
//
// A client may name itself — a phone and a laptop are otherwise two identical
// rows of an IP and a timestamp, and telling them apart is the whole point of
// the list. When it does not, the User-Agent is the only thing left, so it is
// reduced to a browser and an operating system: enough for a member to
// recognise a device, and short enough to sit in a table cell.
//
// Never returns raw User-Agent text. It is attacker-controlled, it is echoed
// back into a page and into logs, and a browser name drawn from a fixed list
// cannot carry anything through.
func DeviceLabel(supplied, userAgent string) string {
	if name := sanitizeDeviceName(supplied); name != "" {
		return name
	}
	if derived := deviceFromUserAgent(userAgent); derived != "" {
		return derived
	}
	return "Unknown device"
}

// sanitizeDeviceName strips control characters, collapses runs of whitespace,
// and truncates to maxDeviceNameLen runes.
func sanitizeDeviceName(name string) string {
	var b strings.Builder
	lastWasSpace := true // leading whitespace is dropped
	for _, r := range name {
		if unicode.IsSpace(r) {
			if !lastWasSpace {
				b.WriteRune(' ')
				lastWasSpace = true
			}
			continue
		}
		if !unicode.IsPrint(r) {
			continue
		}
		b.WriteRune(r)
		lastWasSpace = false
	}

	cleaned := strings.TrimSpace(b.String())
	runes := []rune(cleaned)
	if len(runes) > maxDeviceNameLen {
		return strings.TrimSpace(string(runes[:maxDeviceNameLen]))
	}
	return cleaned
}

// deviceFromUserAgent renders a User-Agent as "<browser> on <system>", or
// whichever half it could recognise. Empty when it recognises neither.
func deviceFromUserAgent(ua string) string {
	browser := matchToken(ua, []struct{ token, name string }{
		// Order matters: Edge and Opera both claim to be Chrome, and Chrome
		// claims to be Safari, so the impostors have to be tested first.
		{"Edg", "Edge"},
		{"OPR", "Opera"},
		{"Opera", "Opera"},
		{"Chrome", "Chrome"},
		{"Chromium", "Chromium"},
		{"Firefox", "Firefox"},
		{"Safari", "Safari"},
	})
	system := matchToken(ua, []struct{ token, name string }{
		// Android before Linux, which it also contains.
		{"Android", "Android"},
		{"iPhone", "iOS"},
		{"iPad", "iPadOS"},
		{"Windows", "Windows"},
		{"Macintosh", "macOS"},
		{"Mac OS X", "macOS"},
		{"CrOS", "ChromeOS"},
		{"Linux", "Linux"},
	})

	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	default:
		return system
	}
}

// matchToken returns the name of the first token present in s.
func matchToken(s string, candidates []struct{ token, name string }) string {
	for _, c := range candidates {
		if strings.Contains(s, c.token) {
			return c.name
		}
	}
	return ""
}

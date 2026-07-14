package handler

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// A BitTorrent client sends the info_hash either as 20 raw bytes (percent-
// encoded) or as 40 hex chars. Both must be accepted, and anything else
// refused — a short hash would otherwise match the wrong torrent.
func TestParseInfoHashParam(t *testing.T) {
	raw20 := "abcdefghij0123456789" // exactly 20 bytes
	hex40 := "6162636465666768696a30313233343536373839"

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "20 raw bytes", value: raw20, want: raw20},
		{name: "40 hex chars decode to the same 20 bytes", value: hex40, want: raw20},
		{name: "missing", value: "", wantErr: true},
		{name: "too short", value: "abc", wantErr: true},
		{name: "40 chars that are not hex", value: strings.Repeat("z", 40), wantErr: true},
		{name: "21 bytes", value: raw20 + "x", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{}
			if tc.value != "" {
				q.Set("info_hash", tc.value)
			}

			got, err := parseInfoHashParam(q)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsed %q, want an error", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseInfoHashParam(%q): %v", tc.value, err)
			}
			if string(got) != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if len(got) != 20 {
				t.Errorf("info hash is %d bytes, want 20", len(got))
			}
		})
	}
}

// The tracker speaks bencode, not HTTP status codes: the client only ever sees
// this string. Mapping an error to the wrong text is the whole failure mode.
func TestMapAnnounceError(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{service.ErrInvalidPasskey, "invalid passkey"},
		{service.ErrTorrentNotFound, "torrent not found"},
		{service.ErrTorrentBanned, "torrent is banned"},
		{service.ErrUserDisabled, "account is disabled"},
		{service.ErrTooManyPeers, "Too many peers on this torrent"},
		{service.ErrTooManyConns, "Too many connections"},
		{errors.New("something unexpected"), "internal server error"},
	}

	for _, tc := range tests {
		if got := mapAnnounceError(tc.err); got != tc.want {
			t.Errorf("mapAnnounceError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// A wait-time error is not a flat string: it has to tell the user how long is
// left, so it is formatted rather than mapped.
func TestMapAnnounceErrorFormatsWaitTime(t *testing.T) {
	err := &service.WaitTimeError{RemainingSeconds: 3660} // 1h 1m

	got := mapAnnounceError(err)
	if !strings.Contains(got, "1h 1m") {
		t.Errorf("mapAnnounceError = %q, want it to carry the remaining wait", got)
	}
}

func TestFormatWaitTimeMessage(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{7200, "2h"},
		{3660, "1h 1m"},
		{600, "10m"},
		{30, "less than 1m"},
		{0, "less than 1m"},
	}

	for _, tc := range tests {
		got := formatWaitTimeMessage(tc.seconds)
		if !strings.Contains(got, tc.want) {
			t.Errorf("formatWaitTimeMessage(%d) = %q, want it to contain %q", tc.seconds, got, tc.want)
		}
	}
}

package handler

import (
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// AnnounceHandler handles BitTorrent announce requests.
type AnnounceHandler struct {
	tracker *service.TrackerService
}

// NewAnnounceHandler creates a new AnnounceHandler.
func NewAnnounceHandler(tracker *service.TrackerService) *AnnounceHandler {
	return &AnnounceHandler{tracker: tracker}
}

// HandleAnnounce handles GET /announce?passkey=&info_hash=&peer_id=&port=&uploaded=&downloaded=&left=&event=
func (h *AnnounceHandler) HandleAnnounce(w http.ResponseWriter, r *http.Request) {
	req, err := parseAnnounceRequest(r)
	if err != nil {
		writeBencodedError(w, err.Error())
		return
	}

	resp, err := h.tracker.Announce(r.Context(), *req)
	if err != nil {
		writeBencodedError(w, mapAnnounceError(err))
		return
	}

	writeBencoded(w, resp)
}

func parseAnnounceRequest(r *http.Request) (*service.AnnounceRequest, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, errors.New("malformed query string")
	}

	passkey := q.Get("passkey")
	if passkey == "" {
		return nil, errors.New("missing passkey")
	}

	infoHash, err := parseInfoHashParam(q)
	if err != nil {
		return nil, err
	}

	peerID, err := parsePeerIDParam(q)
	if err != nil {
		return nil, err
	}

	portStr := q.Get("port")
	if portStr == "" {
		return nil, errors.New("missing port")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid port")
	}

	uploaded, err := parseInt64Param(q, "uploaded")
	if err != nil {
		return nil, errors.New("invalid uploaded")
	}

	downloaded, err := parseInt64Param(q, "downloaded")
	if err != nil {
		return nil, errors.New("invalid downloaded")
	}

	left, err := parseInt64Param(q, "left")
	if err != nil {
		return nil, errors.New("invalid left")
	}

	event := service.AnnounceEvent(q.Get("event"))
	switch event {
	case service.EventStarted, service.EventStopped, service.EventCompleted, service.EventEmpty:
		// valid
	default:
		return nil, errors.New("invalid event")
	}

	// Determine IP: use the client's IP from the request.
	ip := announceClientIP(r)

	return &service.AnnounceRequest{
		Passkey:    passkey,
		InfoHash:   infoHash,
		PeerID:     peerID,
		IP:         ip,
		Port:       port,
		Uploaded:   uploaded,
		Downloaded: downloaded,
		Left:       left,
		Event:      event,
	}, nil
}

// parseInfoHashParam extracts the info_hash from the query.
// Supports both 20-byte URL-encoded binary and 40-char hex formats.
func parseInfoHashParam(q url.Values) ([]byte, error) {
	raw := q.Get("info_hash")
	if raw == "" {
		return nil, errors.New("missing info_hash")
	}

	// URL-decoded value: if it's 20 bytes, use it directly.
	b := []byte(raw)
	if len(b) == 20 {
		return b, nil
	}

	// Try 40-char hex encoding.
	if len(raw) == 40 {
		decoded, err := hex.DecodeString(raw)
		if err == nil && len(decoded) == 20 {
			return decoded, nil
		}
	}

	return nil, errors.New("invalid info_hash: must be 20 bytes")
}

// parsePeerIDParam extracts the peer_id from the query.
func parsePeerIDParam(q url.Values) ([]byte, error) {
	raw := q.Get("peer_id")
	if raw == "" {
		return nil, errors.New("missing peer_id")
	}

	b := []byte(raw)
	if len(b) != 20 {
		return nil, errors.New("invalid peer_id: must be 20 bytes")
	}

	return b, nil
}

func parseInt64Param(q url.Values, name string) (int64, error) {
	s := q.Get(name)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// announceClientIP extracts the IP from the request, stripping the port.
func announceClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func mapAnnounceError(err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidPasskey):
		return "invalid passkey"
	case errors.Is(err, service.ErrTorrentNotFound):
		return "torrent not found"
	case errors.Is(err, service.ErrTorrentBanned):
		return "torrent is banned"
	case errors.Is(err, service.ErrUserDisabled):
		return "account is disabled"
	default:
		return "internal server error"
	}
}

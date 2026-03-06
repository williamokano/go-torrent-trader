package handler

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/zeebo/bencode"
)

const infoHashLength = 20

// ScrapeHandler handles BitTorrent scrape requests.
type ScrapeHandler struct {
	tracker *service.TrackerService
}

// NewScrapeHandler creates a new ScrapeHandler.
func NewScrapeHandler(tracker *service.TrackerService) *ScrapeHandler {
	return &ScrapeHandler{tracker: tracker}
}

// scrapeResponse is the top-level bencoded scrape response.
type scrapeResponse struct {
	Files map[string]service.ScrapeEntry `bencode:"files"`
}

// scrapeErrorResponse is the bencoded error response.
type scrapeErrorResponse struct {
	FailureReason string `bencode:"failure reason"`
}

// HandleScrape handles GET /scrape?info_hash=...
func (h *ScrapeHandler) HandleScrape(w http.ResponseWriter, r *http.Request) {
	infoHashes, err := parseInfoHashes(r.URL.RawQuery)
	if err != nil {
		writeBencodedError(w, "invalid info_hash parameter")
		return
	}

	if len(infoHashes) == 0 {
		writeBencodedError(w, "missing info_hash parameter")
		return
	}

	entries, err := h.tracker.Scrape(r.Context(), infoHashes)
	if err != nil {
		slog.Error("scrape failed", "error", err)
		writeBencodedError(w, "internal server error")
		return
	}

	resp := scrapeResponse{Files: entries}
	writeBencoded(w, resp)
}

// parseInfoHashes extracts info_hash values from the raw query string.
// We parse manually because Go's url.ParseQuery decodes keys/values as UTF-8
// strings, but info_hash values are raw 20-byte binary data that must be
// percent-decoded without UTF-8 interpretation.
func parseInfoHashes(rawQuery string) ([][]byte, error) {
	var hashes [][]byte

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, err
	}

	for _, v := range values["info_hash"] {
		raw := []byte(v)
		if len(raw) != infoHashLength {
			continue
		}
		hashes = append(hashes, raw)
	}

	return hashes, nil
}

func writeBencoded(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "text/plain")
	if err := bencode.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode bencode response", "error", err)
	}
}

func writeBencodedError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/plain")
	if err := bencode.NewEncoder(w).Encode(scrapeErrorResponse{FailureReason: message}); err != nil {
		slog.Error("failed to encode bencode error response", "error", err)
	}
}

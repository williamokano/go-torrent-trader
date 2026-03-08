package handler

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// RSSConfig holds the site metadata needed for RSS feed generation.
type RSSConfig struct {
	SiteName    string
	BaseURL     string // frontend URL (for torrent detail links)
	ApiURL      string // backend URL (for download enclosure URLs)
}

// RSSHandler handles RSS feed HTTP endpoints.
type RSSHandler struct {
	torrentSvc *service.TorrentService
	userRepo   repository.UserRepository
	config     RSSConfig
}

// NewRSSHandler creates a new RSSHandler.
func NewRSSHandler(torrentSvc *service.TorrentService, userRepo repository.UserRepository, cfg RSSConfig) *RSSHandler {
	return &RSSHandler{
		torrentSvc: torrentSvc,
		userRepo:   userRepo,
		config:     cfg,
	}
}

// RSS 2.0 XML structures

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string       `xml:"title"`
	Link        string       `xml:"link"`
	Description string       `xml:"description,omitempty"`
	PubDate     string       `xml:"pubDate"`
	GUID        string       `xml:"guid"`
	Enclosure   rssEnclosure `xml:"enclosure"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

const (
	rssDefaultLimit = 50
	rssMaxLimit     = 100
)

// HandleRSS handles GET /api/v1/rss.
func (h *RSSHandler) HandleRSS(w http.ResponseWriter, r *http.Request) {
	passkey := r.URL.Query().Get("passkey")
	if passkey == "" {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "passkey is required")
		return
	}

	// Validate passkey
	_, err := h.userRepo.GetByPasskey(r.Context(), passkey)
	if err != nil {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "invalid passkey")
		return
	}

	// Parse limit
	limit := rssDefaultLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > rssMaxLimit {
		limit = rssMaxLimit
	}

	// Build list options
	opts := repository.ListTorrentsOptions{
		SortBy:    "created_at",
		SortOrder: "desc",
		Page:      1,
		PerPage:   limit,
	}

	if catStr := r.URL.Query().Get("cat"); catStr != "" {
		catID, err := strconv.ParseInt(catStr, 10, 64)
		if err == nil && catID > 0 {
			opts.CategoryID = &catID
		}
	}

	torrents, _, err := h.torrentSvc.List(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list torrents")
		return
	}

	// Build RSS items
	items := make([]rssItem, 0, len(torrents))
	for i := range torrents {
		t := &torrents[i]
		desc := ""
		if t.Description != nil {
			desc = *t.Description
		}

		downloadURL := fmt.Sprintf("%s/api/v1/torrents/%d/download?passkey=%s", h.config.ApiURL, t.ID, passkey)

		items = append(items, rssItem{
			Title:       t.Name,
			Link:        fmt.Sprintf("%s/torrent/%d", h.config.BaseURL, t.ID),
			Description: desc,
			PubDate:     t.CreatedAt.UTC().Format(time.RFC1123Z),
			GUID:        fmt.Sprintf("%d", t.ID),
			Enclosure: rssEnclosure{
				URL:    downloadURL,
				Length: t.Size,
				Type:   "application/x-bittorrent",
			},
		})
	}

	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:       h.config.SiteName,
			Link:        h.config.BaseURL,
			Description: fmt.Sprintf("Latest torrents on %s", h.config.SiteName),
			Items:       items,
		},
	}

	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to generate RSS feed")
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(output)
}

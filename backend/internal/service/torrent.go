package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/zeebo/bencode"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/storage"
)

var (
	ErrDuplicateTorrent = errors.New("torrent with this info_hash already exists")
	ErrTorrentNotFound  = errors.New("torrent not found")
	ErrInvalidTorrent   = errors.New("invalid torrent file")
	ErrForbidden        = errors.New("forbidden")
)

// torrentMeta represents the top-level structure of a .torrent file.
type torrentMeta struct {
	Announce string          `bencode:"announce"`
	Info     bencode.RawMessage `bencode:"info"`
}

// torrentInfo holds the decoded info dictionary fields we need.
type torrentInfo struct {
	Name        string           `bencode:"name"`
	PieceLength int64            `bencode:"piece length"`
	Pieces      string           `bencode:"pieces"`
	Length      int64            `bencode:"length"`       // single-file mode
	Files       []torrentFile    `bencode:"files"`        // multi-file mode
}

type torrentFile struct {
	Length int64    `bencode:"length"`
	Path   []string `bencode:"path"`
}

// ParsedTorrent holds the extracted metadata from a .torrent file.
type ParsedTorrent struct {
	InfoHash  []byte
	Name      string
	Size      int64
	FileCount int
	RawBytes  []byte // original .torrent file content
}

// UploadTorrentRequest holds the input for torrent upload.
type UploadTorrentRequest struct {
	Name        string
	Description string
	CategoryID  int64
	Anonymous   bool
}

// EditTorrentRequest holds the input for editing a torrent.
type EditTorrentRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	CategoryID  *int64  `json:"category_id"`
	Anonymous   *bool   `json:"anonymous"`
	// Staff-only fields (admin group_id=1)
	Banned *bool `json:"banned"`
	Free   *bool `json:"free"`
}

// TorrentService handles torrent business logic.
// TorrentServiceConfig holds configurable values for torrent file rewriting.
type TorrentServiceConfig struct {
	AnnounceURL      string // base announce URL, e.g. "http://localhost:8080/announce"
	TorrentComment   string // written to "comment" field in downloaded .torrent (empty = don't rewrite)
	TorrentCreatedBy string // written to "created by" field in downloaded .torrent (empty = don't rewrite)
}

type TorrentService struct {
	torrents         repository.TorrentRepository
	users            repository.UserRepository
	storage          storage.FileStorage
	announceURL      string
	torrentComment   string
	torrentCreatedBy string
	eventBus         event.Bus
}

// NewTorrentService creates a new TorrentService.
func NewTorrentService(
	torrents repository.TorrentRepository,
	users repository.UserRepository,
	store storage.FileStorage,
	cfg TorrentServiceConfig,
	bus event.Bus,
) *TorrentService {
	return &TorrentService{
		torrents:         torrents,
		users:            users,
		storage:          store,
		announceURL:      cfg.AnnounceURL,
		torrentComment:   cfg.TorrentComment,
		torrentCreatedBy: cfg.TorrentCreatedBy,
		eventBus:         bus,
	}
}

func (s *TorrentService) actorFromUserID(ctx context.Context, userID int64) event.Actor {
	actor := event.Actor{ID: userID}
	if u, err := s.users.GetByID(ctx, userID); err == nil {
		actor.Username = u.Username
	}
	return actor
}

// ParseTorrentFile parses a .torrent file and extracts metadata.
func ParseTorrentFile(data []byte) (*ParsedTorrent, error) {
	var meta torrentMeta
	if err := bencode.DecodeBytes(data, &meta); err != nil {
		return nil, fmt.Errorf("%w: failed to decode bencode: %v", ErrInvalidTorrent, err)
	}

	if len(meta.Info) == 0 {
		return nil, fmt.Errorf("%w: missing info dictionary", ErrInvalidTorrent)
	}

	// Compute info_hash as SHA1 of the bencoded info dictionary.
	hash := sha1.Sum(meta.Info)

	var info torrentInfo
	if err := bencode.DecodeBytes(meta.Info, &info); err != nil {
		return nil, fmt.Errorf("%w: failed to decode info dictionary: %v", ErrInvalidTorrent, err)
	}

	if info.Name == "" {
		return nil, fmt.Errorf("%w: missing name in info dictionary", ErrInvalidTorrent)
	}

	var totalSize int64
	fileCount := 1
	if len(info.Files) > 0 {
		// Multi-file mode
		fileCount = len(info.Files)
		for _, f := range info.Files {
			totalSize += f.Length
		}
	} else {
		// Single-file mode
		totalSize = info.Length
	}

	return &ParsedTorrent{
		InfoHash:  hash[:],
		Name:      info.Name,
		Size:      totalSize,
		FileCount: fileCount,
		RawBytes:  data,
	}, nil
}

// Upload parses a .torrent file, checks for duplicates, stores it, and creates a DB record.
func (s *TorrentService) Upload(ctx context.Context, fileData []byte, req UploadTorrentRequest, uploaderID int64) (*model.Torrent, error) {
	parsed, err := ParseTorrentFile(fileData)
	if err != nil {
		return nil, err
	}

	// Duplicate check
	existing, err := s.torrents.GetByInfoHash(ctx, parsed.InfoHash)
	if err == nil && existing != nil {
		return nil, ErrDuplicateTorrent
	}

	// Use parsed name if no custom name provided
	name := req.Name
	if name == "" {
		name = parsed.Name
	}

	torrent := &model.Torrent{
		Name:       name,
		InfoHash:   parsed.InfoHash,
		Size:       parsed.Size,
		CategoryID: req.CategoryID,
		UploaderID: uploaderID,
		Anonymous:  req.Anonymous,
		Visible:    true,
		FileCount:  parsed.FileCount,
	}
	if req.Description != "" {
		torrent.Description = &req.Description
	}

	if err := s.torrents.Create(ctx, torrent); err != nil {
		// Handle DB unique constraint as duplicate
		errMsg := err.Error()
		if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "duplicate") {
			return nil, ErrDuplicateTorrent
		}
		return nil, fmt.Errorf("create torrent: %w", err)
	}

	// Store the .torrent file
	storageKey := fmt.Sprintf("torrents/%d.torrent", torrent.ID)
	if err := s.storage.Put(ctx, storageKey, bytes.NewReader(parsed.RawBytes)); err != nil {
		// Best effort: log but don't fail the upload since DB record exists
		slog.Error("failed to store torrent file", "torrent_id", torrent.ID, "error", err)
		return nil, fmt.Errorf("store torrent file: %w", err)
	}

	s.eventBus.Publish(ctx, &event.TorrentUploadedEvent{
		Base:        event.NewBase(event.TorrentUploaded, s.actorFromUserID(ctx, uploaderID)),
		TorrentID:   torrent.ID,
		TorrentName: torrent.Name,
	})

	return torrent, nil
}

// GetByID returns a torrent by its ID.
func (s *TorrentService) GetByID(ctx context.Context, id int64) (*model.Torrent, error) {
	torrent, err := s.torrents.GetByID(ctx, id)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	return torrent, nil
}

// List returns a paginated list of torrents.
func (s *TorrentService) List(ctx context.Context, opts repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 25
	}
	if opts.PerPage > 100 {
		opts.PerPage = 100
	}
	return s.torrents.List(ctx, opts)
}

// DownloadTorrent retrieves the .torrent file and rewrites the announce URL with the user's passkey.
func (s *TorrentService) DownloadTorrent(ctx context.Context, torrentID, userID int64) ([]byte, string, error) {
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, "", ErrTorrentNotFound
	}

	// Get user's passkey
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("get user: %w", err)
	}

	storageKey := fmt.Sprintf("torrents/%d.torrent", torrentID)
	rc, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		return nil, "", fmt.Errorf("get torrent file: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", fmt.Errorf("read torrent file: %w", err)
	}

	// Rewrite announce URL with user's passkey
	rewritten, err := s.rewriteAnnounce(data, user.Passkey)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite announce: %w", err)
	}

	filename := torrent.Name + ".torrent"
	return rewritten, filename, nil
}

// EditTorrent updates a torrent's metadata. Only the owner or staff may edit.
// Staff-only fields (banned, free) are rejected if the caller is not an admin.
func (s *TorrentService) EditTorrent(ctx context.Context, torrentID, userID int64, perms model.Permissions, req EditTorrentRequest) (*model.Torrent, error) {
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, ErrTorrentNotFound
	}

	isOwner := torrent.UploaderID == userID

	if !isOwner && !perms.IsStaff() {
		return nil, ErrForbidden
	}

	// Reject staff-only fields from non-admins
	if !perms.IsAdmin {
		if req.Banned != nil || req.Free != nil {
			return nil, ErrForbidden
		}
	}

	// Apply changes
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", ErrInvalidTorrent)
		}
		torrent.Name = name
	}
	if req.Description != nil {
		torrent.Description = req.Description
	}
	if req.CategoryID != nil {
		if *req.CategoryID <= 0 {
			return nil, fmt.Errorf("%w: invalid category_id", ErrInvalidTorrent)
		}
		torrent.CategoryID = *req.CategoryID
	}
	if req.Anonymous != nil {
		torrent.Anonymous = *req.Anonymous
	}
	if req.Banned != nil {
		torrent.Banned = *req.Banned
	}
	if req.Free != nil {
		torrent.Free = *req.Free
	}

	if err := s.torrents.Update(ctx, torrent); err != nil {
		return nil, fmt.Errorf("update torrent: %w", err)
	}

	s.eventBus.Publish(ctx, &event.TorrentEditedEvent{
		Base:        event.NewBase(event.TorrentEdited, s.actorFromUserID(ctx, userID)),
		TorrentID:   torrent.ID,
		TorrentName: torrent.Name,
	})

	return torrent, nil
}

// DeleteTorrent removes a torrent and its stored file. Only the owner or staff may delete.
func (s *TorrentService) DeleteTorrent(ctx context.Context, torrentID, userID int64, perms model.Permissions) error {
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return ErrTorrentNotFound
	}

	isOwner := torrent.UploaderID == userID

	if !isOwner && !perms.IsStaff() {
		return ErrForbidden
	}

	// Delete from storage first (best effort — log and continue if file missing)
	storageKey := fmt.Sprintf("torrents/%d.torrent", torrentID)
	if err := s.storage.Delete(ctx, storageKey); err != nil {
		slog.Error("failed to delete torrent file from storage", "torrent_id", torrentID, "error", err)
	}

	// Delete from DB
	if err := s.torrents.Delete(ctx, torrentID); err != nil {
		return fmt.Errorf("delete torrent: %w", err)
	}

	s.eventBus.Publish(ctx, &event.TorrentDeletedEvent{
		Base:        event.NewBase(event.TorrentDeleted, s.actorFromUserID(ctx, userID)),
		TorrentID:   torrentID,
		TorrentName: torrent.Name,
	})

	return nil
}

// rewriteAnnounce decodes the torrent, sets the announce URL, and re-encodes.
func (s *TorrentService) rewriteAnnounce(data []byte, passkey *string) ([]byte, error) {
	var meta map[string]bencode.RawMessage
	if err := bencode.DecodeBytes(data, &meta); err != nil {
		return nil, fmt.Errorf("decode torrent: %w", err)
	}

	announceURL := s.announceURL
	if passkey != nil && *passkey != "" {
		announceURL = fmt.Sprintf("%s?passkey=%s", s.announceURL, *passkey)
	}

	encoded, err := bencode.EncodeBytes(announceURL)
	if err != nil {
		return nil, fmt.Errorf("encode announce URL: %w", err)
	}
	meta["announce"] = encoded

	// Remove announce-list to avoid multi-tracker leaking
	delete(meta, "announce-list")

	// Replace comment and created-by with configurable values
	if s.torrentComment != "" {
		enc, err := bencode.EncodeBytes(s.torrentComment)
		if err == nil {
			meta["comment"] = enc
		}
	}

	if s.torrentCreatedBy != "" {
		enc, err := bencode.EncodeBytes(s.torrentCreatedBy)
		if err == nil {
			meta["created by"] = enc
		}
	}

	result, err := bencode.EncodeBytes(meta)
	if err != nil {
		return nil, fmt.Errorf("encode torrent: %w", err)
	}

	return result, nil
}

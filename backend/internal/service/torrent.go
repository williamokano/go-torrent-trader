package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
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
	ErrDuplicateTorrent      = errors.New("torrent with this info_hash already exists")
	ErrTorrentNotFound       = errors.New("torrent not found")
	ErrInvalidTorrent        = errors.New("invalid torrent file")
	ErrForbidden             = errors.New("forbidden")
	ErrDuplicateReseedRequest = errors.New("you have already requested a reseed for this torrent")
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
	Files     []model.TorrentFile // individual files with paths and sizes
	RawBytes  []byte              // original .torrent file content
}

// UploadTorrentRequest holds the input for torrent upload.
type UploadTorrentRequest struct {
	Name        string
	Description string
	Nfo         string
	CategoryID  int64
	Anonymous   bool
}

// EditTorrentRequest holds the input for editing a torrent.
type EditTorrentRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Nfo         *string `json:"nfo"`
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
	db               *sql.DB
	torrents         repository.TorrentRepository
	users            repository.UserRepository
	storage          storage.FileStorage
	announceURL      string
	torrentComment   string
	torrentCreatedBy string
	eventBus         event.Bus
	reseedRequests   repository.ReseedRequestRepository
}

// NewTorrentService creates a new TorrentService.
func NewTorrentService(
	db *sql.DB,
	torrents repository.TorrentRepository,
	users repository.UserRepository,
	store storage.FileStorage,
	cfg TorrentServiceConfig,
	bus event.Bus,
	reseedRequests repository.ReseedRequestRepository,
) *TorrentService {
	return &TorrentService{
		db:               db,
		torrents:         torrents,
		users:            users,
		storage:          store,
		announceURL:      cfg.AnnounceURL,
		torrentComment:   cfg.TorrentComment,
		torrentCreatedBy: cfg.TorrentCreatedBy,
		eventBus:         bus,
		reseedRequests:   reseedRequests,
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
	var files []model.TorrentFile
	if len(info.Files) > 0 {
		// Multi-file mode
		fileCount = len(info.Files)
		files = make([]model.TorrentFile, len(info.Files))
		for i, f := range info.Files {
			totalSize += f.Length
			files[i] = model.TorrentFile{
				Path: strings.Join(f.Path, "/"),
				Size: f.Length,
			}
		}
	} else {
		// Single-file mode
		totalSize = info.Length
		files = []model.TorrentFile{{Path: info.Name, Size: info.Length}}
	}

	return &ParsedTorrent{
		InfoHash:  hash[:],
		Name:      info.Name,
		Size:      totalSize,
		FileCount: fileCount,
		Files:     files,
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

	// Serialize file list to JSON for storage
	var filesJSON *json.RawMessage
	if len(parsed.Files) > 0 {
		if data, err := json.Marshal(parsed.Files); err == nil {
			raw := json.RawMessage(data)
			filesJSON = &raw
		}
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
		Files:      filesJSON,
	}
	if req.Description != "" {
		torrent.Description = &req.Description
	}
	if req.Nfo != "" {
		torrent.Nfo = &req.Nfo
	}

	if s.db != nil {
		// Production path: use a transaction so DB insert + file storage are atomic
		err = repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			createQuery := `INSERT INTO torrents (
				name, info_hash, size, description, nfo, category_id, uploader_id,
				anonymous, seeders, leechers, times_completed, comments_count,
				visible, banned, free, silver, file_count, files
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15, $16, $17, $18
			) RETURNING id, created_at, updated_at`

			if err := tx.QueryRowContext(ctx, createQuery,
				torrent.Name, torrent.InfoHash, torrent.Size, torrent.Description,
				torrent.Nfo, torrent.CategoryID, torrent.UploaderID, torrent.Anonymous,
				torrent.Seeders, torrent.Leechers, torrent.TimesCompleted, torrent.CommentsCount,
				torrent.Visible, torrent.Banned, torrent.Free, torrent.Silver, torrent.FileCount,
				torrent.Files,
			).Scan(&torrent.ID, &torrent.CreatedAt, &torrent.UpdatedAt); err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "duplicate") {
					return ErrDuplicateTorrent
				}
				return fmt.Errorf("create torrent: %w", err)
			}

			storageKey := fmt.Sprintf("torrents/%d.torrent", torrent.ID)
			if err := s.storage.Put(ctx, storageKey, bytes.NewReader(parsed.RawBytes)); err != nil {
				return fmt.Errorf("store torrent file: %w", err)
			}

			return nil
		})
	} else {
		// Test path: no real DB, use repo interface directly
		if err = s.torrents.Create(ctx, torrent); err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "duplicate") {
				return nil, ErrDuplicateTorrent
			}
			return nil, fmt.Errorf("create torrent: %w", err)
		}

		storageKey := fmt.Sprintf("torrents/%d.torrent", torrent.ID)
		if err = s.storage.Put(ctx, storageKey, bytes.NewReader(parsed.RawBytes)); err != nil {
			return nil, fmt.Errorf("store torrent file: %w", err)
		}
	}
	if err != nil {
		return nil, err
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
	if req.Nfo != nil {
		torrent.Nfo = req.Nfo
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
		slog.Warn("torrent file not found in storage (may already be deleted)", "torrent_id", torrentID, "error", err)
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

// RequestReseed creates a reseed request for a torrent.
func (s *TorrentService) RequestReseed(ctx context.Context, torrentID, userID int64) error {
	// Validate torrent exists
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return ErrTorrentNotFound
	}

	// Check for duplicate request
	exists, err := s.reseedRequests.ExistsByTorrentAndUser(ctx, torrentID, userID)
	if err != nil {
		return fmt.Errorf("check reseed request: %w", err)
	}
	if exists {
		return ErrDuplicateReseedRequest
	}

	req := &model.ReseedRequest{
		TorrentID:   torrentID,
		RequesterID: userID,
	}
	if err := s.reseedRequests.Create(ctx, req); err != nil {
		return fmt.Errorf("create reseed request: %w", err)
	}

	// Publish event with uploader info for email notification
	actor := s.actorFromUserID(ctx, userID)
	uploaderEmail := ""
	if uploader, err := s.users.GetByID(ctx, torrent.UploaderID); err == nil {
		uploaderEmail = uploader.Email
	}
	s.eventBus.Publish(ctx, &event.ReseedRequestedEvent{
		Base:          event.NewBase(event.ReseedRequested, actor),
		TorrentID:     torrent.ID,
		TorrentName:   torrent.Name,
		UploaderID:    torrent.UploaderID,
		UploaderEmail: uploaderEmail,
	})

	return nil
}

// GetReseedCount returns the number of reseed requests for a torrent.
func (s *TorrentService) GetReseedCount(ctx context.Context, torrentID int64) (int, error) {
	count, err := s.reseedRequests.CountByTorrent(ctx, torrentID)
	if err != nil {
		return 0, fmt.Errorf("count reseed requests: %w", err)
	}
	return count, nil
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

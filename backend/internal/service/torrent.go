package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/zeebo/bencode"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/metadata"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/storage"
)

var (
	ErrDuplicateTorrent       = errors.New("torrent with this info_hash already exists")
	ErrTorrentNotFound        = errors.New("torrent not found")
	ErrInvalidTorrent         = errors.New("invalid torrent file")
	ErrForbidden              = errors.New("forbidden")
	ErrDuplicateReseedRequest = errors.New("you have already requested a reseed for this torrent")
	// ErrModerationUnavailable is returned when a moderation action is attempted
	// but the moderation repository was never wired (e.g. in a minimal test setup).
	ErrModerationUnavailable = errors.New("moderation is unavailable")
	// ErrNotPending is returned when approve/reject targets a torrent that is not
	// awaiting moderation.
	ErrNotPending = errors.New("torrent is not pending moderation")
	// ErrEmptyMessage is returned when a moderation message body is blank.
	ErrEmptyMessage = errors.New("message body cannot be empty")
)

// torrentMeta represents the top-level structure of a .torrent file.
type torrentMeta struct {
	Announce string             `bencode:"announce"`
	Info     bencode.RawMessage `bencode:"info"`
}

// torrentInfo holds the decoded info dictionary fields we need.
type torrentInfo struct {
	Name        string        `bencode:"name"`
	PieceLength int64         `bencode:"piece length"`
	Pieces      string        `bencode:"pieces"`
	Length      int64         `bencode:"length"` // single-file mode
	Files       []torrentFile `bencode:"files"`  // multi-file mode
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
	// Metadata holds category-schema field values submitted with the upload.
	Metadata map[string]any
}

// EditTorrentRequest holds the input for editing a torrent.
type EditTorrentRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Nfo         *string `json:"nfo"`
	CategoryID  *int64  `json:"category_id"`
	Anonymous   *bool   `json:"anonymous"`
	// Metadata, when non-nil, replaces the whole metadata object; nil leaves the
	// stored values untouched. Validated against the effective category schema.
	Metadata *json.RawMessage `json:"metadata"`
	// Staff-only fields (admin group_id=1)
	Banned *bool `json:"banned"`
	Free   *bool `json:"free"`
	Silver *bool `json:"silver"`
}

// TorrentService handles torrent business logic.
// TorrentServiceConfig holds configurable values for torrent file rewriting.
type TorrentServiceConfig struct {
	AnnounceURL      string // base announce URL, e.g. "http://localhost:8080/announce"
	TorrentComment   string // written to "comment" field in downloaded .torrent (empty = don't rewrite)
	TorrentCreatedBy string // written to "created by" field in downloaded .torrent (empty = don't rewrite)
}

type TorrentService struct {
	db                 *sql.DB
	torrents           repository.TorrentRepository
	users              repository.UserRepository
	categories         repository.CategoryRepository
	storage            storage.FileStorage
	announceURL        string
	torrentComment     string
	torrentCreatedBy   string
	eventBus           event.Bus
	reseedRequests     repository.ReseedRequestRepository
	moderation         repository.TorrentModerationRepository
	moderationMessages repository.TorrentModerationMessageRepository
	siteSettings       *SiteSettingsService
}

// SetCategoryRepo injects the category repository used to resolve per-category
// metadata schemas for upload/edit validation. Wired in production bootstrap;
// left unset in tests that don't exercise metadata (schema then resolves empty).
func (s *TorrentService) SetCategoryRepo(repo repository.CategoryRepository) {
	s.categories = repo
}

// SetModerationRepo injects the moderation write/queue repository (BE-8.22).
// Wired in production bootstrap; left unset in tests that don't exercise
// moderation (moderation actions then return ErrModerationUnavailable).
func (s *TorrentService) SetModerationRepo(repo repository.TorrentModerationRepository) {
	s.moderation = repo
}

// SetSiteSettings injects the site-settings service used to read the moderation
// config (master switch + public-visibility toggle). When unset, uploads moderate
// by default and pending torrents stay hidden from non-author/non-staff viewers.
func (s *TorrentService) SetSiteSettings(settings *SiteSettingsService) {
	s.siteSettings = settings
}

// SetModerationMessageRepo injects the moderation-thread repository (BE-8.22b).
func (s *TorrentService) SetModerationMessageRepo(repo repository.TorrentModerationMessageRepository) {
	s.moderationMessages = repo
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

	// Validate submitted metadata against the category's effective schema.
	metaJSON, err := s.validateMetadata(ctx, req.CategoryID, req.Metadata)
	if err != nil {
		return nil, err
	}

	// Serialize file list to JSON for storage
	var filesJSON *json.RawMessage
	if len(parsed.Files) > 0 {
		if data, err := json.Marshal(parsed.Files); err == nil {
			raw := json.RawMessage(data)
			filesJSON = &raw
		}
	}

	// Moderation gate (BE-8.22): new uploads are pending until approved. When the
	// moderation master switch is off, they auto-approve (no human approver
	// recorded) to preserve the legacy publish-on-upload behavior.
	moderationStatus := model.ModerationPending
	if s.siteSettings != nil && !s.siteSettings.GetBool(ctx, SettingModerationEnabled, true) {
		moderationStatus = model.ModerationApproved
	}

	torrent := &model.Torrent{
		Name:             name,
		InfoHash:         parsed.InfoHash,
		Size:             parsed.Size,
		CategoryID:       req.CategoryID,
		UploaderID:       uploaderID,
		Anonymous:        req.Anonymous,
		Visible:          true,
		ModerationStatus: moderationStatus,
		FileCount:        parsed.FileCount,
		Files:            filesJSON,
		Metadata:         metaJSON,
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
				visible, banned, free, silver, file_count, files, metadata,
				moderation_status
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
			) RETURNING id, created_at, updated_at`

			if err := tx.QueryRowContext(ctx, createQuery,
				torrent.Name, torrent.InfoHash, torrent.Size, torrent.Description,
				torrent.Nfo, torrent.CategoryID, torrent.UploaderID, torrent.Anonymous,
				torrent.Seeders, torrent.Leechers, torrent.TimesCompleted, torrent.CommentsCount,
				torrent.Visible, torrent.Banned, torrent.Free, torrent.Silver, torrent.FileCount,
				torrent.Files, torrent.Metadata, torrent.ModerationStatus,
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

	// With moderation off the upload is public the moment it lands, so this is
	// also its publish point. Re-fetch so the event carries the JOIN-resolved
	// category name the announcement renderers need — the model built above only
	// has the IDs. A failed re-fetch loses the announcement, not the upload.
	if torrent.ModerationStatus == model.ModerationApproved {
		published, err := s.torrents.GetByID(ctx, torrent.ID)
		if err != nil {
			slog.Error("torrent: failed to re-fetch auto-approved torrent for publish event",
				"torrent_id", torrent.ID, "error", err)
		} else {
			s.publishPublished(ctx, published, uploaderID)
		}
	}

	return torrent, nil
}

// validateMetadata resolves the category's effective metadata schema, validates
// the submitted values against it, and returns a canonical JSONB object to
// persist (always at least "{}"). Unknown keys / constraint violations surface
// as metadata.ErrInvalidValues, which the handler maps to a 422.
func (s *TorrentService) validateMetadata(ctx context.Context, categoryID int64, values map[string]any) (json.RawMessage, error) {
	var schema []metadata.FieldDef
	if s.categories != nil {
		resolved, err := ResolveCategorySchema(ctx, s.categories, categoryID)
		if err != nil {
			// A missing category can't carry metadata; fall through with an empty
			// schema so ValidateValues rejects any submitted values and the
			// downstream insert/update surfaces the real FK error.
			if !errors.Is(err, ErrCategoryNotFound) {
				return nil, err
			}
		} else {
			schema = resolved
		}
	}

	canonical, err := metadata.ValidateValues(schema, values)
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return json.RawMessage("{}"), nil
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return data, nil
}

// GetByID returns a torrent by its ID.
func (s *TorrentService) GetByID(ctx context.Context, id int64) (*model.Torrent, error) {
	torrent, err := s.torrents.GetByID(ctx, id)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	return torrent, nil
}

// GetByIDForViewer returns a torrent only if the viewer is allowed to see it. A
// non-approved torrent is visible to its uploader and to staff always; to everyone
// else only a pending one, and only when the public-visibility setting is on. When
// hidden, it returns ErrTorrentNotFound so a non-viewer can't even confirm the
// torrent exists.
func (s *TorrentService) GetByIDForViewer(ctx context.Context, id, userID int64, perms model.Permissions) (*model.Torrent, error) {
	torrent, err := s.torrents.GetByID(ctx, id)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	if !s.canViewTorrent(ctx, torrent, userID, perms) {
		return nil, ErrTorrentNotFound
	}
	return torrent, nil
}

// canViewTorrent centralizes the pending-torrent visibility rule shared by the
// detail and download paths.
func (s *TorrentService) canViewTorrent(ctx context.Context, torrent *model.Torrent, userID int64, perms model.Permissions) bool {
	if !torrent.ModerationRestricted() {
		return true
	}
	if torrent.UploaderID == userID || perms.IsStaff() {
		return true
	}
	// A rejected torrent is never publicly viewable; a pending one is only when the
	// site opts into public visibility of the moderation pipeline.
	if torrent.ModerationStatus == model.ModerationPending &&
		s.siteSettings != nil &&
		s.siteSettings.GetBool(ctx, SettingModerationPublicVisibility, false) {
		return true
	}
	return false
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
// A pending/rejected torrent may only be downloaded by its uploader or by staff —
// nobody else can obtain the file (and thus can't seed it) until it is approved.
func (s *TorrentService) DownloadTorrent(ctx context.Context, torrentID, userID int64, perms model.Permissions) ([]byte, string, error) {
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, "", ErrTorrentNotFound
	}

	if torrent.ModerationRestricted() && torrent.UploaderID != userID && !perms.IsStaff() {
		return nil, "", ErrForbidden
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
		if req.Banned != nil || req.Free != nil || req.Silver != nil {
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
	if req.Silver != nil {
		torrent.Silver = *req.Silver
	}

	// Metadata, when provided, replaces the whole object and is validated against
	// the effective schema of the (possibly newly assigned) category. Omitting it
	// leaves the stored values untouched.
	if req.Metadata != nil {
		var values map[string]any
		if len(*req.Metadata) > 0 {
			if err := json.Unmarshal(*req.Metadata, &values); err != nil {
				return nil, fmt.Errorf("%w: invalid metadata JSON", ErrInvalidTorrent)
			}
		}
		metaJSON, err := s.validateMetadata(ctx, torrent.CategoryID, values)
		if err != nil {
			return nil, err
		}
		torrent.Metadata = metaJSON
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

// ListModerationQueue returns torrents awaiting (or in the requested) moderation
// state. Caller must be staff (enforced at the route).
func (s *TorrentService) ListModerationQueue(ctx context.Context, opts repository.ModerationQueueOptions) ([]model.Torrent, int64, error) {
	if s.moderation == nil {
		return nil, 0, ErrModerationUnavailable
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 25
	}
	if opts.PerPage > 100 {
		opts.PerPage = 100
	}
	return s.moderation.ListModerationQueue(ctx, opts)
}

// ClaimModeration assigns the torrent to moderatorID. Staff can steal an existing
// claim (a stale moderator shouldn't block review). Only pending torrents can be
// claimed.
func (s *TorrentService) ClaimModeration(ctx context.Context, torrentID, moderatorID int64) (*model.Torrent, error) {
	if s.moderation == nil {
		return nil, ErrModerationUnavailable
	}
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	if torrent.ModerationStatus != model.ModerationPending {
		return nil, ErrNotPending
	}
	if err := s.moderation.ClaimModeration(ctx, torrentID, moderatorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTorrentNotFound
		}
		return nil, fmt.Errorf("claim moderation: %w", err)
	}
	return s.torrents.GetByID(ctx, torrentID)
}

// UnclaimModeration clears the torrent's assigned moderator.
func (s *TorrentService) UnclaimModeration(ctx context.Context, torrentID int64) (*model.Torrent, error) {
	if s.moderation == nil {
		return nil, ErrModerationUnavailable
	}
	if _, err := s.torrents.GetByID(ctx, torrentID); err != nil {
		return nil, ErrTorrentNotFound
	}
	if err := s.moderation.UnclaimModeration(ctx, torrentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTorrentNotFound
		}
		return nil, fmt.Errorf("unclaim moderation: %w", err)
	}
	return s.torrents.GetByID(ctx, torrentID)
}

// ApproveTorrent marks a pending torrent approved, recording the approver. Allowed
// for staff; the Uploader class extends this to self-approval of their own uploads
// (see canApprove).
func (s *TorrentService) ApproveTorrent(ctx context.Context, torrentID, userID int64, perms model.Permissions) (*model.Torrent, error) {
	if s.moderation == nil {
		return nil, ErrModerationUnavailable
	}
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	if !s.canApprove(torrent, userID, perms) {
		return nil, ErrForbidden
	}
	if torrent.ModerationStatus == model.ModerationApproved {
		return nil, ErrNotPending
	}
	if err := s.moderation.ApproveTorrent(ctx, torrentID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTorrentNotFound
		}
		return nil, fmt.Errorf("approve torrent: %w", err)
	}
	s.publishModerated(ctx, torrent, userID, model.ModerationApproved)
	published, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, err
	}
	// Approval is the other publish point. A rejected torrent later approved is
	// a legitimate first publish. Two racing approvals could both reach here —
	// the ErrNotPending check above is a read before an unconditional UPDATE —
	// but the connector pipeline dedupes on (instance, torrent) in the database,
	// so the announcement still happens exactly once.
	s.publishPublished(ctx, published, userID)
	return published, nil
}

// publishPublished emits the canonical TorrentPublished event that external
// notification connectors (BE-10) subscribe to. The torrent must be a
// JOIN-resolved read (GetByID), so CategoryName and UploaderName are populated.
//
// An anonymous torrent's uploader name is dropped here, at the source, rather
// than by each renderer: the event never carries it, so no listener, stored
// payload or connector output downstream can leak it even by mistake.
func (s *TorrentService) publishPublished(ctx context.Context, torrent *model.Torrent, actorID int64) {
	// A banned or hidden torrent is not public, whatever its moderation status
	// says — announcing it externally would hand out a link members cannot use.
	if torrent.Banned || !torrent.Visible {
		return
	}

	uploaderName := torrent.UploaderName
	actor := s.actorFromUserID(ctx, actorID)
	if torrent.Anonymous {
		uploaderName = ""
		// On the auto-approve upload path the actor IS the uploader, so leaving
		// the username on the event would smuggle the very name the field above
		// just dropped. No consumer of this event needs it.
		actor.Username = ""
	}
	s.eventBus.Publish(ctx, &event.TorrentPublishedEvent{
		Base:         event.NewBase(event.TorrentPublished, actor),
		TorrentID:    torrent.ID,
		Name:         torrent.Name,
		InfoHashHex:  hex.EncodeToString(torrent.InfoHash),
		CategoryID:   torrent.CategoryID,
		CategoryName: torrent.CategoryName,
		Size:         torrent.Size,
		FileCount:    torrent.FileCount,
		UploaderID:   torrent.UploaderID,
		UploaderName: uploaderName,
		Anonymous:    torrent.Anonymous,
		Freeleech:    torrent.Free,
		Silver:       torrent.Silver,
		PublishedAt:  time.Now(),
	})
}

// publishModerated emits a TorrentModeratedEvent so the uploader is notified of an
// approve/reject decision. The actor is the moderator; NotificationService.Create
// skips self-notification, so an Uploader approving their own upload isn't pinged.
func (s *TorrentService) publishModerated(ctx context.Context, torrent *model.Torrent, actorID int64, decision string) {
	s.eventBus.Publish(ctx, &event.TorrentModeratedEvent{
		Base:        event.NewBase(event.TorrentModerated, s.actorFromUserID(ctx, actorID)),
		TorrentID:   torrent.ID,
		TorrentName: torrent.Name,
		UploaderID:  torrent.UploaderID,
		Decision:    decision,
	})
}

// RejectTorrent marks a pending torrent rejected. Staff only.
func (s *TorrentService) RejectTorrent(ctx context.Context, torrentID, userID int64, perms model.Permissions) (*model.Torrent, error) {
	if s.moderation == nil {
		return nil, ErrModerationUnavailable
	}
	if !perms.IsStaff() {
		return nil, ErrForbidden
	}
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	if torrent.ModerationStatus != model.ModerationPending {
		return nil, ErrNotPending
	}
	if err := s.moderation.RejectTorrent(ctx, torrentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTorrentNotFound
		}
		return nil, fmt.Errorf("reject torrent: %w", err)
	}
	s.publishModerated(ctx, torrent, userID, model.ModerationRejected)
	return s.torrents.GetByID(ctx, torrentID)
}

// canApprove decides who may approve a torrent. Staff always may; the Uploader
// class (BE-8.22c) may approve their own upload — with their own name recorded as
// the approver — which is the whole point of the class: a human self-review that
// is still logged, rather than publishing on upload.
func (s *TorrentService) canApprove(torrent *model.Torrent, userID int64, perms model.Permissions) bool {
	if perms.IsStaff() {
		return true
	}
	return perms.CanSelfApprove && torrent.UploaderID == userID
}

// canAccessModerationThread reports whether a user may read/post in a torrent's
// moderation thread: staff or the uploader.
func (s *TorrentService) canAccessModerationThread(torrent *model.Torrent, userID int64, perms model.Permissions) bool {
	return perms.IsStaff() || torrent.UploaderID == userID
}

// ListModerationMessages returns a torrent's moderation thread. Only staff and the
// uploader may read it.
func (s *TorrentService) ListModerationMessages(ctx context.Context, torrentID, userID int64, perms model.Permissions) ([]model.TorrentModerationMessage, error) {
	if s.moderationMessages == nil {
		return nil, ErrModerationUnavailable
	}
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	if !s.canAccessModerationThread(torrent, userID, perms) {
		return nil, ErrForbidden
	}
	return s.moderationMessages.ListByTorrent(ctx, torrentID)
}

// PostModerationMessage adds a message to a torrent's moderation thread (staff or
// uploader), then publishes an event so the uploader + assigned moderator are
// notified (the actor is skipped).
func (s *TorrentService) PostModerationMessage(ctx context.Context, torrentID, userID int64, perms model.Permissions, body string) (*model.TorrentModerationMessage, error) {
	if s.moderationMessages == nil {
		return nil, ErrModerationUnavailable
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, ErrEmptyMessage
	}
	torrent, err := s.torrents.GetByID(ctx, torrentID)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	if !s.canAccessModerationThread(torrent, userID, perms) {
		return nil, ErrForbidden
	}

	msg := &model.TorrentModerationMessage{
		TorrentID: torrentID,
		AuthorID:  userID,
		Body:      body,
	}
	if err := s.moderationMessages.Create(ctx, msg); err != nil {
		return nil, fmt.Errorf("create moderation message: %w", err)
	}
	msg.AuthorUsername = s.actorFromUserID(ctx, userID).Username

	s.eventBus.Publish(ctx, &event.TorrentModerationMessagePostedEvent{
		Base:                event.NewBase(event.TorrentModerationMsg, s.actorFromUserID(ctx, userID)),
		TorrentID:           torrent.ID,
		TorrentName:         torrent.Name,
		UploaderID:          torrent.UploaderID,
		AssignedModeratorID: torrent.AssignedModeratorID,
	})

	return msg, nil
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

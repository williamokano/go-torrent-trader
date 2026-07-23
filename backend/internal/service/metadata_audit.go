package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/metadata"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// MetadataAuditTorrentRepo is the narrow slice of torrent persistence the
// metadata audit needs, so tests don't have to reimplement the whole repository.
type MetadataAuditTorrentRepo interface {
	ListMissingRequiredMetadata(ctx context.Context, categoryID int64, requiredKeys []string, uploaderID *int64) ([]model.Torrent, error)
}

// MissingField identifies a required metadata field a torrent has no value for.
type MissingField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// MetadataIssue is a torrent that no longer satisfies its category's required
// metadata fields — e.g. a field was made required after the torrent was
// uploaded, so the stored values predate it.
type MetadataIssue struct {
	TorrentID     int64          `json:"torrent_id"`
	TorrentName   string         `json:"torrent_name"`
	CategoryID    int64          `json:"category_id"`
	CategoryName  string         `json:"category_name"`
	UploaderID    int64          `json:"uploader_id"`
	UploaderName  string         `json:"uploader_name"`
	Anonymous     bool           `json:"anonymous"`
	MissingFields []MissingField `json:"missing_fields"`
}

// MetadataAuditService reports torrents whose stored metadata is missing fields
// their category now marks required.
type MetadataAuditService struct {
	torrents   MetadataAuditTorrentRepo
	categories repository.CategoryRepository
}

// NewMetadataAuditService creates a MetadataAuditService.
func NewMetadataAuditService(torrents MetadataAuditTorrentRepo, categories repository.CategoryRepository) *MetadataAuditService {
	return &MetadataAuditService{torrents: torrents, categories: categories}
}

// Issues returns torrents missing required metadata. When uploaderID is non-nil
// the report is scoped to that uploader; nil reports across all uploaders.
// It resolves each category's *effective* schema, so a required field inherited
// from an ancestor category is enforced too.
func (s *MetadataAuditService) Issues(ctx context.Context, uploaderID *int64) ([]MetadataIssue, error) {
	cats, err := s.categories.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	issues := make([]MetadataIssue, 0)
	for i := range cats {
		catID := cats[i].ID

		schema, err := ResolveCategorySchema(ctx, s.categories, catID)
		if err != nil {
			return nil, fmt.Errorf("resolve schema for category %d: %w", catID, err)
		}
		var required []metadata.FieldDef
		for _, f := range schema {
			if f.Required {
				required = append(required, f)
			}
		}
		if len(required) == 0 {
			continue
		}

		keys := make([]string, len(required))
		for j, f := range required {
			keys[j] = f.Key
		}

		torrents, err := s.torrents.ListMissingRequiredMetadata(ctx, catID, keys, uploaderID)
		if err != nil {
			return nil, fmt.Errorf("list torrents missing metadata in category %d: %w", catID, err)
		}

		for k := range torrents {
			t := &torrents[k]
			present := presentMetadataKeys(t.Metadata)

			var missing []MissingField
			for _, f := range required {
				if !present[f.Key] {
					missing = append(missing, MissingField{Key: f.Key, Label: f.Label})
				}
			}
			if len(missing) == 0 {
				continue // guarded by the SQL prefilter, but stay defensive
			}

			issues = append(issues, MetadataIssue{
				TorrentID:     t.ID,
				TorrentName:   t.Name,
				CategoryID:    t.CategoryID,
				CategoryName:  t.CategoryName,
				UploaderID:    t.UploaderID,
				UploaderName:  t.UploaderName,
				Anonymous:     t.Anonymous,
				MissingFields: missing,
			})
		}
	}
	return issues, nil
}

// presentMetadataKeys returns the top-level keys stored in a torrent's metadata.
// Values are only ever persisted when non-empty (see metadata.ValidateValues),
// so a present key means a stored value — matching the jsonb_exists prefilter.
func presentMetadataKeys(raw json.RawMessage) map[string]bool {
	present := make(map[string]bool)
	var obj map[string]json.RawMessage
	if len(raw) > 0 && json.Unmarshal(raw, &obj) == nil {
		for k := range obj {
			present[k] = true
		}
	}
	return present
}

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

type stubAuditTorrentRepo struct {
	byCat         map[int64][]model.Torrent
	calledCats    []int64
	gotUploaderID *int64
}

func (s *stubAuditTorrentRepo) ListMissingRequiredMetadata(_ context.Context, categoryID int64, _ []string, uploaderID *int64) ([]model.Torrent, error) {
	s.calledCats = append(s.calledCats, categoryID)
	s.gotUploaderID = uploaderID
	return s.byCat[categoryID], nil
}

func TestMetadataAuditService_Issues(t *testing.T) {
	ctx := context.Background()
	catRepo := newMockCategoryRepo()
	catRepo.categories = []*model.Category{
		{
			ID:   1,
			Name: "Movies",
			MetadataSchema: json.RawMessage(
				`[{"key":"year","label":"Year","type":"number","required":true},{"key":"codec","label":"Codec","type":"text"}]`,
			),
		},
		// No required fields -> should be skipped entirely.
		{
			ID:             2,
			Name:           "Music",
			MetadataSchema: json.RawMessage(`[{"key":"artist","label":"Artist","type":"text"}]`),
		},
	}
	audit := &stubAuditTorrentRepo{
		byCat: map[int64][]model.Torrent{
			1: {{
				ID:           10,
				Name:         "Old Movie",
				CategoryID:   1,
				CategoryName: "Movies",
				UploaderID:   7,
				UploaderName: "bob",
				Metadata:     json.RawMessage(`{"codec":"x264"}`), // has codec, missing year
			}},
		},
	}
	svc := NewMetadataAuditService(audit, catRepo)

	t.Run("reports missing required fields and skips categories without any", func(t *testing.T) {
		issues, err := svc.Issues(ctx, nil)
		if err != nil {
			t.Fatalf("Issues: %v", err)
		}
		if len(issues) != 1 {
			t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
		}
		is := issues[0]
		if is.TorrentID != 10 || is.CategoryName != "Movies" {
			t.Errorf("issue = %+v, want torrent 10 in Movies", is)
		}
		if len(is.MissingFields) != 1 || is.MissingFields[0].Key != "year" {
			t.Errorf("missing = %+v, want [year]", is.MissingFields)
		}
		// Music (id 2) has no required field, so it's never queried.
		if len(audit.calledCats) != 1 || audit.calledCats[0] != 1 {
			t.Errorf("audit queried %v, want only category 1", audit.calledCats)
		}
	})

	t.Run("passes the uploader scope through to the repository", func(t *testing.T) {
		uid := int64(7)
		if _, err := svc.Issues(ctx, &uid); err != nil {
			t.Fatalf("Issues: %v", err)
		}
		if audit.gotUploaderID == nil || *audit.gotUploaderID != 7 {
			t.Errorf("uploaderID passed = %v, want 7", audit.gotUploaderID)
		}
	})
}

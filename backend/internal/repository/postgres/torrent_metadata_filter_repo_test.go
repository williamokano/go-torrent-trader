package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// setMetadata assigns a torrent's JSONB metadata via the same Update the
// production code uses, so the filter test exercises real persisted values.
func setMetadata(t *testing.T, repo *TorrentRepo, tor *model.Torrent, meta string) {
	t.Helper()
	tor.Metadata = json.RawMessage(meta)
	if err := repo.Update(context.Background(), tor); err != nil {
		t.Fatalf("Update metadata: %v", err)
	}
}

func idSet(torrents []model.Torrent) map[int64]bool {
	out := make(map[int64]bool, len(torrents))
	for _, tor := range torrents {
		out[tor.ID] = true
	}
	return out
}

func TestTorrentRepoMetadataFilters(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	u := newUser(t, db)

	t1 := newTorrent(t, db, u.ID)
	setMetadata(t, repo, t1, `{"year":2024,"codec":"x265","hdr":true,"genres":["Action","Drama"]}`)
	t2 := newTorrent(t, db, u.ID)
	setMetadata(t, repo, t2, `{"year":2005,"codec":"x264","genres":["Drama"]}`)
	t3 := newTorrent(t, db, u.ID)
	setMetadata(t, repo, t3, `{"year":2010,"codec":"x265"}`)
	// A drifted row storing year as text must not abort the ::numeric cast in
	// range queries — the jsonb_typeof guard should simply exclude it.
	t4 := newTorrent(t, db, u.ID)
	setMetadata(t, repo, t4, `{"year":"unknown","codec":"x265"}`)

	tests := []struct {
		name    string
		filters []repository.MetadataFilter
		want    []int64
	}{
		{
			name:    "number equality",
			filters: []repository.MetadataFilter{{Key: "year", Op: repository.MetaFilterEq, Value: float64(2024)}},
			want:    []int64{t1.ID},
		},
		{
			name:    "select equality matches multiple",
			filters: []repository.MetadataFilter{{Key: "codec", Op: repository.MetaFilterEq, Value: "x265"}},
			want:    []int64{t1.ID, t3.ID, t4.ID},
		},
		{
			name:    "boolean equality",
			filters: []repository.MetadataFilter{{Key: "hdr", Op: repository.MetaFilterEq, Value: true}},
			want:    []int64{t1.ID},
		},
		{
			name:    "multiselect containment",
			filters: []repository.MetadataFilter{{Key: "genres", Op: repository.MetaFilterEq, Value: []string{"Action"}}},
			want:    []int64{t1.ID},
		},
		{
			name: "numeric range",
			filters: []repository.MetadataFilter{
				{Key: "year", Op: repository.MetaFilterGte, Value: float64(2008)},
				{Key: "year", Op: repository.MetaFilterLte, Value: float64(2024)},
			},
			want: []int64{t1.ID, t3.ID},
		},
		{
			name: "equality and range combined",
			filters: []repository.MetadataFilter{
				{Key: "codec", Op: repository.MetaFilterEq, Value: "x265"},
				{Key: "year", Op: repository.MetaFilterGte, Value: float64(2011)},
			},
			want: []int64{t1.ID},
		},
		{
			name:    "range does not choke on non-numeric stored value",
			filters: []repository.MetadataFilter{{Key: "year", Op: repository.MetaFilterGte, Value: float64(2000)}},
			want:    []int64{t1.ID, t2.ID, t3.ID}, // t4's text year is excluded, not an error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, total, err := repo.List(ctx, repository.ListTorrentsOptions{MetadataFilters: tt.filters, PerPage: 100})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if int(total) != len(tt.want) {
				t.Errorf("total = %d, want %d", total, len(tt.want))
			}
			set := idSet(got)
			if len(set) != len(tt.want) {
				t.Fatalf("got %d rows, want %d: %v", len(set), len(tt.want), set)
			}
			for _, id := range tt.want {
				if !set[id] {
					t.Errorf("missing expected torrent %d in %v", id, set)
				}
			}
		})
	}
}

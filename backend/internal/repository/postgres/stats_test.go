package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// service.StatsService runs raw SQL that its own unit tests only see through
// sqlmock, which matches any SELECT. This runs it against the real schema, so a
// renamed column or a mistake in the single-pass peers count fails here.
func TestStatsService_AgainstTheRealSchema(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	alice := newUser(t, db)
	bob := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET last_access = NOW() WHERE id = $1`, alice.ID); err != nil {
		t.Fatalf("marking alice online: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET last_access = NOW() - INTERVAL '1 hour' WHERE id = $1`, bob.ID); err != nil {
		t.Fatalf("marking bob offline: %v", err)
	}
	visible := newTorrent(t, db, alice.ID)
	hidden := newTorrent(t, db, alice.ID)
	if _, err := db.ExecContext(ctx, `UPDATE torrents SET visible = false WHERE id = $1`, hidden.ID); err != nil {
		t.Fatalf("hiding torrent: %v", err)
	}

	for i, p := range []struct {
		user   int64
		seeder bool
	}{{alice.ID, true}, {alice.ID, true}, {bob.ID, false}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO peers
			(torrent_id, user_id, peer_id, ip, port, seeder) VALUES ($1, $2, $3, '10.0.0.1', 6881, $4)`,
			visible.ID, p.user, []byte{byte(i), 'p', 'e', 'e', 'r'}, p.seeder); err != nil {
			t.Fatalf("insert peer: %v", err)
		}
	}

	got, err := service.NewStatsService(db).Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := service.SiteStats{Users: 2, Torrents: 1, Peers: 3, Seeders: 2, Leechers: 1, OnlineUsers: 1}
	if *got != want {
		t.Fatalf("stats = %+v, want %+v", *got, want)
	}
}

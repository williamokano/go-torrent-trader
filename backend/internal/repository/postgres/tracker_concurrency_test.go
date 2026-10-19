package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// lookupBarrier holds every announce just after it has looked its peer up,
// until all of them have: the precise interleaving in which they all read
// the same stored counters, which goroutine scheduling alone rarely produces.
type lookupBarrier struct {
	repository.PeerRepository
	arrived sync.WaitGroup
	release chan struct{}
	armed   bool
}

func (b *lookupBarrier) GetByTorrentUserAndPeerID(ctx context.Context, torrentID, userID int64, peerID []byte) (*model.Peer, error) {
	p, err := b.PeerRepository.GetByTorrentUserAndPeerID(ctx, torrentID, userID, peerID)
	if b.armed {
		b.arrived.Done()
		<-b.release
	}
	return p, err
}

// A client that fires several announces at once — a retry storm, or a
// deliberate burst — used to have every one of them measured against the
// same stored counters, so the same bytes were credited once per request.
// Against a real Postgres, twenty concurrent announces of one 1,000-byte
// increase must credit exactly 1,000 bytes.
func TestConcurrentAnnouncesCreditTrafficOnce(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	u := newUser(t, db)
	passkey := uniq("pk")
	if _, err := db.Exec(`UPDATE users SET passkey = $1 WHERE id = $2`, passkey, u.ID); err != nil {
		t.Fatalf("passkey: %v", err)
	}
	tor := newTorrent(t, db, u.ID)
	peers := &lookupBarrier{PeerRepository: NewPeerRepo(db), release: make(chan struct{})}
	tracker := service.NewTrackerService(NewUserRepo(db), NewTorrentRepo(db), peers)

	peerID := []byte("-QB0000-concurrency1")
	announce := func(uploaded, left int64, ev service.AnnounceEvent) error {
		_, err := tracker.Announce(ctx, service.AnnounceRequest{
			Passkey:  passkey,
			InfoHash: tor.InfoHash,
			PeerID:   peerID,
			IP:       "10.0.0.1",
			Port:     6881,
			Uploaded: uploaded,
			Left:     left,
			Event:    ev,
		})
		return err
	}
	if err := announce(5000, 100, service.EventStarted); err != nil { // a leecher
		t.Fatalf("started: %v", err)
	}

	const n = 20
	peers.armed = true
	peers.arrived.Add(n)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := announce(6000, 0, service.EventEmpty); err != nil { // now a seeder
				errs <- err
			}
		}()
	}
	peers.arrived.Wait() // every announce has read the same counters
	close(peers.release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("announce: %v", err)
	}

	got, err := NewUserRepo(db).GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Uploaded != 1000 {
		t.Errorf("uploaded = %d after 20 concurrent announces of +1000, want exactly 1000", got.Uploaded)
	}

	// The leecher became a seeder once, however many announces said so.
	swarm, err := NewTorrentRepo(db).GetByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("torrent: %v", err)
	}
	if swarm.Seeders != 1 || swarm.Leechers != 0 {
		t.Errorf("seeders=%d leechers=%d, want 1/0", swarm.Seeders, swarm.Leechers)
	}
}

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// TestTorrentRepoWithTxRollbackDiscardsWrites proves WithTx actually binds the
// repo to the transaction rather than quietly falling back to the pool: if it
// did the latter, a rollback would not undo the write.
func TestTorrentRepoWithTxRollbackDiscardsWrites(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	uploader := newUser(t, db)
	catID := newCategoryID(t, db)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	txRepo := repo.WithTx(tx)
	tor := &model.Torrent{
		Name:       uniq("Rollback"),
		InfoHash:   infoHash(uniq("rbhash")),
		Size:       2048,
		CategoryID: catID,
		UploaderID: uploader.ID,
		Visible:    true,
		FileCount:  1,
	}
	if err := txRepo.Create(ctx, tor); err != nil {
		_ = tx.Rollback()
		t.Fatalf("Create in tx: %v", err)
	}

	// Inside the transaction the row is visible to the tx-bound repo.
	if _, err := txRepo.GetByID(ctx, tor.ID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("GetByID inside tx: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// After rollback the pool-bound repo must not see it.
	if _, err := repo.GetByID(ctx, tor.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetByID after rollback: err = %v, want sql.ErrNoRows — the write was not transactional", err)
	}
}

// TestTorrentRepoWithTxCommitPersistsWrites is the other half: a committed
// transaction's writes, including counter updates applied through the tx-bound
// repo, must be durable.
func TestTorrentRepoWithTxCommitPersistsWrites(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	uploader := newUser(t, db)
	catID := newCategoryID(t, db)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	txRepo := repo.WithTx(tx)
	tor := &model.Torrent{
		Name:       uniq("Committed"),
		InfoHash:   infoHash(uniq("cmhash")),
		Size:       4096,
		CategoryID: catID,
		UploaderID: uploader.ID,
		Visible:    true,
		FileCount:  1,
	}
	if err := txRepo.Create(ctx, tor); err != nil {
		_ = tx.Rollback()
		t.Fatalf("Create in tx: %v", err)
	}
	if err := txRepo.IncrementSeeders(ctx, tor.ID, 3); err != nil {
		_ = tx.Rollback()
		t.Fatalf("IncrementSeeders in tx: %v", err)
	}
	if err := txRepo.IncrementTimesCompleted(ctx, tor.ID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("IncrementTimesCompleted in tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetByID after commit: %v", err)
	}
	if got.Seeders != 3 {
		t.Errorf("Seeders = %d, want 3", got.Seeders)
	}
	if got.TimesCompleted != 1 {
		t.Errorf("TimesCompleted = %d, want 1", got.TimesCompleted)
	}
}

// TestTorrentRepoWithTxLeavesOriginalOnPool guards against WithTx mutating the
// receiver: the original repo must still be usable on the pool afterwards,
// including once the transaction is finished.
func TestTorrentRepoWithTxLeavesOriginalOnPool(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	uploader := newUser(t, db)
	tor := newTorrent(t, db, uploader.ID)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	_ = repo.WithTx(tx)
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// If WithTx had rebound the receiver, this would fail with ErrTxDone.
	if _, err := repo.GetByID(ctx, tor.ID); err != nil {
		t.Errorf("GetByID on the original repo after WithTx: %v — WithTx mutated the receiver", err)
	}
}

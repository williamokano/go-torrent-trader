package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func newStatsCache(t *testing.T) (*service.StatsCache, sqlmock.Sqlmock, *miniredis.Miniredis) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cache := service.NewStatsCache(db, rdb, 30*time.Second)
	return cache, mock, mr
}

func TestStatsCacheGetQueriesDBOnMiss(t *testing.T) {
	cache, mock, _ := newStatsCache(t)

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers", "seeders", "leechers", "online_users"}).
			AddRow(10, 20, 30, 15, 15, 3))

	stats, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.Users != 10 {
		t.Errorf("expected users=10, got %d", stats.Users)
	}
	if stats.Torrents != 20 {
		t.Errorf("expected torrents=20, got %d", stats.Torrents)
	}
	if stats.Peers != 30 {
		t.Errorf("expected peers=30, got %d", stats.Peers)
	}
	if stats.Seeders != 15 {
		t.Errorf("expected seeders=15, got %d", stats.Seeders)
	}
	if stats.Leechers != 15 {
		t.Errorf("expected leechers=15, got %d", stats.Leechers)
	}
	if stats.OnlineUsers != 3 {
		t.Errorf("expected online_users=3, got %d", stats.OnlineUsers)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestStatsCacheGetReturnsCachedOnHit(t *testing.T) {
	cache, mock, _ := newStatsCache(t)

	// First call hits DB.
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers", "seeders", "leechers", "online_users"}).
			AddRow(10, 20, 30, 15, 15, 3))

	stats1, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get: unexpected error: %v", err)
	}

	// Second call should NOT hit DB (no expectation set).
	stats2, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get: unexpected error: %v", err)
	}

	if stats2.Users != stats1.Users {
		t.Errorf("expected cached users=%d, got %d", stats1.Users, stats2.Users)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestStatsCacheGetQueriesDBAfterTTLExpiry(t *testing.T) {
	cache, mock, mr := newStatsCache(t)

	// First call.
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers", "seeders", "leechers", "online_users"}).
			AddRow(10, 20, 30, 15, 15, 3))

	_, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get: unexpected error: %v", err)
	}

	// Fast-forward past TTL.
	mr.FastForward(31 * time.Second)

	// Second call should hit DB again.
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers", "seeders", "leechers", "online_users"}).
			AddRow(99, 88, 77, 66, 11, 5))

	stats, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get: unexpected error: %v", err)
	}

	if stats.Users != 99 {
		t.Errorf("expected users=99 after TTL, got %d", stats.Users)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestStatsCacheWarm(t *testing.T) {
	cache, mock, _ := newStatsCache(t)

	// Warm populates the cache.
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers", "seeders", "leechers", "online_users"}).
			AddRow(5, 10, 15, 8, 7, 2))

	if err := cache.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: unexpected error: %v", err)
	}

	// Get should use cache (no DB expectation).
	stats, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after Warm: unexpected error: %v", err)
	}

	if stats.Users != 5 {
		t.Errorf("expected users=5, got %d", stats.Users)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestStatsCacheGetReturnsErrorOnDBFailure(t *testing.T) {
	cache, mock, _ := newStatsCache(t)

	mock.ExpectQuery(`SELECT`).
		WillReturnError(context.DeadlineExceeded)

	_, err := cache.Get(context.Background())
	if err == nil {
		t.Fatal("expected error on DB failure")
	}

	if mockErr := mock.ExpectationsWereMet(); mockErr != nil {
		t.Errorf("unmet sqlmock expectations: %v", mockErr)
	}
}

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func newTestStatsService(t *testing.T) (*StatsService, sqlmock.Sqlmock, *time.Time) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := NewStatsService(db)
	svc.now = func() time.Time { return clock }
	return svc, mock, &clock
}

func expectStatsQuery(mock sqlmock.Sqlmock, users, peers, seeders int64) {
	mock.ExpectQuery(`SELECT`).WillReturnRows(
		sqlmock.NewRows([]string{"users", "torrents", "total", "seeders", "online"}).
			AddRow(users, 500, peers, seeders, 7))
}

func TestStatsServiceReadsTheDatabase(t *testing.T) {
	svc, mock, _ := newTestStatsService(t)
	expectStatsQuery(mock, 100, 42, 30)

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := SiteStats{Users: 100, Torrents: 500, Peers: 42, Seeders: 30, Leechers: 12, OnlineUsers: 7}
	if *got != want {
		t.Fatalf("stats = %+v, want %+v (leechers derived as peers - seeders)", *got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Every member's header asks for the stats; inside the TTL that must not be a
// query each.
func TestStatsServiceServesAFreshReadingWithoutQuerying(t *testing.T) {
	svc, mock, clock := newTestStatsService(t)
	expectStatsQuery(mock, 100, 42, 30)

	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	*clock = clock.Add(statsTTL - time.Second)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got.Users != 100 {
		t.Fatalf("second reading = %+v, want the cached one", got)
	}
	// sqlmock fails on any query it was not told to expect.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatsServiceRefreshesAnExpiredReading(t *testing.T) {
	svc, mock, clock := newTestStatsService(t)
	expectStatsQuery(mock, 100, 42, 30)
	expectStatsQuery(mock, 101, 42, 30)

	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	*clock = clock.Add(statsTTL)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after expiry: %v", err)
	}
	if got.Users != 101 {
		t.Fatalf("reading after expiry = %+v, want a fresh one", got)
	}
}

// A failed read with nothing to fall back on is reported, and not remembered:
// the next call tries again.
func TestStatsServiceDoesNotCacheAFailure(t *testing.T) {
	svc, mock, _ := newTestStatsService(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(errors.New("db down"))
	expectStatsQuery(mock, 100, 42, 30)

	if _, err := svc.Get(context.Background()); err == nil {
		t.Fatal("expected the query error")
	}
	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatalf("Get after a failure: %v", err)
	}
}

// Once there is a reading, a failed refresh serves it rather than blanking
// every member's header for as long as the database is struggling.
func TestStatsServiceServesTheLastReadingWhenARefreshFails(t *testing.T) {
	svc, mock, clock := newTestStatsService(t)
	expectStatsQuery(mock, 100, 42, 30)
	mock.ExpectQuery(`SELECT`).WillReturnError(errors.New("db down"))

	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	*clock = clock.Add(statsTTL)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get with a failing refresh: %v", err)
	}
	if got.Users != 100 {
		t.Fatalf("got %+v, want the last good reading", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Many requests arriving on an expired reading share one query.
func TestStatsServiceRefreshesOnceForConcurrentCallers(t *testing.T) {
	svc, mock, _ := newTestStatsService(t)
	mock.ExpectQuery(`SELECT`).WillDelayFor(50 * time.Millisecond).WillReturnRows(
		sqlmock.NewRows([]string{"users", "torrents", "total", "seeders", "online"}).AddRow(100, 500, 42, 30, 7))

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Get(context.Background()); err != nil {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	wg.Wait()
	// sqlmock fails any query beyond the one it expects.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A caller whose request goes away stops waiting — and does not take the
// refresh down with it for everyone else.
func TestStatsServiceCallerCanGiveUpWithoutCancellingTheRefresh(t *testing.T) {
	svc, mock, _ := newTestStatsService(t)
	mock.ExpectQuery(`SELECT`).WillDelayFor(100 * time.Millisecond).WillReturnRows(
		sqlmock.NewRows([]string{"users", "torrents", "total", "seeders", "online"}).AddRow(100, 500, 42, 30, 7))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := svc.Get(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("impatient caller got %v, want its own deadline", err)
	}

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("patient caller: %v", err)
	}
	if got.Users != 100 {
		t.Fatalf("patient caller got %+v, want the refresh the first caller started", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Callers get a copy: one handler adjusting its stats must not change what
// the next caller reads.
func TestStatsServiceReturnsCopies(t *testing.T) {
	svc, mock, _ := newTestStatsService(t)
	expectStatsQuery(mock, 100, 42, 30)

	first, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	first.Users = -1
	second, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if second.Users != 100 {
		t.Fatalf("a caller's change leaked into the cache: %+v", second)
	}
}

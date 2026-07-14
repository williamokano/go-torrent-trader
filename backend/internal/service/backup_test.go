package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
)

const testDatabaseURL = "postgres://tt_user:s3cr3t-pw@db.internal:5433/torrenttrader?sslmode=require"

// fakePgDump writes a shell script that stands in for pg_dump. It records the
// argv and environment it was invoked with (next to the output file) and writes
// the given content to the --file target.
func fakePgDump(t *testing.T, content string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake pg_dump script requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "pg_dump")
	script := `#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--file" ]; then out="$a"; fi
  prev="$a"
done
dir=$(dirname "$out")
printf '%s\n' "$@" > "$dir/last-args"
env > "$dir/last-env"
printf '%s' '` + content + `' > "$out"
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("writing fake pg_dump: %v", err)
	}
	return path
}

// failingPgDump writes a script that fails with output on stderr.
func failingPgDump(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake pg_dump script requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "pg_dump")
	script := "#!/bin/sh\necho 'pg_dump: error: connection failed' >&2\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("writing fake pg_dump: %v", err)
	}
	return path
}

func newTestBackupService(t *testing.T, pgDumpPath string, retention int, bus event.Bus) *BackupService {
	t.Helper()
	svc, err := NewBackupService(BackupServiceConfig{
		DatabaseURL: testDatabaseURL,
		Dir:         t.TempDir(),
		PgDumpPath:  pgDumpPath,
		Timeout:     10 * time.Second,
		Retention:   retention,
	}, bus)
	if err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}
	return svc
}

// --- name validation / path traversal ---------------------------------------

func TestValidateBackupNameRejectsTraversalAndJunk(t *testing.T) {
	bad := []string{
		"",
		"../../etc/passwd",
		"../backup-20260714T031500Z-a1b2c3d4.dump",
		"..%2F..%2Fetc%2Fpasswd",
		"/etc/passwd",
		"subdir/backup-20260714T031500Z-a1b2c3d4.dump",
		`..\..\windows\system32`,
		"backup-20260714T031500Z-a1b2c3d4.dump/../../etc/passwd",
		"backup-20260714T031500Z-a1b2c3d4.dump\x00.png",
		"backup-20260714T031500Z-a1b2c3d4.sql",
		"backup-2026-07-14-a1b2c3d4.dump",
		"backup-20260714T031500Z-ZZZZZZZZ.dump",
		"random.dump",
		".",
		"..",
		strings.Repeat("a", 200) + ".dump",
	}
	for _, name := range bad {
		if err := ValidateBackupName(name); !errors.Is(err, ErrInvalidBackupName) {
			t.Errorf("ValidateBackupName(%q) = %v, want ErrInvalidBackupName", name, err)
		}
	}

	if err := ValidateBackupName("backup-20260714T031500Z-a1b2c3d4.dump"); err != nil {
		t.Errorf("ValidateBackupName(valid) = %v, want nil", err)
	}
}

func TestOpenAndDeleteRejectPathTraversal(t *testing.T) {
	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 0, nil)

	// A file that a traversal attempt would target if the guard were missing.
	outside := filepath.Join(filepath.Dir(svc.Dir()), "passwd")
	if err := os.WriteFile(outside, []byte("root:x:0:0"), 0o600); err != nil {
		t.Fatalf("writing outside file: %v", err)
	}

	for _, name := range []string{"../../etc/passwd", "../passwd", "/etc/passwd", "..", "subdir/x.dump"} {
		if _, _, err := svc.Open(name); !errors.Is(err, ErrInvalidBackupName) {
			t.Errorf("Open(%q) = %v, want ErrInvalidBackupName", name, err)
		}
		if err := svc.Delete(context.Background(), name, event.Actor{ID: 1}); !errors.Is(err, ErrInvalidBackupName) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidBackupName", name, err)
		}
	}

	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("traversal deleted a file outside the backup dir: %v", err)
	}
}

func TestOpenRejectsSymlinkInsideBackupDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 0, nil)

	secret := filepath.Join(filepath.Dir(svc.Dir()), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatalf("writing secret: %v", err)
	}
	// A symlink with a perfectly valid backup name pointing outside the dir.
	link := filepath.Join(svc.Dir(), "backup-20260714T031500Z-deadbeef.dump")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	if _, _, err := svc.Open("backup-20260714T031500Z-deadbeef.dump"); !errors.Is(err, ErrInvalidBackupName) {
		t.Errorf("Open(symlink) = %v, want ErrInvalidBackupName", err)
	}
	if err := svc.Delete(context.Background(), "backup-20260714T031500Z-deadbeef.dump", event.Actor{ID: 1}); !errors.Is(err, ErrInvalidBackupName) {
		t.Errorf("Delete(symlink) = %v, want ErrInvalidBackupName", err)
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("symlink target was deleted: %v", err)
	}

	// A symlink is not a backup, so it must not show up in the listing either.
	backups, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected symlink to be excluded from List, got %d entries", len(backups))
	}
}

// --- create / list / open / delete ------------------------------------------

func TestCreateBackupWritesFileAndPublishesEvent(t *testing.T) {
	bus := event.NewInMemoryBus()
	var published []event.Event
	bus.Subscribe(event.BackupCreated, func(_ context.Context, evt event.Event) error {
		published = append(published, evt)
		return nil
	})

	svc := newTestBackupService(t, fakePgDump(t, "PGDUMP-CONTENT"), 0, bus)

	backup, err := svc.Create(context.Background(), event.Actor{ID: 7, Username: "admin"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ValidateBackupName(backup.Name); err != nil {
		t.Errorf("generated name %q is not valid: %v", backup.Name, err)
	}
	if backup.Size != int64(len("PGDUMP-CONTENT")) {
		t.Errorf("expected size %d, got %d", len("PGDUMP-CONTENT"), backup.Size)
	}
	if backup.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// No .partial file left behind.
	entries, err := os.ReadDir(svc.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), partialSuffix) {
			t.Errorf("partial file left behind: %s", e.Name())
		}
	}

	if len(published) != 1 {
		t.Fatalf("expected 1 backup_created event, got %d", len(published))
	}
	evt, ok := published[0].(*event.BackupCreatedEvent)
	if !ok {
		t.Fatalf("expected *event.BackupCreatedEvent, got %T", published[0])
	}
	if evt.Name != backup.Name || evt.Actor.Username != "admin" {
		t.Errorf("unexpected event: %+v", evt)
	}
}

func TestCreateBackupPassesCredentialsViaEnvNotArgv(t *testing.T) {
	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 0, nil)

	if _, err := svc.Create(context.Background(), event.Actor{ID: 1}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	args, err := os.ReadFile(filepath.Join(svc.Dir(), "last-args")) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("reading recorded args: %v", err)
	}
	env, err := os.ReadFile(filepath.Join(svc.Dir(), "last-env")) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("reading recorded env: %v", err)
	}

	if strings.Contains(string(args), "s3cr3t-pw") {
		t.Errorf("password leaked into pg_dump argv:\n%s", args)
	}
	if strings.Contains(string(args), "postgres://") {
		t.Errorf("connection URL leaked into pg_dump argv:\n%s", args)
	}
	for _, want := range []string{
		"PGPASSWORD=s3cr3t-pw",
		"PGUSER=tt_user",
		"PGHOST=db.internal",
		"PGPORT=5433",
		"PGDATABASE=torrenttrader",
		"PGSSLMODE=require",
	} {
		if !strings.Contains(string(env), want) {
			t.Errorf("expected %q in pg_dump env:\n%s", want, env)
		}
	}
}

func TestCreateBackupFailureCleansUpPartial(t *testing.T) {
	svc := newTestBackupService(t, failingPgDump(t), 0, nil)

	if _, err := svc.Create(context.Background(), event.Actor{ID: 1}); err == nil {
		t.Fatal("expected Create to fail")
	}

	entries, err := os.ReadDir(svc.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected backup dir to be empty after failure, got %d entries", len(entries))
	}

	backups, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected no backups after failure, got %d", len(backups))
	}
}

func TestCreateBackupRejectsConcurrentRuns(t *testing.T) {
	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 0, nil)

	// Simulate a dump already in flight.
	svc.running.Store(true)
	_, err := svc.Create(context.Background(), event.Actor{ID: 1})
	if !errors.Is(err, ErrBackupInProgress) {
		t.Fatalf("expected ErrBackupInProgress, got %v", err)
	}
	svc.running.Store(false)

	// Once it finishes, a new backup can start.
	if _, err := svc.Create(context.Background(), event.Actor{ID: 1}); err != nil {
		t.Fatalf("Create after release: %v", err)
	}
}

func TestCreateBackupTimesOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake pg_dump script requires a POSIX shell")
	}
	slow := filepath.Join(t.TempDir(), "pg_dump")
	// exec replaces the shell so the kill on timeout terminates sleep directly.
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nexec sleep 5\n"), 0o700); err != nil {
		t.Fatalf("writing slow pg_dump: %v", err)
	}

	svc, err := NewBackupService(BackupServiceConfig{
		DatabaseURL: testDatabaseURL,
		Dir:         t.TempDir(),
		PgDumpPath:  slow,
		Timeout:     50 * time.Millisecond,
	}, nil)
	if err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}

	if _, err := svc.Create(context.Background(), event.Actor{ID: 1}); err == nil {
		t.Fatal("expected timeout error")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got %v", err)
	}
}

func TestCreateBackupIgnoresCallerCancellation(t *testing.T) {
	bus := event.NewInMemoryBus()
	var listenerCtxErr error
	published := 0
	bus.Subscribe(event.BackupCreated, func(ctx context.Context, _ event.Event) error {
		published++
		listenerCtxErr = ctx.Err()
		return nil
	})

	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 0, bus)

	// An admin closing the browser tab must not abort a running backup.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	backup, err := svc.Create(ctx, event.Actor{ID: 1})
	if err != nil {
		t.Fatalf("Create with cancelled ctx: %v", err)
	}
	if backup.Size == 0 {
		t.Error("expected backup to be written despite cancelled caller context")
	}
	// ...and the activity-log listener must still get a usable context.
	if published != 1 {
		t.Fatalf("expected 1 backup_created event, got %d", published)
	}
	if listenerCtxErr != nil {
		t.Errorf("expected listener to receive a live context, got err %v", listenerCtxErr)
	}
}

func TestListOpenDeleteRoundTrip(t *testing.T) {
	bus := event.NewInMemoryBus()
	var deleted []event.Event
	bus.Subscribe(event.BackupDeleted, func(_ context.Context, evt event.Event) error {
		deleted = append(deleted, evt)
		return nil
	})

	svc := newTestBackupService(t, fakePgDump(t, "CONTENT"), 0, bus)
	ctx := context.Background()

	created, err := svc.Create(ctx, event.Actor{ID: 1, Username: "admin"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	backups, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 || backups[0].Name != created.Name {
		t.Fatalf("expected 1 backup named %s, got %+v", created.Name, backups)
	}

	rc, size, err := svc.Open(created.Name)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	_ = rc.Close()
	if string(data) != "CONTENT" {
		t.Errorf("expected backup content %q, got %q", "CONTENT", data)
	}
	if size != int64(len("CONTENT")) {
		t.Errorf("expected size %d, got %d", len("CONTENT"), size)
	}

	if err := svc.Delete(ctx, created.Name, event.Actor{ID: 1, Username: "admin"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(deleted) != 1 {
		t.Errorf("expected 1 backup_deleted event, got %d", len(deleted))
	}

	if _, _, err := svc.Open(created.Name); !errors.Is(err, ErrBackupNotFound) {
		t.Errorf("Open after delete = %v, want ErrBackupNotFound", err)
	}
	if err := svc.Delete(ctx, created.Name, event.Actor{ID: 1}); !errors.Is(err, ErrBackupNotFound) {
		t.Errorf("Delete twice = %v, want ErrBackupNotFound", err)
	}
}

func TestListSortsNewestFirstAndIgnoresStrays(t *testing.T) {
	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 0, nil)

	older := writeBackupFile(t, svc.Dir(), "backup-20260101T010000Z-aaaaaaaa.dump", time.Now().Add(-2*time.Hour))
	newer := writeBackupFile(t, svc.Dir(), "backup-20260102T010000Z-bbbbbbbb.dump", time.Now().Add(-1*time.Hour))
	writeBackupFile(t, svc.Dir(), "notes.txt", time.Now())
	writeBackupFile(t, svc.Dir(), "backup-20260103T010000Z-cccccccc.dump.partial", time.Now())

	backups, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups (strays ignored), got %d: %+v", len(backups), backups)
	}
	if backups[0].Name != newer || backups[1].Name != older {
		t.Errorf("expected newest first, got %s then %s", backups[0].Name, backups[1].Name)
	}
}

func TestListReturnsEmptyWhenDirMissing(t *testing.T) {
	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 0, nil)
	if err := os.RemoveAll(svc.Dir()); err != nil {
		t.Fatalf("removing dir: %v", err)
	}

	backups, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected empty list, got %d", len(backups))
	}
}

func TestCreatePrunesBeyondRetention(t *testing.T) {
	svc := newTestBackupService(t, fakePgDump(t, "DUMP"), 2, nil)

	oldest := writeBackupFile(t, svc.Dir(), "backup-20260101T010000Z-aaaaaaaa.dump", time.Now().Add(-3*time.Hour))
	middle := writeBackupFile(t, svc.Dir(), "backup-20260102T010000Z-bbbbbbbb.dump", time.Now().Add(-2*time.Hour))
	newest := writeBackupFile(t, svc.Dir(), "backup-20260103T010000Z-cccccccc.dump", time.Now().Add(-1*time.Hour))

	fresh, err := svc.Create(context.Background(), event.Actor{ID: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	backups, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected retention to keep 2 backups, got %d: %+v", len(backups), backups)
	}
	names := map[string]bool{backups[0].Name: true, backups[1].Name: true}
	if !names[fresh.Name] || !names[newest] {
		t.Errorf("expected the 2 newest backups to survive, got %v", names)
	}
	if names[oldest] || names[middle] {
		t.Errorf("expected the oldest backups to be pruned, got %v", names)
	}
}

func TestNewBackupServiceClearsStalePartials(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "backup-20260101T010000Z-aaaaaaaa.dump.partial")
	if err := os.WriteFile(stale, []byte("half a dump"), 0o600); err != nil {
		t.Fatalf("writing stale partial: %v", err)
	}

	if _, err := NewBackupService(BackupServiceConfig{
		DatabaseURL: testDatabaseURL,
		Dir:         dir,
		PgDumpPath:  "pg_dump",
		Timeout:     time.Minute,
	}, nil); err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}

	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected stale partial to be removed, stat err = %v", err)
	}
}

func TestNewBackupServiceCreatesDirAndDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "backups")

	svc, err := NewBackupService(BackupServiceConfig{
		DatabaseURL: testDatabaseURL,
		Dir:         dir,
	}, nil)
	if err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("expected backup dir to be created: %v", err)
	}
	if svc.pgDumpPath != "pg_dump" {
		t.Errorf("expected default pg_dump path, got %q", svc.pgDumpPath)
	}
	if svc.timeout != 30*time.Minute {
		t.Errorf("expected default timeout 30m, got %s", svc.timeout)
	}
	if !filepath.IsAbs(svc.Dir()) {
		t.Errorf("expected absolute backup dir, got %q", svc.Dir())
	}
}

// --- connection env ---------------------------------------------------------

func TestPgEnvFromURL(t *testing.T) {
	env, err := pgEnvFromURL("postgres://user:pass@localhost:5432/tt?sslmode=disable")
	if err != nil {
		t.Fatalf("pgEnvFromURL: %v", err)
	}
	joined := strings.Join(env, "\n")
	for _, want := range []string{"PGHOST=localhost", "PGPORT=5432", "PGDATABASE=tt", "PGUSER=user", "PGPASSWORD=pass", "PGSSLMODE=disable"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in env, got %v", want, env)
		}
	}
}

func TestPgEnvFromURLDefaults(t *testing.T) {
	env, err := pgEnvFromURL("postgresql:///tt")
	if err != nil {
		t.Fatalf("pgEnvFromURL: %v", err)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "PGHOST=localhost") || !strings.Contains(joined, "PGPORT=5432") {
		t.Errorf("expected host/port defaults, got %v", env)
	}
	if strings.Contains(joined, "PGPASSWORD=") {
		t.Errorf("expected no PGPASSWORD when the URL has none, got %v", env)
	}
}

func TestPgEnvFromURLErrors(t *testing.T) {
	for _, raw := range []string{
		"",
		"mysql://user:pass@localhost:3306/tt",
		"postgres://user:pass@localhost:5432/",
		"://nope",
	} {
		if _, err := pgEnvFromURL(raw); err == nil {
			t.Errorf("pgEnvFromURL(%q) = nil error, want error", raw)
		}
	}
}

func TestNewBackupServiceRejectsBadDatabaseURL(t *testing.T) {
	if _, err := NewBackupService(BackupServiceConfig{
		DatabaseURL: "mysql://user:pass@localhost/tt",
		Dir:         t.TempDir(),
	}, nil); err == nil {
		t.Fatal("expected error for non-postgres database URL")
	}
}

func TestTruncateForError(t *testing.T) {
	if got := truncateForError("  boom  "); got != "boom" {
		t.Errorf("expected trimmed message, got %q", got)
	}
	long := strings.Repeat("x", 600)
	got := truncateForError(long)
	if len(got) != 503 || !strings.HasSuffix(got, "...") {
		t.Errorf("expected truncated message of 503 chars, got %d", len(got))
	}
}

// writeBackupFile creates a file in dir with the given name and modification time.
func writeBackupFile(t *testing.T, dir, name string, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
	return name
}

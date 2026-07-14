package worker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func TestNewBackupTask(t *testing.T) {
	task, err := NewBackupTask()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskBackupDatabase {
		t.Errorf("expected task type %s, got %s", TaskBackupDatabase, task.Type())
	}
}

func TestBackupHandlerSkipsWhenServiceMissing(t *testing.T) {
	handler := NewBackupHandler(&WorkerDeps{})
	if err := handler(context.Background(), asynq.NewTask(TaskBackupDatabase, nil)); err != nil {
		t.Fatalf("expected nil error when backup service is not configured, got %v", err)
	}
}

func TestBackupHandlerCreatesBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake pg_dump script requires a POSIX shell")
	}

	// Stand-in for pg_dump: writes a byte to the --file target.
	pgDump := filepath.Join(t.TempDir(), "pg_dump")
	script := `#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--file" ]; then out="$a"; fi
  prev="$a"
done
printf 'DUMP' > "$out"
`
	if err := os.WriteFile(pgDump, []byte(script), 0o700); err != nil {
		t.Fatalf("writing fake pg_dump: %v", err)
	}

	dir := t.TempDir()
	backupSvc, err := service.NewBackupService(service.BackupServiceConfig{
		DatabaseURL: "postgres://user:pass@localhost:5432/tt?sslmode=disable",
		Dir:         dir,
		PgDumpPath:  pgDump,
		Timeout:     10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}

	handler := NewBackupHandler(&WorkerDeps{BackupSvc: backupSvc})
	if err := handler(context.Background(), asynq.NewTask(TaskBackupDatabase, nil)); err != nil {
		t.Fatalf("backup handler: %v", err)
	}

	backups, err := backupSvc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup after the scheduled job, got %d", len(backups))
	}
}

func TestBackupHandlerReturnsErrorOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake pg_dump script requires a POSIX shell")
	}

	pgDump := filepath.Join(t.TempDir(), "pg_dump")
	if err := os.WriteFile(pgDump, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("writing failing pg_dump: %v", err)
	}

	backupSvc, err := service.NewBackupService(service.BackupServiceConfig{
		DatabaseURL: "postgres://user:pass@localhost:5432/tt?sslmode=disable",
		Dir:         t.TempDir(),
		PgDumpPath:  pgDump,
		Timeout:     10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}

	handler := NewBackupHandler(&WorkerDeps{BackupSvc: backupSvc})
	if err := handler(context.Background(), asynq.NewTask(TaskBackupDatabase, nil)); err == nil {
		t.Fatal("expected an error when pg_dump fails so asynq retries")
	}
}

func TestBackupHandlerDoesNotRetryWhenBackupAlreadyRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake pg_dump script requires a POSIX shell")
	}

	// This pg_dump writes its output file and then lingers, so the service's
	// concurrency guard is held for the duration of the assertion below.
	pgDump := filepath.Join(t.TempDir(), "pg_dump")
	script := `#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--file" ]; then out="$a"; fi
  prev="$a"
done
printf 'DUMP' > "$out"
exec sleep 3
`
	if err := os.WriteFile(pgDump, []byte(script), 0o700); err != nil {
		t.Fatalf("writing fake pg_dump: %v", err)
	}

	dir := t.TempDir()
	backupSvc, err := service.NewBackupService(service.BackupServiceConfig{
		DatabaseURL: "postgres://user:pass@localhost:5432/tt?sslmode=disable",
		Dir:         dir,
		PgDumpPath:  pgDump,
		Timeout:     500 * time.Millisecond, // the lingering dump is killed by this
	}, nil)
	if err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}

	// An admin kicks off a backup by hand...
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = backupSvc.Create(context.Background(), service.SystemActor)
	}()

	// ...wait until it is actually running (its .partial file exists).
	if !waitForPartial(dir) {
		t.Fatal("the manual backup never started")
	}

	// ...and the scheduled job fires at the same moment. It must stand down
	// rather than return an error, which asynq would retry.
	handler := NewBackupHandler(&WorkerDeps{BackupSvc: backupSvc})
	if err := handler(context.Background(), asynq.NewTask(TaskBackupDatabase, nil)); err != nil {
		t.Errorf("expected a concurrent backup to be skipped, not retried, got %v", err)
	}

	<-done
}

// waitForPartial reports whether an in-progress dump (*.partial) appears in dir.
func waitForPartial(dir string) bool {
	for i := 0; i < 200; i++ {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".partial") {
					return true
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

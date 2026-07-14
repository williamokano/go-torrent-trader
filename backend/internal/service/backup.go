package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// Backup errors.
var (
	// ErrInvalidBackupName is returned when a caller-supplied backup name does
	// not match the server-generated naming scheme. Any attempt at path
	// traversal (or a symlink planted in the backup directory) lands here.
	ErrInvalidBackupName = errors.New("invalid backup name")
	// ErrBackupNotFound is returned when the named backup does not exist.
	ErrBackupNotFound = errors.New("backup not found")
	// ErrBackupInProgress is returned when a backup is already running. Only one
	// pg_dump may run at a time — concurrent dumps only add load without value.
	ErrBackupInProgress = errors.New("a backup is already in progress")
)

// backupTimeLayout is the UTC timestamp layout embedded in backup file names.
const backupTimeLayout = "20060102T150405Z"

// backupNamePattern matches exactly the names Create generates. Names are never
// taken from user input — download and delete accept a name and it must match
// this pattern before it is ever joined onto a filesystem path.
var backupNamePattern = regexp.MustCompile(`^backup-\d{8}T\d{6}Z-[0-9a-f]{8}\.dump$`)

// ValidateBackupName reports whether name is a well-formed, server-generated
// backup file name. It rejects anything containing a path separator, a parent
// directory reference, or characters outside the generated scheme, so the name
// is always safe to join onto the backup directory.
//
// Exported so the HTTP layer can reject bad input before it reaches the service.
func ValidateBackupName(name string) error {
	if name == "" || len(name) > 128 {
		return ErrInvalidBackupName
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return ErrInvalidBackupName
	}
	if name != filepath.Base(name) {
		return ErrInvalidBackupName
	}
	if !backupNamePattern.MatchString(name) {
		return ErrInvalidBackupName
	}
	return nil
}

// BackupServiceConfig holds the settings needed to run pg_dump.
type BackupServiceConfig struct {
	DatabaseURL string        // postgres:// connection URL; credentials are passed to pg_dump via env, never argv
	Dir         string        // directory backup files are written to
	PgDumpPath  string        // pg_dump binary (absolute path or resolved via PATH)
	Timeout     time.Duration // hard limit for a single pg_dump run
	Retention   int           // number of backups to keep; 0 keeps all
}

// BackupService creates, lists, serves and deletes pg_dump backups of the
// application database. Backup files live in a configured directory; there is
// no database table — the filesystem is the source of truth.
type BackupService struct {
	dir        string
	pgDumpPath string
	timeout    time.Duration
	retention  int
	// connEnv holds the libpq environment (PGHOST/PGUSER/PGPASSWORD/...) handed
	// to pg_dump. Credentials go in the environment, never in argv, so they do
	// not leak into the process list.
	connEnv []string
	bus     event.Bus
	running atomic.Bool
	now     func() time.Time
}

// NewBackupService creates a BackupService, ensures the backup directory exists
// and clears any partial files left behind by an interrupted dump.
func NewBackupService(cfg BackupServiceConfig, bus event.Bus) (*BackupService, error) {
	connEnv, err := pgEnvFromURL(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	dir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("resolving backup dir %q: %w", cfg.Dir, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating backup dir %q: %w", dir, err)
	}

	s := &BackupService{
		dir:        dir,
		pgDumpPath: cfg.PgDumpPath,
		timeout:    cfg.Timeout,
		retention:  cfg.Retention,
		connEnv:    connEnv,
		bus:        bus,
		now:        time.Now,
	}
	if s.pgDumpPath == "" {
		s.pgDumpPath = "pg_dump"
	}
	if s.timeout <= 0 {
		s.timeout = 30 * time.Minute
	}
	s.cleanPartials()
	return s, nil
}

// Dir returns the absolute backup directory. Useful for logging.
func (s *BackupService) Dir() string { return s.dir }

// Create runs pg_dump and writes a compressed custom-format dump to the backup
// directory. Only one backup may run at a time; a concurrent call returns
// ErrBackupInProgress.
//
// The dump is deliberately detached from the caller's cancellation (an admin
// closing the browser tab must not kill a half-finished disaster-recovery
// backup) but is still bounded by the configured timeout.
func (s *BackupService) Create(ctx context.Context, actor event.Actor) (*model.Backup, error) {
	if !s.running.CompareAndSwap(false, true) {
		return nil, ErrBackupInProgress
	}
	defer s.running.Store(false)

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating backup dir: %w", err)
	}

	suffix, err := randomHex(4)
	if err != nil {
		return nil, fmt.Errorf("generating backup name: %w", err)
	}
	name := fmt.Sprintf("backup-%s-%s.dump", s.now().UTC().Format(backupTimeLayout), suffix)
	dest := filepath.Join(s.dir, name)
	partial := dest + partialSuffix

	// The dump — and the bookkeeping that follows it — outlive the caller: an
	// admin closing the tab must not abort the dump, nor lose its activity-log
	// entry, nor skip retention pruning.
	detached := context.WithoutCancel(ctx)
	runCtx, cancel := context.WithTimeout(detached, s.timeout)
	defer cancel()

	// No shell is involved: the binary and every argument are passed as a slice,
	// and none of them are derived from user input. Connection credentials are
	// supplied through the environment (see pgEnvFromURL).
	cmd := exec.CommandContext(runCtx, s.pgDumpPath, //nolint:gosec // fixed args, no shell, path comes from config
		"--format=custom",
		"--compress=9",
		"--no-owner",
		"--no-privileges",
		"--file", partial,
	)
	cmd.Env = s.connEnv
	// After the context expires, don't wait forever on a child that ignores the
	// kill signal (or on pipes inherited by a grandchild).
	cmd.WaitDelay = 10 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(partial)
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("pg_dump timed out after %s: %w", s.timeout, err)
		}
		return nil, fmt.Errorf("pg_dump failed: %w: %s", err, truncateForError(stderr.String()))
	}

	if err := os.Rename(partial, dest); err != nil {
		_ = os.Remove(partial)
		return nil, fmt.Errorf("finalising backup: %w", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		return nil, fmt.Errorf("stat backup: %w", err)
	}

	backup := &model.Backup{
		Name:      name,
		Size:      info.Size(),
		CreatedAt: info.ModTime().UTC(),
	}

	if s.bus != nil {
		s.bus.Publish(detached, &event.BackupCreatedEvent{
			Base: event.NewBase(event.BackupCreated, actor),
			Name: backup.Name,
			Size: backup.Size,
		})
	}

	s.prune(detached)

	return backup, nil
}

// List returns all backups in the backup directory, newest first.
func (s *BackupService) List(_ context.Context) ([]model.Backup, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.Backup{}, nil
		}
		return nil, fmt.Errorf("reading backup dir: %w", err)
	}

	backups := make([]model.Backup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Anything that is not a well-formed backup file (partials, strays) is
		// ignored: it could not be downloaded or deleted through the API anyway.
		if err := ValidateBackupName(entry.Name()); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		backups = append(backups, model.Backup{
			Name:      entry.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime().UTC(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		if backups[i].CreatedAt.Equal(backups[j].CreatedAt) {
			return backups[i].Name > backups[j].Name
		}
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// Open returns a reader for the named backup along with its size. The caller
// must close the reader.
func (s *BackupService) Open(name string) (io.ReadCloser, int64, error) {
	path, info, err := s.resolveExisting(name)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path) //nolint:gosec // path is validated by resolveExisting and confined to the backup dir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrBackupNotFound
		}
		return nil, 0, fmt.Errorf("opening backup: %w", err)
	}
	return f, info.Size(), nil
}

// Delete removes the named backup file.
func (s *BackupService) Delete(ctx context.Context, name string, actor event.Actor) error {
	path, _, err := s.resolveExisting(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrBackupNotFound
		}
		return fmt.Errorf("deleting backup: %w", err)
	}

	if s.bus != nil {
		s.bus.Publish(ctx, &event.BackupDeletedEvent{
			Base: event.NewBase(event.BackupDeleted, actor),
			Name: name,
		})
	}
	return nil
}

// partialSuffix marks an in-progress dump. Partial files never match
// backupNamePattern, so they are invisible to List, Open and Delete.
const partialSuffix = ".partial"

// resolveExisting validates the name, resolves it inside the backup directory
// and confirms it points at a regular file. Symlinks and directories are
// rejected, so a symlink planted in the backup directory cannot be used to read
// or delete a file elsewhere on the filesystem.
func (s *BackupService) resolveExisting(name string) (string, os.FileInfo, error) {
	if err := ValidateBackupName(name); err != nil {
		return "", nil, err
	}

	path := filepath.Join(s.dir, name)

	// Defence in depth: after cleaning, the path must still be exactly one
	// element below the backup directory.
	rel, err := filepath.Rel(s.dir, path)
	if err != nil || rel != name {
		return "", nil, ErrInvalidBackupName
	}

	// Lstat (not Stat) so a symlink is reported as a symlink rather than its target.
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, ErrBackupNotFound
		}
		return "", nil, fmt.Errorf("stat backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, ErrInvalidBackupName
	}
	return path, info, nil
}

// prune deletes the oldest backups beyond the retention count. Retention of 0
// keeps everything. Failures are logged, never fatal — a successful dump must
// not be reported as a failure because housekeeping went wrong.
func (s *BackupService) prune(ctx context.Context) {
	if s.retention <= 0 {
		return
	}
	backups, err := s.List(ctx)
	if err != nil {
		slog.Error("backup prune: failed to list backups", "error", err)
		return
	}
	if len(backups) <= s.retention {
		return
	}
	for _, old := range backups[s.retention:] {
		path := filepath.Join(s.dir, old.Name)
		if err := os.Remove(path); err != nil {
			slog.Error("backup prune: failed to delete old backup", "name", old.Name, "error", err)
			continue
		}
		slog.Info("backup prune: deleted old backup", "name", old.Name)
	}
}

// cleanPartials removes leftover *.partial files from dumps interrupted by a
// crash or shutdown. Safe at startup: nothing can legitimately be in progress.
func (s *BackupService) cleanPartials() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), partialSuffix) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil {
			slog.Warn("failed to remove partial backup file", "name", entry.Name(), "error", err)
		}
	}
}

// pgEnvFromURL converts a postgres connection URL into libpq environment
// variables for pg_dump. Passing the password through the environment (rather
// than a --dbname=postgres://user:pass@... argument) keeps it out of the
// process list, where any local user could read it.
func pgEnvFromURL(raw string) ([]string, error) {
	if raw == "" {
		return nil, errors.New("database URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("unsupported database URL scheme %q (want postgres://)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		return nil, errors.New("database URL has no database name")
	}

	env := []string{
		// PATH is kept so a bare "pg_dump" resolves; everything else is dropped
		// so the child process sees no unrelated application secrets.
		"PATH=" + os.Getenv("PATH"),
		"PGHOST=" + host,
		"PGPORT=" + port,
		"PGDATABASE=" + database,
	}
	if user := u.User.Username(); user != "" {
		env = append(env, "PGUSER="+user)
	}
	if password, ok := u.User.Password(); ok && password != "" {
		env = append(env, "PGPASSWORD="+password)
	}
	if sslmode := u.Query().Get("sslmode"); sslmode != "" {
		env = append(env, "PGSSLMODE="+sslmode)
	}
	return env, nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// truncateForError trims pg_dump stderr so a runaway error message cannot bloat
// the API response or the log line.
func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	const max = 500
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// BackupManager is the subset of service.BackupService the HTTP layer needs.
type BackupManager interface {
	Create(ctx context.Context, actor event.Actor) (*model.Backup, error)
	List(ctx context.Context) ([]model.Backup, error)
	Open(name string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, name string, actor event.Actor) error
}

// BackupHandler exposes database backup management. Every route is mounted
// behind RequireAdmin — see router.go.
type BackupHandler struct {
	backups BackupManager
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(backups BackupManager) *BackupHandler {
	return &BackupHandler{backups: backups}
}

// HandleListBackups handles GET /api/v1/admin/backups.
func (h *BackupHandler) HandleListBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := h.backups.List(r.Context())
	if err != nil {
		slog.Error("failed to list backups", "error", err)
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list backups")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"backups": backups})
}

// HandleCreateBackup handles POST /api/v1/admin/backups. The dump runs
// synchronously; only one backup may run at a time.
func (h *BackupHandler) HandleCreateBackup(w http.ResponseWriter, r *http.Request) {
	backup, err := h.backups.Create(r.Context(), actorFromRequest(r))
	if err != nil {
		if errors.Is(err, service.ErrBackupInProgress) {
			ErrorResponse(w, http.StatusConflict, "backup_in_progress", "a backup is already in progress")
			return
		}
		// The pg_dump error can carry connection details — log it, don't return it.
		slog.Error("failed to create backup", "error", err)
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create backup")
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{"backup": backup})
}

// HandleDownloadBackup handles GET /api/v1/admin/backups/{name}/download.
func (h *BackupHandler) HandleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	// Reject anything that is not a server-generated name before it gets
	// anywhere near the filesystem. The service validates again.
	if err := service.ValidateBackupName(name); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid backup name")
		return
	}

	file, size, err := h.backups.Open(name)
	if err != nil {
		h.writeBackupError(w, err, "failed to download backup")
		return
	}
	defer func() { _ = file.Close() }()

	// name matched backupNamePattern, so it holds no quotes, spaces or newlines
	// that could break out of the Content-Disposition header.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, file); err != nil {
		// Headers are already sent; the client sees a truncated body.
		slog.Error("failed to stream backup", "name", name, "error", err)
	}
}

// HandleDeleteBackup handles DELETE /api/v1/admin/backups/{name}.
func (h *BackupHandler) HandleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := service.ValidateBackupName(name); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid backup name")
		return
	}

	if err := h.backups.Delete(r.Context(), name, actorFromRequest(r)); err != nil {
		h.writeBackupError(w, err, "failed to delete backup")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeBackupError maps service errors to HTTP responses.
func (h *BackupHandler) writeBackupError(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, service.ErrInvalidBackupName):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid backup name")
	case errors.Is(err, service.ErrBackupNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "backup not found")
	default:
		slog.Error(fallbackMsg, "error", err)
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", fallbackMsg)
	}
}

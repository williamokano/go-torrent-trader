package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock -------------------------------------------------------------------

type backupManagerStub struct {
	created   *model.Backup
	createErr error
	list      []model.Backup
	listErr   error
	content   string
	openErr   error
	deleteErr error

	createCalls   int
	openedNames   []string
	deletedNames  []string
	lastActor     event.Actor
	closedOnceGot bool
}

func (m *backupManagerStub) Create(_ context.Context, actor event.Actor) (*model.Backup, error) {
	m.createCalls++
	m.lastActor = actor
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.created, nil
}

func (m *backupManagerStub) List(_ context.Context) ([]model.Backup, error) {
	return m.list, m.listErr
}

func (m *backupManagerStub) Open(name string) (io.ReadCloser, int64, error) {
	m.openedNames = append(m.openedNames, name)
	if m.openErr != nil {
		return nil, 0, m.openErr
	}
	return &trackedReader{Reader: strings.NewReader(m.content), stub: m}, int64(len(m.content)), nil
}

func (m *backupManagerStub) Delete(_ context.Context, name string, actor event.Actor) error {
	m.deletedNames = append(m.deletedNames, name)
	m.lastActor = actor
	return m.deleteErr
}

type trackedReader struct {
	io.Reader
	stub *backupManagerStub
}

func (r *trackedReader) Close() error {
	r.stub.closedOnceGot = true
	return nil
}

const validBackupName = "backup-20260714T031500Z-a1b2c3d4.dump"

// --- list -------------------------------------------------------------------

func TestListBackups(t *testing.T) {
	stub := &backupManagerStub{list: []model.Backup{
		{Name: validBackupName, Size: 2048, CreatedAt: time.Date(2026, 7, 14, 3, 15, 0, 0, time.UTC)},
	}}
	h := NewBackupHandler(stub)

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil), 1, 3)
	w := httptest.NewRecorder()
	h.HandleListBackups(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := decodeBody(t, w)
	backups, ok := body["backups"].([]interface{})
	if !ok || len(backups) != 1 {
		t.Fatalf("expected 1 backup in response, got %v", body["backups"])
	}
	first := backups[0].(map[string]interface{})
	if first["name"] != validBackupName {
		t.Errorf("expected name %s, got %v", validBackupName, first["name"])
	}
	if first["size"].(float64) != 2048 {
		t.Errorf("expected size 2048, got %v", first["size"])
	}
}

func TestListBackupsInternalError(t *testing.T) {
	h := NewBackupHandler(&backupManagerStub{listErr: errors.New("disk on fire")})

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil), 1, 3)
	w := httptest.NewRecorder()
	h.HandleListBackups(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// --- create -----------------------------------------------------------------

func TestCreateBackup(t *testing.T) {
	stub := &backupManagerStub{created: &model.Backup{
		Name: validBackupName, Size: 128, CreatedAt: time.Now().UTC(),
	}}
	h := NewBackupHandler(stub)

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil), 42, 3)
	w := httptest.NewRecorder()
	h.HandleCreateBackup(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if stub.createCalls != 1 {
		t.Errorf("expected 1 create call, got %d", stub.createCalls)
	}
	if stub.lastActor.ID != 42 {
		t.Errorf("expected actor id 42, got %d", stub.lastActor.ID)
	}
	body := decodeBody(t, w)
	backup, ok := body["backup"].(map[string]interface{})
	if !ok || backup["name"] != validBackupName {
		t.Fatalf("expected backup in response, got %v", body)
	}
}

func TestCreateBackupConflictWhenInProgress(t *testing.T) {
	h := NewBackupHandler(&backupManagerStub{createErr: service.ErrBackupInProgress})

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil), 1, 3)
	w := httptest.NewRecorder()
	h.HandleCreateBackup(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestCreateBackupInternalErrorDoesNotLeakDetails(t *testing.T) {
	h := NewBackupHandler(&backupManagerStub{
		createErr: errors.New(`pg_dump failed: connection to db.internal failed: password "hunter2" rejected`),
	})

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil), 1, 3)
	w := httptest.NewRecorder()
	h.HandleCreateBackup(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "hunter2") || strings.Contains(w.Body.String(), "db.internal") {
		t.Errorf("pg_dump error details leaked to client: %s", w.Body.String())
	}
}

// --- download ---------------------------------------------------------------

func TestDownloadBackup(t *testing.T) {
	stub := &backupManagerStub{content: "PGDMP-binary-bytes"}
	h := NewBackupHandler(stub)

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/"+validBackupName+"/download", nil), 1, 3)
	req = withURLParam(req, "name", validBackupName)
	w := httptest.NewRecorder()
	h.HandleDownloadBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Body.String(); got != "PGDMP-binary-bytes" {
		t.Errorf("unexpected body: %q", got)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("expected octet-stream, got %q", ct)
	}
	wantDisposition := `attachment; filename="` + validBackupName + `"`
	if cd := w.Header().Get("Content-Disposition"); cd != wantDisposition {
		t.Errorf("expected Content-Disposition %q, got %q", wantDisposition, cd)
	}
	if cl := w.Header().Get("Content-Length"); cl != "18" {
		t.Errorf("expected Content-Length 18, got %q", cl)
	}
	if !stub.closedOnceGot {
		t.Error("expected the backup file handle to be closed")
	}
}

func TestDownloadBackupNotFound(t *testing.T) {
	h := NewBackupHandler(&backupManagerStub{openErr: service.ErrBackupNotFound})

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/x/download", nil), 1, 3)
	req = withURLParam(req, "name", validBackupName)
	w := httptest.NewRecorder()
	h.HandleDownloadBackup(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestDownloadBackupRejectsPathTraversal proves the handler refuses traversal
// names without ever reaching the service.
func TestDownloadBackupRejectsPathTraversal(t *testing.T) {
	traversal := []string{
		"../../etc/passwd",
		"../../../../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"/etc/passwd",
		"..",
		"backup-20260714T031500Z-a1b2c3d4.dump/../../../etc/passwd",
		`..\..\windows\system32\config\sam`,
		"",
	}

	for _, name := range traversal {
		stub := &backupManagerStub{content: "should never be read"}
		h := NewBackupHandler(stub)

		req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/x/download", nil), 1, 3)
		req = withURLParam(req, "name", name)
		w := httptest.NewRecorder()
		h.HandleDownloadBackup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("download %q: expected 400, got %d", name, w.Code)
		}
		if len(stub.openedNames) != 0 {
			t.Errorf("download %q: reached the service with %v", name, stub.openedNames)
		}
	}
}

// --- delete -----------------------------------------------------------------

func TestDeleteBackup(t *testing.T) {
	stub := &backupManagerStub{}
	h := NewBackupHandler(stub)

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/"+validBackupName, nil), 9, 3)
	req = withURLParam(req, "name", validBackupName)
	w := httptest.NewRecorder()
	h.HandleDeleteBackup(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if len(stub.deletedNames) != 1 || stub.deletedNames[0] != validBackupName {
		t.Errorf("expected delete of %s, got %v", validBackupName, stub.deletedNames)
	}
	if stub.lastActor.ID != 9 {
		t.Errorf("expected actor id 9, got %d", stub.lastActor.ID)
	}
}

func TestDeleteBackupNotFound(t *testing.T) {
	h := NewBackupHandler(&backupManagerStub{deleteErr: service.ErrBackupNotFound})

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/x", nil), 1, 3)
	req = withURLParam(req, "name", validBackupName)
	w := httptest.NewRecorder()
	h.HandleDeleteBackup(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteBackupServiceRejectsName(t *testing.T) {
	// Defence in depth: even if the handler check were bypassed, a service-level
	// ErrInvalidBackupName must surface as 400.
	h := NewBackupHandler(&backupManagerStub{deleteErr: service.ErrInvalidBackupName})

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/x", nil), 1, 3)
	req = withURLParam(req, "name", validBackupName)
	w := httptest.NewRecorder()
	h.HandleDeleteBackup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteBackupRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../../etc/passwd", "/etc/passwd", "..", "sub/dir.dump", ""} {
		stub := &backupManagerStub{}
		h := NewBackupHandler(stub)

		req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/x", nil), 1, 3)
		req = withURLParam(req, "name", name)
		w := httptest.NewRecorder()
		h.HandleDeleteBackup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("delete %q: expected 400, got %d", name, w.Code)
		}
		if len(stub.deletedNames) != 0 {
			t.Errorf("delete %q: reached the service with %v", name, stub.deletedNames)
		}
	}
}

func TestDeleteBackupInternalError(t *testing.T) {
	h := NewBackupHandler(&backupManagerStub{deleteErr: errors.New("permission denied")})

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/x", nil), 1, 3)
	req = withURLParam(req, "name", validBackupName)
	w := httptest.NewRecorder()
	h.HandleDeleteBackup(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// --- routing (admin gate + real chi params) ---------------------------------

// backupRouter mounts the backup routes exactly as router.go does: behind
// RequireAdmin, with the file name as a chi URL param.
func backupRouter(stub *backupManagerStub) chi.Router {
	h := NewBackupHandler(stub)
	r := chi.NewRouter()
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(middleware.RequireAdmin)
		r.Get("/backups", h.HandleListBackups)
		r.Post("/backups", h.HandleCreateBackup)
		r.Get("/backups/{name}/download", h.HandleDownloadBackup)
		r.Delete("/backups/{name}", h.HandleDeleteBackup)
	})
	return r
}

func asUser(r *http.Request, perms model.Permissions) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, int64(1))
	ctx = context.WithValue(ctx, middleware.PermissionsKey, perms)
	return r.WithContext(ctx)
}

func TestBackupRoutesRequireAdmin(t *testing.T) {
	stub := &backupManagerStub{created: &model.Backup{Name: validBackupName}}
	router := backupRouter(stub)

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/"+validBackupName+"/download", nil),
		httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/"+validBackupName, nil),
	}

	// A moderator (staff, but not admin) must not reach any backup endpoint.
	for _, req := range requests {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, asUser(req, model.Permissions{Level: 2, IsModerator: true}))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 for non-admin, got %d", req.Method, req.URL.Path, w.Code)
		}
	}

	if stub.createCalls != 0 || len(stub.openedNames) != 0 || len(stub.deletedNames) != 0 {
		t.Error("non-admin requests reached the backup service")
	}

	// An admin gets through.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, asUser(httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil), model.Permissions{Level: 3, IsAdmin: true}))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for admin, got %d", w.Code)
	}
}

// TestBackupRouteRejectsEncodedTraversal drives a real chi router with a
// percent-encoded traversal payload — the shape an attacker would actually send,
// since a raw ../../ would never match the route.
func TestBackupRouteRejectsEncodedTraversal(t *testing.T) {
	stub := &backupManagerStub{content: "should never be read"}
	router := backupRouter(stub)
	admin := model.Permissions{Level: 3, IsAdmin: true}

	targets := []string{
		"/api/v1/admin/backups/..%2F..%2F..%2Fetc%2Fpasswd/download",
		"/api/v1/admin/backups/%2Fetc%2Fpasswd/download",
		"/api/v1/admin/backups/%2E%2E%2F%2E%2E%2Fetc%2Fpasswd/download",
	}
	for _, target := range targets {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, asUser(httptest.NewRequest(http.MethodGet, target, nil), admin))
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s: expected 400, got %d (body: %s)", target, w.Code, w.Body.String())
		}
	}

	deleteTargets := []string{
		"/api/v1/admin/backups/..%2F..%2F..%2Fetc%2Fpasswd",
		"/api/v1/admin/backups/%2Fetc%2Fpasswd",
	}
	for _, target := range deleteTargets {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, asUser(httptest.NewRequest(http.MethodDelete, target, nil), admin))
		if w.Code != http.StatusBadRequest {
			t.Errorf("DELETE %s: expected 400, got %d (body: %s)", target, w.Code, w.Body.String())
		}
	}

	if len(stub.openedNames) != 0 || len(stub.deletedNames) != 0 {
		t.Errorf("traversal payload reached the service: opened=%v deleted=%v", stub.openedNames, stub.deletedNames)
	}
}

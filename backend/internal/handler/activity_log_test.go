package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

type fakeActivityLogRepo struct {
	lastOpts repository.ListActivityLogsOptions
	logs     []model.ActivityLog
}

func (f *fakeActivityLogRepo) Create(_ context.Context, _ *model.ActivityLog) error {
	return nil
}

func (f *fakeActivityLogRepo) List(_ context.Context, opts repository.ListActivityLogsOptions) ([]model.ActivityLog, int64, error) {
	f.lastOpts = opts
	return f.logs, int64(len(f.logs)), nil
}

func listActivityLogs(t *testing.T, repo *fakeActivityLogRepo, perms model.Permissions) map[string]interface{} {
	t.Helper()
	h := NewActivityLogHandler(service.NewActivityLogService(repo))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity-logs", nil)
	ctx := context.WithValue(req.Context(), middleware.PermissionsKey, perms)
	rec := httptest.NewRecorder()
	h.HandleList(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestActivityLogListMemberView(t *testing.T) {
	meta := `{"user_id":7,"token":"secret-invite-token"}`
	repo := &fakeActivityLogRepo{logs: []model.ActivityLog{
		{ID: 1, EventType: "user_registered", Message: "alice joined the site", Metadata: &meta},
	}}

	body := listActivityLogs(t, repo, model.Permissions{})

	if len(repo.lastOpts.ExcludeEventTypes) == 0 {
		t.Error("expected staff-only event types excluded for member request")
	}
	logs := body["logs"].([]interface{})
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	entry := logs[0].(map[string]interface{})
	if _, ok := entry["metadata"]; ok {
		t.Error("expected metadata to be omitted for non-staff viewers")
	}
	if entry["message"] != "alice joined the site" {
		t.Errorf("unexpected message: %v", entry["message"])
	}
}

func TestActivityLogListStaffView(t *testing.T) {
	meta := `{"user_id":7}`
	repo := &fakeActivityLogRepo{logs: []model.ActivityLog{
		{ID: 1, EventType: "backup_created", Message: "operator created database backup: x.dump", Metadata: &meta},
	}}

	body := listActivityLogs(t, repo, model.Permissions{IsModerator: true})

	if len(repo.lastOpts.ExcludeEventTypes) != 0 {
		t.Errorf("expected no exclusions for staff request, got %v", repo.lastOpts.ExcludeEventTypes)
	}
	logs := body["logs"].([]interface{})
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	entry := logs[0].(map[string]interface{})
	if _, ok := entry["metadata"]; !ok {
		t.Error("expected metadata to be present for staff viewers")
	}
}

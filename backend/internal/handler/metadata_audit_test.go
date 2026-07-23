package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// stubAuditRepo records the uploader scope the service resolved.
type stubAuditRepo struct {
	gotUploaderID *int64
	called        bool
}

func (s *stubAuditRepo) ListMissingRequiredMetadata(_ context.Context, _ int64, _ []string, uploaderID *int64) ([]model.Torrent, error) {
	s.called = true
	s.gotUploaderID = uploaderID
	return nil, nil
}

func newAuditHandler() (*handler.MetadataAuditHandler, *stubAuditRepo) {
	catRepo := newMockCategoryRepo()
	// A required field so the audit repo is actually reached.
	catRepo.categories = append(catRepo.categories, &model.Category{
		ID:             1,
		Name:           "Movies",
		MetadataSchema: json.RawMessage(`[{"key":"year","label":"Year","type":"number","required":true}]`),
	})
	audit := &stubAuditRepo{}
	svc := service.NewMetadataAuditService(audit, catRepo)
	return handler.NewMetadataAuditHandler(svc), audit
}

func auditRequest(scope string, userID int64, admin bool) *http.Request {
	url := "/api/v1/torrents/metadata-issues"
	if scope != "" {
		url += "?scope=" + scope
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	ctx := req.Context()
	if userID != 0 {
		ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
		ctx = context.WithValue(ctx, middleware.PermissionsKey, model.Permissions{IsAdmin: admin})
	}
	return req.WithContext(ctx)
}

func TestHandleMetadataIssues_Unauthenticated(t *testing.T) {
	h, _ := newAuditHandler()
	rec := httptest.NewRecorder()
	h.HandleMetadataIssues(rec, auditRequest("", 0, false))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleMetadataIssues_NonAdminAllForbidden(t *testing.T) {
	h, audit := newAuditHandler()
	rec := httptest.NewRecorder()
	h.HandleMetadataIssues(rec, auditRequest("all", 7, false))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a non-admin requesting scope=all", rec.Code)
	}
	if audit.called {
		t.Error("audit repo should not be queried when access is denied")
	}
}

func TestHandleMetadataIssues_NonAdminScopedToSelf(t *testing.T) {
	h, audit := newAuditHandler()
	rec := httptest.NewRecorder()
	h.HandleMetadataIssues(rec, auditRequest("", 7, false))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if audit.gotUploaderID == nil || *audit.gotUploaderID != 7 {
		t.Errorf("uploaderID = %v, want 7 (scoped to the caller)", audit.gotUploaderID)
	}
}

func TestHandleMetadataIssues_AdminAllUnscoped(t *testing.T) {
	h, audit := newAuditHandler()
	rec := httptest.NewRecorder()
	h.HandleMetadataIssues(rec, auditRequest("all", 9, true))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if audit.gotUploaderID != nil {
		t.Errorf("uploaderID = %v, want nil (all uploaders)", *audit.gotUploaderID)
	}
}

package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// --- stub repositories ---

type stubConnectorRepo struct {
	rows   map[int64]*model.NotificationConnector
	nextID int64
}

func newStubConnectorRepo() *stubConnectorRepo {
	return &stubConnectorRepo{rows: map[int64]*model.NotificationConnector{}, nextID: 1}
}

func (s *stubConnectorRepo) Create(_ context.Context, c *model.NotificationConnector) error {
	c.ID = s.nextID
	s.nextID++
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	stored := *c
	s.rows[c.ID] = &stored
	return nil
}

func (s *stubConnectorRepo) GetByID(_ context.Context, id int64) (*model.NotificationConnector, error) {
	c, ok := s.rows[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copied := *c
	return &copied, nil
}

func (s *stubConnectorRepo) List(_ context.Context) ([]model.NotificationConnector, error) {
	var out []model.NotificationConnector
	for id := int64(1); id < s.nextID; id++ {
		if c, ok := s.rows[id]; ok {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (s *stubConnectorRepo) ListEnabled(ctx context.Context) ([]model.NotificationConnector, error) {
	return s.List(ctx)
}

func (s *stubConnectorRepo) Update(_ context.Context, c *model.NotificationConnector) error {
	if _, ok := s.rows[c.ID]; !ok {
		return sql.ErrNoRows
	}
	stored := *c
	s.rows[c.ID] = &stored
	return nil
}

func (s *stubConnectorRepo) Delete(_ context.Context, id int64) error {
	if _, ok := s.rows[id]; !ok {
		return sql.ErrNoRows
	}
	delete(s.rows, id)
	return nil
}

func (s *stubConnectorRepo) CountByKind(_ context.Context, kind string) (int64, error) {
	var n int64
	for _, c := range s.rows {
		if c.Kind == kind {
			n++
		}
	}
	return n, nil
}

var _ repository.ConnectorRepository = (*stubConnectorRepo)(nil)

type stubConnectorDeliveryRepo struct {
	rows   map[int64]*model.ConnectorDelivery
	nextID int64
}

func newStubConnectorDeliveryRepo() *stubConnectorDeliveryRepo {
	return &stubConnectorDeliveryRepo{rows: map[int64]*model.ConnectorDelivery{}, nextID: 1}
}

func (s *stubConnectorDeliveryRepo) InsertPending(_ context.Context, d *model.ConnectorDelivery) (bool, error) {
	d.ID = s.nextID
	s.nextID++
	d.CreatedAt = time.Now()
	d.UpdatedAt = d.CreatedAt
	stored := *d
	s.rows[d.ID] = &stored
	return true, nil
}

func (s *stubConnectorDeliveryRepo) ListDue(context.Context, int64, time.Time, int) ([]model.ConnectorDelivery, error) {
	return nil, nil
}
func (s *stubConnectorDeliveryRepo) ClaimForDelivery(context.Context, int64, time.Time, time.Time) (bool, error) {
	return true, nil
}
func (s *stubConnectorDeliveryRepo) CountSentSince(context.Context, int64, time.Time) (int64, error) {
	return 0, nil
}

func (s *stubConnectorDeliveryRepo) MarkSent(_ context.Context, id int64, status string) error {
	d, ok := s.rows[id]
	if !ok || d.Status != model.DeliveryPending {
		return sql.ErrNoRows
	}
	d.Status = status
	return nil
}

func (s *stubConnectorDeliveryRepo) MarkFailedAttempt(_ context.Context, id int64, attempts int, lastError string, next *time.Time) error {
	d, ok := s.rows[id]
	if !ok {
		return sql.ErrNoRows
	}
	d.Attempts = attempts
	msg := lastError
	d.LastError = &msg
	d.NextAttemptAt = next
	if next == nil {
		d.Status = model.DeliveryFailed
	}
	return nil
}

func (s *stubConnectorDeliveryRepo) ListByInstance(_ context.Context, instanceID int64, page, perPage int) ([]model.ConnectorDelivery, int64, error) {
	var all []model.ConnectorDelivery
	for id := int64(1); id < s.nextID; id++ {
		if d, ok := s.rows[id]; ok && d.InstanceID == instanceID {
			all = append(all, *d)
		}
	}
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start > len(all) {
		start = len(all)
	}
	end := start + perPage
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], int64(len(all)), nil
}

func (s *stubConnectorDeliveryRepo) LatestStatusByInstance(context.Context) (map[int64]model.ConnectorDelivery, error) {
	latest := map[int64]model.ConnectorDelivery{}
	for id := int64(1); id < s.nextID; id++ {
		if d, ok := s.rows[id]; ok {
			latest[d.InstanceID] = *d
		}
	}
	return latest, nil
}

func (s *stubConnectorDeliveryRepo) InstancesWithDue(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (s *stubConnectorDeliveryRepo) DeleteOld(context.Context, time.Time) (int64, error) {
	return 0, nil
}

var _ repository.ConnectorDeliveryRepository = (*stubConnectorDeliveryRepo)(nil)

// --- stub connector ---

type stubWebhookConnector struct {
	deliverErr error
}

func (s *stubWebhookConnector) Kind() string           { return "webhook" }
func (s *stubWebhookConnector) Singleton() bool        { return false }
func (s *stubWebhookConnector) SecretFields() []string { return []string{"hmac_secret"} }
func (s *stubWebhookConnector) Coalescable() bool      { return false }

func (s *stubWebhookConnector) ValidateConfig(cfg json.RawMessage) error {
	var parsed struct {
		URL string `json:"url"`
	}
	if err := connector.DecodeConfig(cfg, &parsed); err != nil {
		return err
	}
	if parsed.URL == "" {
		return errors.New("url is required")
	}
	return nil
}

func (s *stubWebhookConnector) Deliver(context.Context, connector.Instance, connector.Announcement) error {
	return s.deliverErr
}

type stubChatConnector struct{}

func (stubChatConnector) Kind() string                         { return "chat" }
func (stubChatConnector) Singleton() bool                      { return true }
func (stubChatConnector) SecretFields() []string               { return nil }
func (stubChatConnector) Coalescable() bool                    { return true }
func (stubChatConnector) ValidateConfig(json.RawMessage) error { return nil }
func (stubChatConnector) Deliver(context.Context, connector.Instance, connector.Announcement) error {
	return nil
}

// --- harness ---

type connectorAdminHarness struct {
	router   http.Handler
	sessions service.SessionStore
	repo     *stubConnectorRepo
	hook     *stubWebhookConnector
	admin    string
	member   string
}

func setupConnectorAdmin(t *testing.T) *connectorAdminHarness {
	t.Helper()

	userRepo := newMockUserRepo()
	groupStore := newGroupCRUDStore() // group 1 = admin, 5 = regular user
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()

	hook := &stubWebhookConnector{}
	registry := connector.NewRegistry()
	registry.Register(hook)
	registry.Register(stubChatConnector{})

	repo := newStubConnectorRepo()
	svc := service.NewConnectorService(repo, newStubConnectorDeliveryRepo(), registry, "https://tracker.test")

	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(),
		&testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL,
		service.DefaultRefreshTokenTTL, groupStore, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:      authSvc,
		SessionStore:     sessions,
		AdminService:     service.NewAdminService(userRepo, groupStore, bus),
		ConnectorService: svc,
	})

	return &connectorAdminHarness{
		router:   router,
		sessions: sessions,
		repo:     repo,
		hook:     hook,
		admin:    createSessionWithGroup(sessions, 6001, 1),
		member:   createSessionWithGroup(sessions, 6002, 5),
	}
}

func webhookBody() map[string]interface{} {
	return map[string]interface{}{
		"kind":    "webhook",
		"name":    "My Hook",
		"enabled": true,
		"config":  map[string]interface{}{"url": "https://example.test/hook", "hmac_secret": "s3cr3t"},
		"filters": map[string]interface{}{},
	}
}

// createConnector posts one webhook instance and returns its ID.
func (h *connectorAdminHarness) createConnector(t *testing.T) int64 {
	t.Helper()
	rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors", webhookBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Connector struct {
			ID int64 `json:"id"`
		} `json:"connector"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return resp.Connector.ID
}

// --- authorization ---

func TestConnectorRoutesRejectNonAdmin(t *testing.T) {
	h := setupConnectorAdmin(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/connectors"},
		{http.MethodPost, "/api/v1/admin/connectors"},
		{http.MethodGet, "/api/v1/admin/connectors/1"},
		{http.MethodPut, "/api/v1/admin/connectors/1"},
		{http.MethodDelete, "/api/v1/admin/connectors/1"},
		{http.MethodPost, "/api/v1/admin/connectors/1/test"},
		{http.MethodGet, "/api/v1/admin/connectors/1/deliveries"},
	}
	for _, c := range cases {
		rec := doGroupRequest(t, h.router, h.member, c.method, c.path, map[string]interface{}{})
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 for a non-admin, got %d", c.method, c.path, rec.Code)
		}
	}
}

// --- CRUD ---

func TestConnectorCreateAndList(t *testing.T) {
	h := setupConnectorAdmin(t)
	h.createConnector(t)

	rec := doGroupRequest(t, h.router, h.admin, http.MethodGet, "/api/v1/admin/connectors", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Connectors []map[string]interface{} `json:"connectors"`
		Kinds      []string                 `json:"kinds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(resp.Connectors) != 1 || resp.Connectors[0]["name"] != "My Hook" {
		t.Fatalf("connectors = %+v", resp.Connectors)
	}
	// The UI's kind picker comes from here, so it always matches this build.
	if len(resp.Kinds) != 2 || resp.Kinds[0] != "chat" || resp.Kinds[1] != "webhook" {
		t.Fatalf("kinds = %v, want [chat webhook]", resp.Kinds)
	}
}

// The response body is the thing an admin's browser (and any proxy log) sees.
func TestConnectorResponsesNeverContainTheSecret(t *testing.T) {
	h := setupConnectorAdmin(t)
	h.createConnector(t)

	for _, path := range []string{"/api/v1/admin/connectors", "/api/v1/admin/connectors/1"} {
		rec := doGroupRequest(t, h.router, h.admin, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "s3cr3t") {
			t.Fatalf("GET %s leaked the secret: %s", path, rec.Body.String())
		}
	}

	rec := doGroupRequest(t, h.router, h.admin, http.MethodGet, "/api/v1/admin/connectors/1", nil)
	var resp struct {
		Connector struct {
			SecretsSet []string        `json:"secrets_set"`
			Config     json.RawMessage `json:"config"`
		} `json:"connector"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Connector.SecretsSet) != 1 || resp.Connector.SecretsSet[0] != "hmac_secret" {
		t.Fatalf("secrets_set = %v, want [hmac_secret]", resp.Connector.SecretsSet)
	}
	if strings.Contains(string(resp.Connector.Config), "hmac_secret") {
		t.Fatalf("config still names the secret key: %s", resp.Connector.Config)
	}
}

func TestConnectorCreateRejectsSecondSingleton(t *testing.T) {
	h := setupConnectorAdmin(t)

	body := map[string]interface{}{"kind": "chat", "name": "Shoutbox", "config": map[string]interface{}{}}
	if rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors", body); rec.Code != http.StatusCreated {
		t.Fatalf("first chat instance: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second chat instance: expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectorCreateRejectsInvalidConfig(t *testing.T) {
	h := setupConnectorAdmin(t)

	body := webhookBody()
	body["config"] = map[string]interface{}{} // no url
	rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "url is required") {
		t.Fatalf("body = %s, want the connector's own message surfaced", rec.Body.String())
	}
}

func TestConnectorCreateRejectsUnknownKind(t *testing.T) {
	h := setupConnectorAdmin(t)

	body := webhookBody()
	body["kind"] = "carrier-pigeon"
	if rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// The admin list's enable toggle is a whole-row PUT with the flag flipped; the
// secret is not in the form, so it must survive untouched.
func TestConnectorUpdateTogglesEnabledAndKeepsSecret(t *testing.T) {
	h := setupConnectorAdmin(t)
	id := h.createConnector(t)

	body := map[string]interface{}{
		"name":    "My Hook",
		"enabled": false,
		"config":  map[string]interface{}{"url": "https://example.test/hook"},
		"filters": map[string]interface{}{},
	}
	rec := doGroupRequest(t, h.router, h.admin, http.MethodPut, "/api/v1/admin/connectors/1", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	stored := h.repo.rows[id]
	if stored.Enabled {
		t.Fatal("enabled flag not flipped")
	}
	if !strings.Contains(string(stored.Config), "s3cr3t") {
		t.Fatalf("stored config = %s, want the secret preserved", stored.Config)
	}
}

func TestConnectorUpdateUnknownReturnsNotFound(t *testing.T) {
	h := setupConnectorAdmin(t)

	rec := doGroupRequest(t, h.router, h.admin, http.MethodPut, "/api/v1/admin/connectors/999",
		map[string]interface{}{"name": "x"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectorGetUnknownReturnsNotFound(t *testing.T) {
	h := setupConnectorAdmin(t)

	if rec := doGroupRequest(t, h.router, h.admin, http.MethodGet, "/api/v1/admin/connectors/999", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestConnectorDelete(t *testing.T) {
	h := setupConnectorAdmin(t)
	id := h.createConnector(t)

	if rec := doGroupRequest(t, h.router, h.admin, http.MethodDelete, "/api/v1/admin/connectors/1", nil); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if _, ok := h.repo.rows[id]; ok {
		t.Fatal("row still present after delete")
	}
	if rec := doGroupRequest(t, h.router, h.admin, http.MethodDelete, "/api/v1/admin/connectors/1", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: expected 404, got %d", rec.Code)
	}
}

// --- test-send ---

func TestConnectorTestSendSuccess(t *testing.T) {
	h := setupConnectorAdmin(t)
	h.createConnector(t)

	rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors/1/test", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != model.DeliverySent {
		t.Fatalf("status = %q, want sent", resp.Status)
	}
	if resp.Error != "" {
		t.Fatalf("error = %q, want none", resp.Error)
	}
}

// A destination refusing the test is an answer, not an API failure: the request
// succeeded and the UI needs the reason to show the admin.
func TestConnectorTestSendFailureIs200WithReason(t *testing.T) {
	h := setupConnectorAdmin(t)
	h.createConnector(t)
	h.hook.deliverErr = errors.New("webhook returned 500")

	rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors/1/test", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with a failure payload, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed", resp.Status)
	}
	if !strings.Contains(resp.Error, "webhook returned 500") {
		t.Fatalf("error = %q, want the reason surfaced", resp.Error)
	}
}

func TestConnectorTestSendUnknownReturnsNotFound(t *testing.T) {
	h := setupConnectorAdmin(t)

	if rec := doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors/999/test", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// --- deliveries ---

func TestConnectorDeliveriesPagination(t *testing.T) {
	h := setupConnectorAdmin(t)
	h.createConnector(t)
	for i := 0; i < 3; i++ {
		doGroupRequest(t, h.router, h.admin, http.MethodPost, "/api/v1/admin/connectors/1/test", nil)
	}

	rec := doGroupRequest(t, h.router, h.admin, http.MethodGet,
		"/api/v1/admin/connectors/1/deliveries?page=1&per_page=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Deliveries []map[string]interface{} `json:"deliveries"`
		Total      int64                    `json:"total"`
		Page       int                      `json:"page"`
		PerPage    int                      `json:"per_page"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 || len(resp.Deliveries) != 2 || resp.Page != 1 || resp.PerPage != 2 {
		t.Fatalf("response = %+v", resp)
	}
	if strings.Contains(rec.Body.String(), "s3cr3t") {
		t.Fatalf("delivery log leaked the secret: %s", rec.Body.String())
	}
}

func TestConnectorDeliveriesRejectsBadID(t *testing.T) {
	h := setupConnectorAdmin(t)

	if rec := doGroupRequest(t, h.router, h.admin, http.MethodGet, "/api/v1/admin/connectors/abc/deliveries", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestConnectorRejectsMalformedJSON(t *testing.T) {
	h := setupConnectorAdmin(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/connectors", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.admin)
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

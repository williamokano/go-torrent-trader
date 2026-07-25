package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- mock connector repository ---

type mockConnectorRepo struct {
	rows      map[int64]*model.NotificationConnector
	nextID    int64
	createErr error
}

func newMockConnectorRepo() *mockConnectorRepo {
	return &mockConnectorRepo{rows: map[int64]*model.NotificationConnector{}, nextID: 1}
}

func (r *mockConnectorRepo) Create(_ context.Context, c *model.NotificationConnector) error {
	if r.createErr != nil {
		return r.createErr
	}
	c.ID = r.nextID
	r.nextID++
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	stored := *c
	r.rows[c.ID] = &stored
	return nil
}

func (r *mockConnectorRepo) GetByID(_ context.Context, id int64) (*model.NotificationConnector, error) {
	c, ok := r.rows[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copied := *c
	return &copied, nil
}

func (r *mockConnectorRepo) List(_ context.Context) ([]model.NotificationConnector, error) {
	var out []model.NotificationConnector
	for id := int64(1); id < r.nextID; id++ {
		if c, ok := r.rows[id]; ok {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (r *mockConnectorRepo) ListEnabled(ctx context.Context) ([]model.NotificationConnector, error) {
	all, _ := r.List(ctx)
	var out []model.NotificationConnector
	for _, c := range all {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *mockConnectorRepo) Update(_ context.Context, c *model.NotificationConnector) error {
	if _, ok := r.rows[c.ID]; !ok {
		return sql.ErrNoRows
	}
	stored := *c
	stored.UpdatedAt = time.Now()
	r.rows[c.ID] = &stored
	return nil
}

func (r *mockConnectorRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.rows[id]; !ok {
		return sql.ErrNoRows
	}
	delete(r.rows, id)
	return nil
}

func (r *mockConnectorRepo) CountByKind(_ context.Context, kind string) (int64, error) {
	var n int64
	for _, c := range r.rows {
		if c.Kind == kind {
			n++
		}
	}
	return n, nil
}

// --- mock delivery repository ---

type mockDeliveryRepo struct {
	rows   map[int64]*model.ConnectorDelivery
	nextID int64
}

func newMockDeliveryRepo() *mockDeliveryRepo {
	return &mockDeliveryRepo{rows: map[int64]*model.ConnectorDelivery{}, nextID: 1}
}

func (r *mockDeliveryRepo) InsertPending(_ context.Context, d *model.ConnectorDelivery) (bool, error) {
	for _, existing := range r.rows {
		if existing.InstanceID == d.InstanceID && existing.EventKey == d.EventKey {
			return false, nil
		}
	}
	d.ID = r.nextID
	r.nextID++
	d.CreatedAt = time.Now()
	d.UpdatedAt = d.CreatedAt
	stored := *d
	r.rows[d.ID] = &stored
	return true, nil
}

func (r *mockDeliveryRepo) ListDue(_ context.Context, instanceID int64, now time.Time, limit int) ([]model.ConnectorDelivery, error) {
	var out []model.ConnectorDelivery
	for id := int64(1); id < r.nextID; id++ {
		d, ok := r.rows[id]
		if !ok || d.InstanceID != instanceID || d.Status != model.DeliveryPending {
			continue
		}
		if d.NextAttemptAt != nil && d.NextAttemptAt.After(now) {
			continue
		}
		out = append(out, *d)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *mockDeliveryRepo) ClaimForDelivery(_ context.Context, id int64, leaseUntil, now time.Time) (bool, error) {
	d, ok := r.rows[id]
	if !ok || d.Status != model.DeliveryPending {
		return false, nil
	}
	if d.NextAttemptAt != nil && d.NextAttemptAt.After(now) {
		return false, nil
	}
	d.NextAttemptAt = &leaseUntil
	return true, nil
}

func (r *mockDeliveryRepo) CountSentSince(_ context.Context, instanceID int64, since time.Time) (int64, error) {
	var n int64
	for _, d := range r.rows {
		if d.InstanceID != instanceID {
			continue
		}
		if d.Status == model.DeliverySent && !d.UpdatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (r *mockDeliveryRepo) MarkSent(_ context.Context, id int64, status string) error {
	d, ok := r.rows[id]
	if !ok {
		return sql.ErrNoRows
	}
	d.Status = status
	d.NextAttemptAt = nil
	d.UpdatedAt = time.Now()
	return nil
}

func (r *mockDeliveryRepo) MarkFailedAttempt(_ context.Context, id int64, attempts int, lastError string, next *time.Time) error {
	d, ok := r.rows[id]
	if !ok {
		return sql.ErrNoRows
	}
	d.Attempts = attempts
	msg := lastError
	d.LastError = &msg
	d.NextAttemptAt = next
	if next == nil {
		d.Status = model.DeliveryFailed
	} else {
		d.Status = model.DeliveryPending
	}
	d.UpdatedAt = time.Now()
	return nil
}

func (r *mockDeliveryRepo) ListByInstance(_ context.Context, instanceID int64, page, perPage int) ([]model.ConnectorDelivery, int64, error) {
	var all []model.ConnectorDelivery
	for id := int64(1); id < r.nextID; id++ {
		if d, ok := r.rows[id]; ok && d.InstanceID == instanceID {
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

func (r *mockDeliveryRepo) LatestStatusByInstance(_ context.Context) (map[int64]model.ConnectorDelivery, error) {
	latest := map[int64]model.ConnectorDelivery{}
	for id := int64(1); id < r.nextID; id++ {
		if d, ok := r.rows[id]; ok {
			latest[d.InstanceID] = *d
		}
	}
	return latest, nil
}

func (r *mockDeliveryRepo) InstancesWithDue(_ context.Context, now time.Time) ([]int64, error) {
	seen := map[int64]bool{}
	var out []int64
	for _, d := range r.rows {
		if d.Status != model.DeliveryPending {
			continue
		}
		if d.NextAttemptAt != nil && d.NextAttemptAt.After(now) {
			continue
		}
		if !seen[d.InstanceID] {
			seen[d.InstanceID] = true
			out = append(out, d.InstanceID)
		}
	}
	return out, nil
}

func (r *mockDeliveryRepo) DeleteOld(_ context.Context, cutoff time.Time) (int64, error) {
	var n int64
	for id, d := range r.rows {
		if d.CreatedAt.Before(cutoff) && d.Status != model.DeliveryPending {
			delete(r.rows, id)
			n++
		}
	}
	return n, nil
}

// --- fake connectors ---

type fakeConnector struct {
	kind       string
	singleton  bool
	secrets    []string
	validateFn func(json.RawMessage) error
	deliverFn  func(connector.Instance, connector.Announcement) error
	delivered  []connector.Announcement
}

func (f *fakeConnector) Kind() string           { return f.kind }
func (f *fakeConnector) Singleton() bool        { return f.singleton }
func (f *fakeConnector) SecretFields() []string { return f.secrets }

func (f *fakeConnector) ValidateConfig(cfg json.RawMessage) error {
	if f.validateFn != nil {
		return f.validateFn(cfg)
	}
	return nil
}

func (f *fakeConnector) Deliver(_ context.Context, inst connector.Instance, a connector.Announcement) error {
	f.delivered = append(f.delivered, a)
	if f.deliverFn != nil {
		return f.deliverFn(inst, a)
	}
	return nil
}

type connectorHarness struct {
	svc        *ConnectorService
	repo       *mockConnectorRepo
	deliveries *mockDeliveryRepo
	hook       *fakeConnector
	chat       *fakeConnector
}

func newConnectorHarness(t *testing.T) *connectorHarness {
	t.Helper()

	registry := connector.NewRegistry()
	hook := &fakeConnector{kind: "webhook", secrets: []string{"hmac_secret"}}
	chat := &fakeConnector{kind: "chat", singleton: true}
	registry.Register(hook)
	registry.Register(chat)

	repo := newMockConnectorRepo()
	deliveries := newMockDeliveryRepo()
	settings := NewSiteSettingsService(newMockSiteSettingsRepo(), event.NewInMemoryBus())

	return &connectorHarness{
		svc:        NewConnectorService(repo, deliveries, registry, settings, "https://tracker.test"),
		repo:       repo,
		deliveries: deliveries,
		hook:       hook,
		chat:       chat,
	}
}

func mustCreate(t *testing.T, h *connectorHarness, in ConnectorInput) *ConnectorView {
	t.Helper()
	view, err := h.svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return view
}

func webhookInput() ConnectorInput {
	return ConnectorInput{
		Kind:    "webhook",
		Name:    "My Hook",
		Enabled: true,
		Config:  json.RawMessage(`{"url":"https://example.test/hook","hmac_secret":"s3cr3t"}`),
		Filters: json.RawMessage(`{}`),
	}
}

func TestConnectorCreateRejectsUnknownKind(t *testing.T) {
	h := newConnectorHarness(t)

	_, err := h.svc.Create(context.Background(), ConnectorInput{Kind: "carrier-pigeon", Name: "x"})
	if !errors.Is(err, ErrConnectorUnknownKind) {
		t.Fatalf("err = %v, want ErrConnectorUnknownKind", err)
	}
}

func TestConnectorCreateRequiresName(t *testing.T) {
	h := newConnectorHarness(t)

	in := webhookInput()
	in.Name = "   "
	if _, err := h.svc.Create(context.Background(), in); !errors.Is(err, ErrConnectorInvalid) {
		t.Fatalf("err = %v, want ErrConnectorInvalid", err)
	}
}

// chat and sse announce to one shared destination, so a second instance would
// double-post to it.
func TestConnectorCreateRejectsSecondSingleton(t *testing.T) {
	h := newConnectorHarness(t)

	mustCreate(t, h, ConnectorInput{Kind: "chat", Name: "Shoutbox", Config: json.RawMessage(`{}`)})

	_, err := h.svc.Create(context.Background(), ConnectorInput{Kind: "chat", Name: "Shoutbox 2", Config: json.RawMessage(`{}`)})
	if !errors.Is(err, ErrConnectorSingleton) {
		t.Fatalf("err = %v, want ErrConnectorSingleton", err)
	}
}

func TestConnectorCreateRejectsInvalidConfig(t *testing.T) {
	h := newConnectorHarness(t)
	h.hook.validateFn = func(json.RawMessage) error { return errors.New("url is required") }

	in := webhookInput()
	in.Config = json.RawMessage(`{}`)
	if _, err := h.svc.Create(context.Background(), in); !errors.Is(err, ErrConnectorInvalid) {
		t.Fatalf("err = %v, want ErrConnectorInvalid", err)
	}
}

func TestConnectorCreateRejectsInvalidFilters(t *testing.T) {
	h := newConnectorHarness(t)

	in := webhookInput()
	in.Filters = json.RawMessage(`{"catgeory_ids":[1]}`)
	if _, err := h.svc.Create(context.Background(), in); !errors.Is(err, ErrConnectorInvalid) {
		t.Fatalf("err = %v, want ErrConnectorInvalid", err)
	}
}

// The whole write-only contract in one assertion: whatever the API hands back,
// the credential is not in it.
func TestConnectorViewsNeverContainTheSecret(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	fetched, err := h.svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	list, err := h.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	for name, view := range map[string]any{"create": created, "get": fetched, "list": list} {
		encoded, err := json.Marshal(view)
		if err != nil {
			t.Fatalf("marshal %s view: %v", name, err)
		}
		if strings.Contains(string(encoded), "s3cr3t") {
			t.Fatalf("%s response contains the secret: %s", name, encoded)
		}
		if strings.Contains(string(encoded), "hmac_secret\":\"") {
			t.Fatalf("%s response contains the secret key with a value: %s", name, encoded)
		}
	}

	if len(fetched.SecretsSet) != 1 || fetched.SecretsSet[0] != "hmac_secret" {
		t.Fatalf("secrets_set = %v, want [hmac_secret] so the UI can show the 'set' state", fetched.SecretsSet)
	}
}

// The edit form never receives the secret, so it submits nothing for it. That
// must not wipe the stored credential.
func TestConnectorUpdateWithOmittedSecretKeepsStoredValue(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	_, err := h.svc.Update(context.Background(), created.ID, ConnectorInput{
		Name:    "Renamed Hook",
		Enabled: false,
		Config:  json.RawMessage(`{"url":"https://example.test/other"}`),
		Filters: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored := h.repo.rows[created.ID]
	if !strings.Contains(string(stored.Config), `"hmac_secret":"s3cr3t"`) {
		t.Fatalf("stored config = %s, want the secret preserved", stored.Config)
	}
	if !strings.Contains(string(stored.Config), "https://example.test/other") {
		t.Fatalf("stored config = %s, want the updated url", stored.Config)
	}
	if stored.Name != "Renamed Hook" || stored.Enabled {
		t.Fatalf("stored = %+v, want the name and enabled flag updated", stored)
	}
}

func TestConnectorUpdateExplicitNullClearsSecret(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	_, err := h.svc.Update(context.Background(), created.ID, ConnectorInput{
		Name:   "My Hook",
		Config: json.RawMessage(`{"url":"https://example.test/hook","hmac_secret":null}`),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored := h.repo.rows[created.ID]
	if strings.Contains(string(stored.Config), "s3cr3t") {
		t.Fatalf("stored config = %s, want the secret cleared", stored.Config)
	}
}

// Changing the kind would reinterpret the stored config under a different
// connector — e.g. a webhook URL suddenly read as an IRC server.
func TestConnectorUpdateCannotChangeKind(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	_, err := h.svc.Update(context.Background(), created.ID, ConnectorInput{Kind: "chat", Name: "My Hook"})
	if !errors.Is(err, ErrConnectorKindImmutable) {
		t.Fatalf("err = %v, want ErrConnectorKindImmutable", err)
	}
}

func TestConnectorUpdateUnknownIDIsNotFound(t *testing.T) {
	h := newConnectorHarness(t)

	if _, err := h.svc.Update(context.Background(), 999, ConnectorInput{Name: "x"}); !errors.Is(err, ErrConnectorNotFound) {
		t.Fatalf("err = %v, want ErrConnectorNotFound", err)
	}
	if _, err := h.svc.Get(context.Background(), 999); !errors.Is(err, ErrConnectorNotFound) {
		t.Fatalf("err = %v, want ErrConnectorNotFound", err)
	}
	if err := h.svc.Delete(context.Background(), 999); !errors.Is(err, ErrConnectorNotFound) {
		t.Fatalf("err = %v, want ErrConnectorNotFound", err)
	}
}

func TestConnectorDelete(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	if err := h.svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := h.repo.rows[created.ID]; ok {
		t.Fatal("row still present after delete")
	}
}

func TestConnectorListIncludesLastDeliveryAndKinds(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	if _, err := h.svc.TestSend(context.Background(), created.ID); err != nil {
		t.Fatalf("TestSend: %v", err)
	}

	views, err := h.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("got %d views, want 1", len(views))
	}
	if views[0].LastDelivery == nil {
		t.Fatal("list view must carry the latest delivery for the status chip")
	}
	if views[0].LastDelivery.Status != model.DeliverySent {
		t.Fatalf("last delivery status = %q, want sent", views[0].LastDelivery.Status)
	}

	kinds := h.svc.Kinds()
	if len(kinds) != 2 || kinds[0] != "chat" || kinds[1] != "webhook" {
		t.Fatalf("Kinds() = %v, want [chat webhook]", kinds)
	}
}

func TestTestSendRecordsSuccess(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	result, err := h.svc.TestSend(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TestSend: %v", err)
	}
	if result.Status != model.DeliverySent {
		t.Fatalf("status = %q, want sent", result.Status)
	}
	if !strings.HasPrefix(result.EventKey, "test:") {
		t.Fatalf("event key = %q, want a test: prefix so it never collides with a real event", result.EventKey)
	}
	if len(h.hook.delivered) != 1 || h.hook.delivered[0].Event != connector.EventTest {
		t.Fatalf("delivered = %+v, want one test announcement", h.hook.delivered)
	}
	if !strings.Contains(h.hook.delivered[0].URL, "https://tracker.test/") {
		t.Fatalf("sample URL = %q, want it to point at this site", h.hook.delivered[0].URL)
	}
}

func TestTestSendRecordsFailureWithRedactedError(t *testing.T) {
	h := newConnectorHarness(t)
	h.hook.deliverFn = func(connector.Instance, connector.Announcement) error {
		return errors.New("auth rejected for secret s3cr3t")
	}
	created := mustCreate(t, h, webhookInput())

	result, err := h.svc.TestSend(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TestSend must report the failure in its result, not as an error: %v", err)
	}
	if result.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if strings.Contains(result.LastError, "s3cr3t") {
		t.Fatalf("recorded error leaked the secret: %s", result.LastError)
	}
	if !strings.Contains(result.LastError, connector.Redacted) {
		t.Fatalf("recorded error = %q, want the secret replaced with a marker", result.LastError)
	}
}

// A panicking connector must not take down the admin's request.
func TestTestSendSurvivesPanickingConnector(t *testing.T) {
	h := newConnectorHarness(t)
	h.hook.deliverFn = func(connector.Instance, connector.Announcement) error {
		panic("boom")
	}
	created := mustCreate(t, h, webhookInput())

	result, err := h.svc.TestSend(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TestSend: %v", err)
	}
	if result.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
}

// Testing before turning an instance on is the normal setup flow, so a disabled
// instance must still be testable.
func TestTestSendWorksOnDisabledInstance(t *testing.T) {
	h := newConnectorHarness(t)
	in := webhookInput()
	in.Enabled = false
	created := mustCreate(t, h, in)

	result, err := h.svc.TestSend(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TestSend: %v", err)
	}
	if result.Status != model.DeliverySent {
		t.Fatalf("status = %q, want sent", result.Status)
	}
}

func TestTestSendKeysAreUniquePerRun(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	first, err := h.svc.TestSend(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TestSend: %v", err)
	}
	second, err := h.svc.TestSend(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TestSend: %v", err)
	}
	if first.EventKey == second.EventKey {
		t.Fatal("repeated test-sends must not share a dedupe key, or the second would be swallowed")
	}
	if first.ID == second.ID {
		t.Fatal("each test-send must record its own delivery row")
	}
}

func TestConnectorDeliveriesPagination(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	for i := 0; i < 3; i++ {
		if _, err := h.svc.TestSend(context.Background(), created.ID); err != nil {
			t.Fatalf("TestSend: %v", err)
		}
	}

	rows, total, err := h.svc.Deliveries(context.Background(), created.ID, 1, 2)
	if err != nil {
		t.Fatalf("Deliveries: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	if _, _, err := h.svc.Deliveries(context.Background(), 999, 1, 10); !errors.Is(err, ErrConnectorNotFound) {
		t.Fatalf("err = %v, want ErrConnectorNotFound", err)
	}
}

// A delivery payload is stored and shown in the admin log, so it must be free of
// secrets even when the instance has one.
func TestStoredTestPayloadHasNoSecret(t *testing.T) {
	h := newConnectorHarness(t)
	created := mustCreate(t, h, webhookInput())

	if _, err := h.svc.TestSend(context.Background(), created.ID); err != nil {
		t.Fatalf("TestSend: %v", err)
	}

	for _, row := range h.deliveries.rows {
		if strings.Contains(string(row.Payload), "s3cr3t") {
			t.Fatalf("delivery payload contains the secret: %s", row.Payload)
		}
	}
}

package listener

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mocks ---

type mockConnectorRepo struct {
	rows []model.NotificationConnector
	err  error
}

func (r *mockConnectorRepo) Create(context.Context, *model.NotificationConnector) error { return nil }
func (r *mockConnectorRepo) GetByID(context.Context, int64) (*model.NotificationConnector, error) {
	return nil, sql.ErrNoRows
}
func (r *mockConnectorRepo) List(context.Context) ([]model.NotificationConnector, error) {
	return r.rows, r.err
}
func (r *mockConnectorRepo) ListEnabled(context.Context) ([]model.NotificationConnector, error) {
	if r.err != nil {
		return nil, r.err
	}
	var out []model.NotificationConnector
	for _, c := range r.rows {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out, nil
}
func (r *mockConnectorRepo) Update(context.Context, *model.NotificationConnector) error { return nil }
func (r *mockConnectorRepo) Delete(context.Context, int64) error                        { return nil }
func (r *mockConnectorRepo) CountByKind(context.Context, string) (int64, error)         { return 0, nil }

type insertCall struct {
	instanceID int64
	eventKey   string
	payload    json.RawMessage
	status     string
	lastError  *string
}

type mockDeliveryRepo struct {
	mu        sync.Mutex
	inserts   []insertCall
	seen      map[string]bool
	insertErr error
}

func newMockDeliveryRepo() *mockDeliveryRepo {
	return &mockDeliveryRepo{seen: map[string]bool{}}
}

func (r *mockDeliveryRepo) InsertPending(_ context.Context, d *model.ConnectorDelivery) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.insertErr != nil {
		return false, r.insertErr
	}
	key := fmt.Sprintf("%d@%s", d.InstanceID, d.EventKey)
	if r.seen[key] {
		return false, nil
	}
	r.seen[key] = true
	r.inserts = append(r.inserts, insertCall{
		instanceID: d.InstanceID, eventKey: d.EventKey, payload: d.Payload,
		status: d.Status, lastError: d.LastError,
	})
	return true, nil
}

func (r *mockDeliveryRepo) ListDue(context.Context, int64, time.Time, int) ([]model.ConnectorDelivery, error) {
	return nil, nil
}
func (r *mockDeliveryRepo) ClaimForDelivery(context.Context, int64, time.Time, time.Time) (bool, error) {
	return true, nil
}
func (r *mockDeliveryRepo) CountSentSince(context.Context, int64, time.Time) (int64, error) {
	return 0, nil
}
func (r *mockDeliveryRepo) MarkSent(context.Context, int64, string) error { return nil }
func (r *mockDeliveryRepo) MarkFailedAttempt(context.Context, int64, int, string, *time.Time) error {
	return nil
}
func (r *mockDeliveryRepo) ListByInstance(context.Context, int64, int, int) ([]model.ConnectorDelivery, int64, error) {
	return nil, 0, nil
}
func (r *mockDeliveryRepo) LatestStatusByInstance(context.Context) (map[int64]model.ConnectorDelivery, error) {
	return nil, nil
}
func (r *mockDeliveryRepo) InstancesWithDue(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (r *mockDeliveryRepo) DeleteOld(context.Context, time.Time) (int64, error) { return 0, nil }

type recordingEnqueuer struct {
	mu    sync.Mutex
	calls []int64
	err   error
}

func (e *recordingEnqueuer) EnqueueConnectorDrain(_ context.Context, instanceID int64, _ time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	e.calls = append(e.calls, instanceID)
	return nil
}

type mockSettingsRepo struct{ values map[string]string }

func (m *mockSettingsRepo) Get(_ context.Context, key string) (*model.SiteSetting, error) {
	v, ok := m.values[key]
	if !ok {
		return nil, nil
	}
	return &model.SiteSetting{Key: key, Value: v}, nil
}
func (m *mockSettingsRepo) GetAll(context.Context) ([]model.SiteSetting, error) { return nil, nil }
func (m *mockSettingsRepo) Set(_ context.Context, key, value string) error {
	m.values[key] = value
	return nil
}

// --- harness ---

type dispatchHarness struct {
	bus        event.Bus
	connectors *mockConnectorRepo
	deliveries *mockDeliveryRepo
	enqueuer   *recordingEnqueuer
	settings   *mockSettingsRepo
	// categoryPaths overrides the ancestor chain a category resolves to;
	// categoryPathErr makes resolution fail outright.
	categoryPaths   map[int64][]int64
	categoryPathErr error
	// categoryPathCalls counts resolver invocations, so a test can assert the
	// approve path does no category work when nothing filters on category.
	categoryPathCalls int
}

func newDispatchHarness(t *testing.T, rows []model.NotificationConnector) *dispatchHarness {
	t.Helper()

	h := &dispatchHarness{
		bus:        event.NewInMemoryBus(),
		connectors: &mockConnectorRepo{rows: rows},
		deliveries: newMockDeliveryRepo(),
		enqueuer:   &recordingEnqueuer{},
		settings:   &mockSettingsRepo{values: map[string]string{}},
	}
	RegisterConnectorDispatcher(h.bus, h.connectors, h.deliveries,
		service.NewSiteSettingsService(h.settings, h.bus), h.enqueuer, "https://tracker.test",
		func(_ context.Context, categoryID int64) ([]int64, error) {
			h.categoryPathCalls++
			if h.categoryPathErr != nil {
				return nil, h.categoryPathErr
			}
			if path, ok := h.categoryPaths[categoryID]; ok {
				return path, nil
			}
			return []int64{categoryID}, nil
		})
	return h
}

// newDispatchHarnessWithoutResolver wires the dispatcher the way a bad refactor
// of main.go would: no category resolver at all.
func newDispatchHarnessWithoutResolver(t *testing.T, rows []model.NotificationConnector) *dispatchHarness {
	t.Helper()

	h := &dispatchHarness{
		bus:        event.NewInMemoryBus(),
		connectors: &mockConnectorRepo{rows: rows},
		deliveries: newMockDeliveryRepo(),
		enqueuer:   &recordingEnqueuer{},
		settings:   &mockSettingsRepo{values: map[string]string{}},
	}
	RegisterConnectorDispatcher(h.bus, h.connectors, h.deliveries,
		service.NewSiteSettingsService(h.settings, h.bus), h.enqueuer, "https://tracker.test", nil)
	return h
}

func publishedEvent() *event.TorrentPublishedEvent {
	return &event.TorrentPublishedEvent{
		Base:         event.NewBase(event.TorrentPublished, event.Actor{ID: 9, Username: "mod"}),
		TorrentID:    42,
		Name:         "Some.Release-GROUP",
		InfoHashHex:  "deadbeef",
		CategoryID:   3,
		CategoryName: "Movies",
		Size:         2 * 1024 * 1024 * 1024,
		FileCount:    1,
		UploaderID:   5,
		UploaderName: "alice",
		PublishedAt:  time.Now(),
	}
}

func instance(id int64, kind string, enabled bool, filters string) model.NotificationConnector {
	return model.NotificationConnector{
		ID:      id,
		Kind:    kind,
		Name:    kind,
		Enabled: enabled,
		Config:  json.RawMessage(`{}`),
		Filters: json.RawMessage(filters),
	}
}

// --- tests ---

func TestDispatchRecordsOneDeliveryPerEnabledInstance(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{
		instance(1, "chat", true, `{}`),
		instance(2, "webhook", true, `{}`),
		instance(3, "webhook", false, `{}`), // disabled
	})

	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 2 {
		t.Fatalf("recorded %d deliveries, want 2 (disabled instances are skipped)", len(h.deliveries.inserts))
	}
	for _, ins := range h.deliveries.inserts {
		if ins.eventKey != "torrent.published:42" {
			t.Fatalf("event key = %q, want torrent.published:42", ins.eventKey)
		}
	}
	if len(h.enqueuer.calls) != 2 {
		t.Fatalf("enqueued %d drains, want 2", len(h.enqueuer.calls))
	}
}

func TestDispatchStoresRenderedAnnouncementPayload(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{instance(1, "webhook", true, `{}`)})

	h.bus.Publish(context.Background(), publishedEvent())

	var a connector.Announcement
	if err := json.Unmarshal(h.deliveries.inserts[0].payload, &a); err != nil {
		t.Fatalf("stored payload is not an Announcement: %v", err)
	}
	if a.Event != connector.EventTorrentPublished {
		t.Fatalf("event = %q, want %q", a.Event, connector.EventTorrentPublished)
	}
	if a.URL != "https://tracker.test/torrent/42" {
		t.Fatalf("url = %q, want the site link", a.URL)
	}
	if a.Uploader != "alice" {
		t.Fatalf("uploader = %q, want alice", a.Uploader)
	}
	if a.Body == "" || a.Title == "" {
		t.Fatal("payload must arrive pre-rendered so a connector with no template still says something")
	}
}

// The stored payload is the source for every connector's output, so this is
// where the anonymity guarantee has to hold.
func TestDispatchAnonymousPayloadNeverNamesTheUploader(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{instance(1, "webhook", true, `{}`)})

	e := publishedEvent()
	e.Anonymous = true
	e.UploaderName = "" // the service drops it at the source
	h.bus.Publish(context.Background(), e)

	payload := string(h.deliveries.inserts[0].payload)
	if strings.Contains(payload, "alice") {
		t.Fatalf("payload leaked the real uploader: %s", payload)
	}
	if !strings.Contains(payload, `"uploader":"Anonymous"`) {
		t.Fatalf("payload = %s, want uploader Anonymous", payload)
	}
}

func TestDispatchAppliesInstanceFilters(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{
		instance(1, "webhook", true, `{"category_ids":[3]}`),  // matches
		instance(2, "webhook", true, `{"category_ids":[99]}`), // does not
		instance(3, "webhook", true, `{"min_size":999999999999}`),
	})

	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 1 || h.deliveries.inserts[0].instanceID != 1 {
		t.Fatalf("inserts = %+v, want only instance 1", h.deliveries.inserts)
	}
}

// One switch that silences everything, without touching per-instance state.
func TestDispatchKillSwitchSilencesEverything(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{
		instance(1, "chat", true, `{}`),
		instance(2, "webhook", true, `{}`),
	})
	h.settings.values[service.SettingConnectorsEnabled] = "false"

	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 0 {
		t.Fatalf("recorded %d deliveries with the kill-switch off, want 0", len(h.deliveries.inserts))
	}
	if len(h.enqueuer.calls) != 0 {
		t.Fatalf("enqueued %d drains with the kill-switch off, want 0", len(h.enqueuer.calls))
	}
}

func TestDispatchDefaultsToEnabledWhenSettingIsMissing(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{instance(1, "webhook", true, `{}`)})

	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 1 {
		t.Fatal("connectors must default to enabled when the setting has never been written")
	}
}

// A duplicate dispatch is caught by the unique index, and nothing is queued for
// it — that is what stops a double approve becoming a double announcement.
func TestDispatchDuplicateDoesNotEnqueueAgain(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{instance(1, "webhook", true, `{}`)})

	h.bus.Publish(context.Background(), publishedEvent())
	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 1 {
		t.Fatalf("recorded %d deliveries, want 1", len(h.deliveries.inserts))
	}
	if len(h.enqueuer.calls) != 1 {
		t.Fatalf("enqueued %d drains, want 1", len(h.enqueuer.calls))
	}
}

// A malformed filters column on one row must not silence the others.
func TestDispatchBadFiltersSkipsOnlyThatInstance(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{
		instance(1, "webhook", true, `{"nonsense_key":1}`),
		instance(2, "webhook", true, `{}`),
	})

	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 1 || h.deliveries.inserts[0].instanceID != 2 {
		t.Fatalf("inserts = %+v, want only instance 2", h.deliveries.inserts)
	}
}

// The delivery row is already safely persisted, so a Redis blip delays delivery
// (the maintenance sweep re-enqueues) rather than losing it — and must not stop
// the loop over the other instances.
func TestDispatchEnqueueFailureStillRecordsEveryDelivery(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{
		instance(1, "webhook", true, `{}`),
		instance(2, "chat", true, `{}`),
	})
	h.enqueuer.err = errors.New("redis down")

	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 2 {
		t.Fatalf("recorded %d deliveries, want 2 despite the enqueue failure", len(h.deliveries.inserts))
	}
}

func TestDispatchInsertFailureDoesNotStopOtherInstances(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{instance(1, "webhook", true, `{}`)})
	h.deliveries.insertErr = errors.New("database down")

	// The event bus must not propagate this: the approve request that triggered
	// it has to succeed regardless.
	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.enqueuer.calls) != 0 {
		t.Fatal("nothing should be queued when the delivery row could not be written")
	}
}

func TestDispatchSurvivesRepositoryFailure(t *testing.T) {
	h := newDispatchHarness(t, nil)
	h.connectors.err = errors.New("database down")

	h.bus.Publish(context.Background(), publishedEvent())

	if len(h.deliveries.inserts) != 0 {
		t.Fatal("no deliveries expected when the connector list could not be read")
	}
}

func TestAnnouncementURLHandlesTrailingSlashAndEmptyBase(t *testing.T) {
	e := publishedEvent()

	if got := AnnouncementFromPublished(e, "https://tracker.test/").URL; got != "https://tracker.test/torrent/42" {
		t.Fatalf("url = %q, want no doubled slash", got)
	}
	if got := AnnouncementFromPublished(e, "").URL; got != "" {
		t.Fatalf("url = %q, want empty when no base URL is configured", got)
	}
}

func TestDispatchPutsTheCategoryPathOnThePayload(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{
		{ID: 1, Kind: "chat", Enabled: true},
	})
	h.categoryPaths = map[int64][]int64{3: {1, 2, 3}}

	h.bus.Publish(context.Background(), publishedEvent())

	rows := h.deliveries.inserts
	if len(rows) != 1 {
		t.Fatalf("got %d delivery rows, want 1", len(rows))
	}
	var a connector.Announcement
	if err := json.Unmarshal(rows[0].payload, &a); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(a.CategoryPath) != 3 || a.CategoryPath[0] != 1 || a.CategoryPath[2] != 3 {
		t.Fatalf("category path = %v, want [1 2 3]", a.CategoryPath)
	}
}

func TestDispatchDeliversToAnInstanceFilteredOnAParentCategory(t *testing.T) {
	// The torrent is in category 3, whose parent is 1. An instance including
	// category 1 must receive it — that is what makes a subtree filter work.
	h := newDispatchHarness(t, []model.NotificationConnector{
		{ID: 1, Kind: "chat", Enabled: true, Filters: json.RawMessage(`{"category_ids":[1]}`)},
	})
	h.categoryPaths = map[int64][]int64{3: {1, 3}}

	h.bus.Publish(context.Background(), publishedEvent())

	if got := len(h.deliveries.inserts); got != 1 {
		t.Fatalf("got %d delivery rows, want 1 for the parent-category filter", got)
	}
}

func TestDispatchExcludesASubcategoryOfAnExcludedParent(t *testing.T) {
	h := newDispatchHarness(t, []model.NotificationConnector{
		{ID: 1, Kind: "chat", Enabled: true,
			Filters: json.RawMessage(`{"category_ids":[1],"category_mode":"exclude"}`)},
	})
	h.categoryPaths = map[int64][]int64{3: {1, 3}}

	h.bus.Publish(context.Background(), publishedEvent())

	if got := len(h.deliveries.inserts); got != 0 {
		t.Fatalf("got %d delivery rows, want none: the parent category is excluded", got)
	}
}

func TestDispatchWithholdsFromAnExcludeFilterWhenThePathIsUnknown(t *testing.T) {
	// Failing open here would leak a torrent into the very feed configured to
	// keep its category out. The withheld instance still gets a row — a failed
	// one, carrying the reason — because an announcement that silently never
	// happened is the one thing the delivery log must not hide.
	h := newDispatchHarness(t, []model.NotificationConnector{
		{ID: 1, Kind: "chat", Enabled: true,
			Filters: json.RawMessage(`{"category_ids":[1],"category_mode":"exclude"}`)},
		{ID: 2, Kind: "webhook", Enabled: true},
	})
	h.categoryPathErr = errors.New("database is down")

	h.bus.Publish(context.Background(), publishedEvent())

	byInstance := map[int64]insertCall{}
	for _, row := range h.deliveries.inserts {
		byInstance[row.instanceID] = row
	}
	withheld, recorded := byInstance[1]
	if !recorded {
		t.Fatal("the withheld instance must still get a delivery row, or nothing shows in the log")
	}
	if withheld.status != model.DeliveryFailed {
		t.Fatalf("withheld row status = %q, want %q", withheld.status, model.DeliveryFailed)
	}
	if withheld.lastError == nil || *withheld.lastError == "" {
		t.Fatal("the withheld row must carry a reason an admin can read")
	}
	if delivered, ok := byInstance[2]; !ok || delivered.status != model.DeliveryPending {
		t.Fatal("the unfiltered instance must still be delivered to normally")
	}
	if len(h.enqueuer.calls) != 1 || h.enqueuer.calls[0] != 2 {
		t.Fatalf("enqueued drains %v, want only instance 2: a withheld row has nothing to drain",
			h.enqueuer.calls)
	}
}

func TestDispatchStillDeliversToAnIncludeFilterWhenThePathIsUnknown(t *testing.T) {
	// An include filter on the leaf fallback can only ever under-match, never
	// over-deliver, so withholding would lose announcements for no safety gain.
	h := newDispatchHarness(t, []model.NotificationConnector{
		{ID: 1, Kind: "chat", Enabled: true, Filters: json.RawMessage(`{"category_ids":[3]}`)},
	})
	h.categoryPathErr = errors.New("database is down")

	h.bus.Publish(context.Background(), publishedEvent())

	rows := h.deliveries.inserts
	if len(rows) != 1 || rows[0].status != model.DeliveryPending {
		t.Fatalf("include-filtered instance must still be delivered to, got %+v", rows)
	}
}

func TestDispatchResolvesTheCategoryPathOncePerEvent(t *testing.T) {
	// Once, not once per instance — this runs inside the approve request.
	h := newDispatchHarness(t, []model.NotificationConnector{
		{ID: 1, Kind: "chat", Enabled: true},
		{ID: 2, Kind: "webhook", Enabled: true, Filters: json.RawMessage(`{"min_size":1}`)},
		{ID: 3, Kind: "irc", Enabled: true},
	})

	h.bus.Publish(context.Background(), publishedEvent())

	if h.categoryPathCalls != 1 {
		t.Fatalf("resolved the category path %d times, want 1", h.categoryPathCalls)
	}
	if len(h.deliveries.inserts) != 3 {
		t.Fatalf("got %d delivery rows, want 3", len(h.deliveries.inserts))
	}
}

func TestDispatchResolvesNothingWhenNoInstanceIsEnabled(t *testing.T) {
	// A site with connectors switched off pays nothing for the feature.
	h := newDispatchHarness(t, nil)

	h.bus.Publish(context.Background(), publishedEvent())

	if h.categoryPathCalls != 0 {
		t.Fatalf("resolved the category path %d times with no instances, want 0", h.categoryPathCalls)
	}
}

func TestDispatchWithholdsWhenNoResolverIsWired(t *testing.T) {
	// A nil resolver reported as "path known" would downgrade every exclude
	// filter to leaf-only matching, permanently and silently.
	h := newDispatchHarnessWithoutResolver(t, []model.NotificationConnector{
		{ID: 1, Kind: "chat", Enabled: true,
			Filters: json.RawMessage(`{"category_ids":[1],"category_mode":"exclude"}`)},
	})

	h.bus.Publish(context.Background(), publishedEvent())

	rows := h.deliveries.inserts
	if len(rows) != 1 || rows[0].status != model.DeliveryFailed {
		t.Fatalf("expected one withheld row with no resolver wired, got %+v", rows)
	}
}

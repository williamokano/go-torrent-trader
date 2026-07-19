package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock notification repo (digest-focused) --------------------------------

// digestNotifRepo implements repository.NotificationRepository. Only
// CountUnreadSince/ListUnreadSince carry behaviour for these tests; the rest
// are no-ops so the mock satisfies the full interface.
type digestNotifRepo struct {
	mu          sync.Mutex
	unread      map[int64]int
	items       map[int64][]model.Notification
	countErrFor map[int64]error
}

func newDigestNotifRepo() *digestNotifRepo {
	return &digestNotifRepo{
		unread:      map[int64]int{},
		items:       map[int64][]model.Notification{},
		countErrFor: map[int64]error{},
	}
}

func (m *digestNotifRepo) Create(_ context.Context, _ *model.Notification) error { return nil }
func (m *digestNotifRepo) GetByID(_ context.Context, _ int64) (*model.Notification, error) {
	return nil, nil
}
func (m *digestNotifRepo) List(_ context.Context, _ int64, _ repository.ListNotificationsOptions) ([]model.Notification, int64, error) {
	return nil, 0, nil
}
func (m *digestNotifRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (m *digestNotifRepo) MarkAllRead(_ context.Context, _ int64) error { return nil }
func (m *digestNotifRepo) MarkTopicReplyGroupRead(_ context.Context, _, _ int64) (int64, error) {
	return 0, nil
}
func (m *digestNotifRepo) CountUnread(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *digestNotifRepo) DeleteOld(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *digestNotifRepo) CountUnreadSince(_ context.Context, userID int64, _ time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.countErrFor[userID]; err != nil {
		return 0, err
	}
	return m.unread[userID], nil
}

func (m *digestNotifRepo) ListUnreadSince(_ context.Context, userID int64, _ time.Time, limit int) ([]model.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := m.items[userID]
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// --- mock digest preference repo ---------------------------------------------

type listDueCall struct {
	frequency  string
	sentBefore time.Time
}

type digestPrefRepo struct {
	mu           sync.Mutex
	due          map[string][]model.DigestRecipient
	listDueCalls []listDueCall
	listDueErr   map[string]error // per-frequency error injection
	markedSent   []int64
	markSentErr  error
}

func newDigestPrefRepo() *digestPrefRepo {
	return &digestPrefRepo{due: map[string][]model.DigestRecipient{}, listDueErr: map[string]error{}}
}

func (m *digestPrefRepo) GetFrequency(_ context.Context, _ int64) (string, error) {
	return model.DigestOff, nil
}
func (m *digestPrefRepo) SetFrequency(_ context.Context, _ int64, _ string) error { return nil }

func (m *digestPrefRepo) ListDue(_ context.Context, frequency string, sentBefore time.Time) ([]model.DigestRecipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listDueCalls = append(m.listDueCalls, listDueCall{frequency: frequency, sentBefore: sentBefore})
	if err := m.listDueErr[frequency]; err != nil {
		return nil, err
	}
	return m.due[frequency], nil
}

func (m *digestPrefRepo) MarkSent(_ context.Context, userID int64, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markSentErr != nil {
		return m.markSentErr
	}
	m.markedSent = append(m.markedSent, userID)
	return nil
}

// --- mock task enqueuer -------------------------------------------------------

type sentEmail struct {
	to, subject, body string
}

type mockEnqueuer struct {
	mu   sync.Mutex
	sent []sentEmail
	err  error
}

func (m *mockEnqueuer) EnqueueSendEmail(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentEmail{to: to, subject: subject, body: body})
	return nil
}

// --- tests --------------------------------------------------------------------

func TestNotificationDigestService_Run_SendsEmailAndMarksSent(t *testing.T) {
	ctx := context.Background()
	notifRepo := newDigestNotifRepo()
	prefRepo := newDigestPrefRepo()
	enqueuer := &mockEnqueuer{}

	prefRepo.due[model.DigestDaily] = []model.DigestRecipient{{UserID: 1, Email: "alice@example.com"}}
	notifRepo.unread[1] = 3
	notifRepo.items[1] = []model.Notification{
		{ID: 1, UserID: 1, Type: model.NotifMention, Data: json.RawMessage(`{}`), CreatedAt: time.Now()},
		{ID: 2, UserID: 1, Type: model.NotifMention, Data: json.RawMessage(`{}`), CreatedAt: time.Now()},
		{ID: 3, UserID: 1, Type: model.NotifForumReply, Data: json.RawMessage(`{}`), CreatedAt: time.Now()},
	}

	svc := service.NewNotificationDigestService(prefRepo, notifRepo, enqueuer, "https://example.com")
	summary, err := svc.Run(ctx, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Recipients != 1 || summary.Emailed != 1 {
		t.Errorf("summary = %+v, want 1 recipient / 1 emailed", summary)
	}
	if len(enqueuer.sent) != 1 {
		t.Fatalf("expected 1 email enqueued, got %d", len(enqueuer.sent))
	}
	email := enqueuer.sent[0]
	if email.to != "alice@example.com" {
		t.Errorf("to = %q, want alice@example.com", email.to)
	}
	if !strings.Contains(email.subject, "3 unread") {
		t.Errorf("subject = %q, want it to mention 3 unread", email.subject)
	}
	if !strings.Contains(email.body, "2 mentions") {
		t.Errorf("body = %q, want a mentions breakdown", email.body)
	}
	if !strings.Contains(email.body, "1 forum reply") {
		t.Errorf("body = %q, want a forum reply breakdown", email.body)
	}
	if !strings.Contains(email.body, "https://example.com/notifications") {
		t.Errorf("body = %q, want a link back to the site", email.body)
	}
	if len(prefRepo.markedSent) != 1 || prefRepo.markedSent[0] != 1 {
		t.Errorf("markedSent = %v, want [1]", prefRepo.markedSent)
	}
}

func TestNotificationDigestService_Run_SkipsRecipientWithNothingUnread(t *testing.T) {
	ctx := context.Background()
	notifRepo := newDigestNotifRepo()
	prefRepo := newDigestPrefRepo()
	enqueuer := &mockEnqueuer{}

	prefRepo.due[model.DigestDaily] = []model.DigestRecipient{{UserID: 1, Email: "alice@example.com"}}
	// unread[1] defaults to 0.

	svc := service.NewNotificationDigestService(prefRepo, notifRepo, enqueuer, "https://example.com")
	summary, err := svc.Run(ctx, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Recipients != 1 || summary.Emailed != 0 {
		t.Errorf("summary = %+v, want 1 recipient / 0 emailed", summary)
	}
	if len(enqueuer.sent) != 0 {
		t.Errorf("expected no email, got %d", len(enqueuer.sent))
	}
	// The cursor must still advance so the window doesn't grow unbounded.
	if len(prefRepo.markedSent) != 1 {
		t.Errorf("expected MarkSent to be called even with nothing to report, got %v", prefRepo.markedSent)
	}
}

func TestNotificationDigestService_Run_SkipsRecipientWithNoEmail(t *testing.T) {
	ctx := context.Background()
	notifRepo := newDigestNotifRepo()
	prefRepo := newDigestPrefRepo()
	enqueuer := &mockEnqueuer{}

	prefRepo.due[model.DigestDaily] = []model.DigestRecipient{{UserID: 1, Email: ""}}
	notifRepo.unread[1] = 5

	svc := service.NewNotificationDigestService(prefRepo, notifRepo, enqueuer, "https://example.com")
	summary, err := svc.Run(ctx, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Emailed != 0 {
		t.Errorf("expected no email for a recipient with no address, got emailed=%d", summary.Emailed)
	}
	if len(enqueuer.sent) != 0 {
		t.Errorf("expected no email enqueued, got %d", len(enqueuer.sent))
	}
	if len(prefRepo.markedSent) != 1 {
		t.Error("expected the cursor to still advance for an emailless recipient")
	}
}

func TestNotificationDigestService_Run_ProcessesBothCadencesWithDistinctCutoffs(t *testing.T) {
	ctx := context.Background()
	notifRepo := newDigestNotifRepo()
	prefRepo := newDigestPrefRepo()
	enqueuer := &mockEnqueuer{}

	now := time.Date(2026, 7, 19, 6, 0, 0, 0, time.UTC)

	svc := service.NewNotificationDigestService(prefRepo, notifRepo, enqueuer, "https://example.com")
	if _, err := svc.Run(ctx, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prefRepo.listDueCalls) != 2 {
		t.Fatalf("expected ListDue called once per cadence, got %d calls", len(prefRepo.listDueCalls))
	}

	var dailyCutoff, weeklyCutoff time.Time
	for _, c := range prefRepo.listDueCalls {
		switch c.frequency {
		case model.DigestDaily:
			dailyCutoff = c.sentBefore
		case model.DigestWeekly:
			weeklyCutoff = c.sentBefore
		default:
			t.Errorf("unexpected frequency queried: %q", c.frequency)
		}
	}

	wantDaily := now.Add(-24 * time.Hour)
	wantWeekly := now.Add(-7 * 24 * time.Hour)
	if !dailyCutoff.Equal(wantDaily) {
		t.Errorf("daily cutoff = %v, want %v", dailyCutoff, wantDaily)
	}
	if !weeklyCutoff.Equal(wantWeekly) {
		t.Errorf("weekly cutoff = %v, want %v", weeklyCutoff, wantWeekly)
	}
}

func TestNotificationDigestService_Run_OneRecipientFailureDoesNotAbortOthers(t *testing.T) {
	ctx := context.Background()
	notifRepo := newDigestNotifRepo()
	prefRepo := newDigestPrefRepo()
	enqueuer := &mockEnqueuer{}

	prefRepo.due[model.DigestDaily] = []model.DigestRecipient{
		{UserID: 1, Email: "broken@example.com"},
		{UserID: 2, Email: "fine@example.com"},
	}
	notifRepo.unread[1] = 1
	notifRepo.countErrFor[1] = errors.New("db down for user 1")
	notifRepo.unread[2] = 1
	notifRepo.items[2] = []model.Notification{
		{ID: 1, UserID: 2, Type: model.NotifSystem, Data: json.RawMessage(`{}`), CreatedAt: time.Now()},
	}

	svc := service.NewNotificationDigestService(prefRepo, notifRepo, enqueuer, "https://example.com")
	summary, err := svc.Run(ctx, time.Now())
	if err != nil {
		t.Fatalf("expected a per-recipient failure to be swallowed, not returned: %v", err)
	}
	if summary.Recipients != 2 {
		t.Errorf("expected both recipients evaluated, got %d", summary.Recipients)
	}
	if summary.Emailed != 1 {
		t.Errorf("expected only the healthy recipient emailed, got %d", summary.Emailed)
	}
	if len(enqueuer.sent) != 1 || enqueuer.sent[0].to != "fine@example.com" {
		t.Errorf("expected exactly one email to the healthy recipient, got %+v", enqueuer.sent)
	}
	// User 1 errored before MarkSent, so only user 2's cursor should have advanced.
	if len(prefRepo.markedSent) != 1 || prefRepo.markedSent[0] != 2 {
		t.Errorf("markedSent = %v, want [2]", prefRepo.markedSent)
	}
}

// If the email fails to enqueue, the recipient's cursor must NOT advance —
// otherwise the unread notifications that failed to send would be silently
// dropped from every future digest instead of being retried next run.
func TestNotificationDigestService_Run_EnqueueFailureDoesNotAdvanceCursor(t *testing.T) {
	ctx := context.Background()
	notifRepo := newDigestNotifRepo()
	prefRepo := newDigestPrefRepo()
	enqueuer := &mockEnqueuer{err: errors.New("smtp down")}

	prefRepo.due[model.DigestDaily] = []model.DigestRecipient{{UserID: 1, Email: "alice@example.com"}}
	notifRepo.unread[1] = 2
	notifRepo.items[1] = []model.Notification{
		{ID: 1, UserID: 1, Type: model.NotifMention, Data: json.RawMessage(`{}`), CreatedAt: time.Now()},
	}

	svc := service.NewNotificationDigestService(prefRepo, notifRepo, enqueuer, "https://example.com")
	summary, err := svc.Run(ctx, time.Now())
	if err != nil {
		t.Fatalf("expected a per-recipient enqueue failure to be swallowed, not returned: %v", err)
	}
	if summary.Emailed != 0 {
		t.Errorf("expected 0 emailed when enqueue fails, got %d", summary.Emailed)
	}
	if len(prefRepo.markedSent) != 0 {
		t.Errorf("expected the cursor to stay put on enqueue failure so the retry sees the same window, but MarkSent was called for %v", prefRepo.markedSent)
	}
}

func TestNotificationDigestService_Run_ListDueErrorAbortsThatCadence(t *testing.T) {
	ctx := context.Background()
	notifRepo := newDigestNotifRepo()
	prefRepo := newDigestPrefRepo()
	enqueuer := &mockEnqueuer{}

	prefRepo.listDueErr[model.DigestDaily] = errors.New("db down")

	svc := service.NewNotificationDigestService(prefRepo, notifRepo, enqueuer, "https://example.com")
	_, err := svc.Run(ctx, time.Now())
	if err == nil {
		t.Fatal("expected an error when ListDue fails")
	}
}

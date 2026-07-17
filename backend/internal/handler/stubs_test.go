package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// Shared repository stubs for the in-package handler tests. Each stub carries
// only the behaviour the handlers under test actually exercise; everything else
// returns a zero value so the compiler is satisfied without inventing semantics.

// errStub is the generic failure a stub returns when a scenario asks for one.
var errStub = errors.New("stub failure")

// staffAuthed authenticates the request as a moderator. Staff checks go through
// Permissions.IsStaff(), which is IsAdmin || IsModerator — a high Level alone is
// not enough, so tests for staff-only endpoints must set the flag.
func staffAuthed(r *http.Request, userID int64) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.PermissionsKey,
		model.Permissions{Level: 100, IsModerator: true})
	return r.WithContext(ctx)
}

// adminAuthed authenticates the request as a full admin.
func adminAuthed(r *http.Request, userID int64) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.PermissionsKey,
		model.Permissions{Level: 100, IsAdmin: true, IsModerator: true})
	return r.WithContext(ctx)
}

// --- users ------------------------------------------------------------------

type stubUserRepo struct {
	users   map[int64]*model.User
	getErr  error
	updated []*model.User
	updErr  error
}

func newStubUserRepo(users ...*model.User) *stubUserRepo {
	m := make(map[int64]*model.User, len(users))
	for _, u := range users {
		m[u.ID] = u
	}
	return &stubUserRepo{users: m}
}

func (s *stubUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	u, ok := s.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}
func (s *stubUserRepo) GetByUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("user not found")
}
func (s *stubUserRepo) GetByUsernames(_ context.Context, _ []string) ([]model.User, error) {
	return nil, nil
}
func (s *stubUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("user not found")
}
func (s *stubUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("user not found")
}
func (s *stubUserRepo) Count(_ context.Context) (int64, error) { return int64(len(s.users)), nil }
func (s *stubUserRepo) Create(_ context.Context, _ *model.User) error {
	return nil
}
func (s *stubUserRepo) Update(_ context.Context, u *model.User) error {
	if s.updErr != nil {
		return s.updErr
	}
	s.updated = append(s.updated, u)
	s.users[u.ID] = u
	return nil
}
func (s *stubUserRepo) IncrementStats(_ context.Context, _ int64, _, _ int64) error { return nil }
func (s *stubUserRepo) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (s *stubUserRepo) ListStaff(_ context.Context) ([]model.User, error) { return nil, nil }
func (s *stubUserRepo) UpdateLastAccess(_ context.Context, _ int64) error { return nil }
func (s *stubUserRepo) SetPrivilegeFlag(_ context.Context, userID int64, restrictionType string, value bool) error {
	u, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	switch restrictionType {
	case "download":
		u.CanDownload = value
	case "upload":
		u.CanUpload = value
	case "chat":
		u.CanChat = value
	case "invite":
		u.CanInvite = value
	}
	return nil
}

func (s *stubUserRepo) AdjustInvites(_ context.Context, userID int64, delta int64) error {
	u, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	u.Invites += int(delta)
	return nil
}

func (s *stubUserRepo) SetInvites(_ context.Context, userID int64, invites int) error {
	u, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	u.Invites = invites
	return nil
}

// --- private messages -------------------------------------------------------

type stubMessageRepo struct {
	byID       map[int64]*model.Message
	inbox      []model.Message
	outbox     []model.Message
	total      int64
	unread     int
	created    []*model.Message
	markedRead []int64
	deleted    []int64

	createErr error
	getErr    error
	listErr   error
	markErr   error
	deleteErr error
	unreadErr error

	nextID int64
}

func newStubMessageRepo() *stubMessageRepo {
	return &stubMessageRepo{byID: map[int64]*model.Message{}, nextID: 1}
}

func (s *stubMessageRepo) Create(_ context.Context, msg *model.Message) error {
	if s.createErr != nil {
		return s.createErr
	}
	msg.ID = s.nextID
	s.nextID++
	s.created = append(s.created, msg)
	stored := *msg
	s.byID[msg.ID] = &stored
	return nil
}

func (s *stubMessageRepo) GetByID(_ context.Context, id int64) (*model.Message, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	m, ok := s.byID[id]
	if !ok {
		return nil, errors.New("message not found")
	}
	return m, nil
}

func (s *stubMessageRepo) ListInbox(_ context.Context, _ int64, _, _ int) ([]model.Message, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.inbox, s.total, nil
}

func (s *stubMessageRepo) ListOutbox(_ context.Context, _ int64, _, _ int) ([]model.Message, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.outbox, s.total, nil
}

func (s *stubMessageRepo) MarkAsRead(_ context.Context, id, _ int64) error {
	if s.markErr != nil {
		return s.markErr
	}
	s.markedRead = append(s.markedRead, id)
	return nil
}

func (s *stubMessageRepo) DeleteForUser(_ context.Context, id, _ int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *stubMessageRepo) CountUnread(_ context.Context, _ int64) (int, error) {
	return s.unread, s.unreadErr
}

// --- site settings ----------------------------------------------------------

type stubSiteSettingsRepo struct {
	values map[string]string
	all    []model.SiteSetting
	set    map[string]string

	getErr    error
	getAllErr error
	setErr    error
}

func newStubSiteSettingsRepo() *stubSiteSettingsRepo {
	return &stubSiteSettingsRepo{values: map[string]string{}, set: map[string]string{}}
}

func (s *stubSiteSettingsRepo) Get(_ context.Context, key string) (*model.SiteSetting, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	v, ok := s.values[key]
	if !ok {
		return nil, nil
	}
	return &model.SiteSetting{Key: key, Value: v}, nil
}

func (s *stubSiteSettingsRepo) Set(_ context.Context, key, value string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.set[key] = value
	s.values[key] = value
	return nil
}

func (s *stubSiteSettingsRepo) GetAll(_ context.Context) ([]model.SiteSetting, error) {
	if s.getAllErr != nil {
		return nil, s.getAllErr
	}
	return s.all, nil
}

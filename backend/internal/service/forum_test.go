package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

type mockForumCategoryRepo struct {
	categories  []model.ForumCategory
	err         error
	nextID      int64
	forumCounts map[int64]int64
}

func (m *mockForumCategoryRepo) GetByID(_ context.Context, id int64) (*model.ForumCategory, error) {
	for i := range m.categories {
		if m.categories[i].ID == id {
			return &m.categories[i], nil
		}
	}
	return nil, sql.ErrNoRows
}
func (m *mockForumCategoryRepo) List(_ context.Context) ([]model.ForumCategory, error) { return m.categories, m.err }
func (m *mockForumCategoryRepo) Create(_ context.Context, cat *model.ForumCategory) error {
	if m.nextID == 0 { m.nextID = 1 }
	cat.ID = m.nextID
	m.nextID++
	cat.CreatedAt = time.Now()
	m.categories = append(m.categories, *cat)
	return nil
}
func (m *mockForumCategoryRepo) Update(_ context.Context, cat *model.ForumCategory) error {
	for i := range m.categories {
		if m.categories[i].ID == cat.ID {
			m.categories[i] = *cat
			return nil
		}
	}
	return sql.ErrNoRows
}
func (m *mockForumCategoryRepo) Delete(_ context.Context, id int64) error {
	for i := range m.categories {
		if m.categories[i].ID == id {
			m.categories = append(m.categories[:i], m.categories[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}
func (m *mockForumCategoryRepo) CountForumsByCategory(_ context.Context, categoryID int64) (int64, error) {
	if m.forumCounts != nil {
		return m.forumCounts[categoryID], nil
	}
	return 0, nil
}

type mockForumRepo struct {
	forums       []model.Forum
	forumByID    map[int64]*model.Forum
	listErr      error
	getErr       error
	recalculated []int64
	nextID       int64
	topicCounts  map[int64]int64
}

func (m *mockForumRepo) GetByID(_ context.Context, id int64) (*model.Forum, error) {
	if m.getErr != nil { return nil, m.getErr }
	if f, ok := m.forumByID[id]; ok { return f, nil }
	return nil, sql.ErrNoRows
}
func (m *mockForumRepo) ListByCategory(_ context.Context, _ int64) ([]model.Forum, error) { return m.forums, m.listErr }
func (m *mockForumRepo) List(_ context.Context) ([]model.Forum, error) { return m.forums, m.listErr }
func (m *mockForumRepo) Create(_ context.Context, forum *model.Forum) error {
	if m.nextID == 0 { m.nextID = 1 }
	forum.ID = m.nextID
	m.nextID++
	forum.CreatedAt = time.Now()
	if m.forumByID == nil { m.forumByID = make(map[int64]*model.Forum) }
	m.forumByID[forum.ID] = forum
	m.forums = append(m.forums, *forum)
	return nil
}
func (m *mockForumRepo) Update(_ context.Context, forum *model.Forum) error {
	if m.forumByID != nil { m.forumByID[forum.ID] = forum }
	return nil
}
func (m *mockForumRepo) Delete(_ context.Context, id int64) error {
	if m.forumByID != nil { delete(m.forumByID, id) }
	return nil
}
func (m *mockForumRepo) CountTopicsByForum(_ context.Context, forumID int64) (int64, error) {
	if m.topicCounts != nil { return m.topicCounts[forumID], nil }
	return 0, nil
}
func (m *mockForumRepo) IncrementTopicCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumRepo) UpdateLastPost(_ context.Context, _ int64, _ int64) error { return nil }
func (m *mockForumRepo) RecalculateLastPost(_ context.Context, _ int64) error { return nil }
func (m *mockForumRepo) RecalculateCounts(_ context.Context, forumID int64) error { m.recalculated = append(m.recalculated, forumID); return nil }

type mockForumTopicRepo struct{
	topics []model.ForumTopic; topicByID map[int64]*model.ForumTopic; total int64; created *model.ForumTopic; listErr, getErr error
	lockedCalls  map[int64]bool
	pinnedCalls  map[int64]bool
	titleCalls   map[int64]string
	forumIDCalls map[int64]int64
	deletedIDs   []int64
}
func (m *mockForumTopicRepo) GetByID(_ context.Context, id int64) (*model.ForumTopic, error) { if m.getErr != nil { return nil, m.getErr }; if t, ok := m.topicByID[id]; ok { return t, nil }; return nil, sql.ErrNoRows }
func (m *mockForumTopicRepo) ListByForum(_ context.Context, _ int64, _, _ int) ([]model.ForumTopic, int64, error) { return m.topics, m.total, m.listErr }
func (m *mockForumTopicRepo) Create(_ context.Context, topic *model.ForumTopic) error { topic.ID = 100; topic.CreatedAt = time.Now(); topic.UpdatedAt = time.Now(); m.created = topic; return nil }
func (m *mockForumTopicRepo) IncrementViewCount(_ context.Context, _ int64) error { return nil }
func (m *mockForumTopicRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumTopicRepo) UpdateLastPost(_ context.Context, _ int64, _ int64, _ time.Time) error { return nil }
func (m *mockForumTopicRepo) RecalculateLastPost(_ context.Context, _ int64) error { return nil }
func (m *mockForumTopicRepo) SetLocked(_ context.Context, id int64, locked bool) error { if m.lockedCalls == nil { m.lockedCalls = make(map[int64]bool) }; m.lockedCalls[id] = locked; return nil }
func (m *mockForumTopicRepo) SetPinned(_ context.Context, id int64, pinned bool) error { if m.pinnedCalls == nil { m.pinnedCalls = make(map[int64]bool) }; m.pinnedCalls[id] = pinned; return nil }
func (m *mockForumTopicRepo) UpdateTitle(_ context.Context, id int64, title string) error { if m.titleCalls == nil { m.titleCalls = make(map[int64]string) }; m.titleCalls[id] = title; return nil }
func (m *mockForumTopicRepo) UpdateForumID(_ context.Context, id int64, forumID int64) error { if m.forumIDCalls == nil { m.forumIDCalls = make(map[int64]int64) }; m.forumIDCalls[id] = forumID; return nil }
func (m *mockForumTopicRepo) Delete(_ context.Context, id int64) error { m.deletedIDs = append(m.deletedIDs, id); return nil }

type mockForumPostRepo struct{
	posts         []model.ForumPost
	postByID      map[int64]*model.ForumPost
	total         int64
	listErr       error
	getErr        error
	firstPostID   int64
	updated       *model.ForumPost
	deleted       int64
	searchResults []model.ForumSearchResult
	searchTotal   int64
	searchErr     error
}
func (m *mockForumPostRepo) GetByID(_ context.Context, id int64) (*model.ForumPost, error) { if m.getErr != nil { return nil, m.getErr }; if p, ok := m.postByID[id]; ok { return p, nil }; return nil, sql.ErrNoRows }
func (m *mockForumPostRepo) ListByTopic(_ context.Context, _ int64, _, _ int) ([]model.ForumPost, int64, error) { return m.posts, m.total, m.listErr }
func (m *mockForumPostRepo) Create(_ context.Context, post *model.ForumPost) error { post.ID = 200; post.CreatedAt = time.Now(); return nil }
func (m *mockForumPostRepo) Update(_ context.Context, post *model.ForumPost) error { m.updated = post; return nil }
func (m *mockForumPostRepo) Delete(_ context.Context, id int64) error { m.deleted = id; return nil }
func (m *mockForumPostRepo) CountByUser(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *mockForumPostRepo) Search(_ context.Context, _ string, _ *int64, _ int, _, _ int) ([]model.ForumSearchResult, int64, error) { return m.searchResults, m.searchTotal, m.searchErr }
func (m *mockForumPostRepo) GetFirstPostIDByTopic(_ context.Context, _ int64) (int64, error) { return m.firstPostID, nil }

type mockForumUserRepo struct{ user *model.User; err error }
func (m *mockForumUserRepo) GetByID(_ context.Context, _ int64) (*model.User, error) { return m.user, m.err }
func (m *mockForumUserRepo) GetByUsername(_ context.Context, _ string) (*model.User, error) { return nil, sql.ErrNoRows }
func (m *mockForumUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) { return nil, sql.ErrNoRows }
func (m *mockForumUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) { return nil, sql.ErrNoRows }
func (m *mockForumUserRepo) Count(_ context.Context) (int64, error) { return 0, nil }
func (m *mockForumUserRepo) Create(_ context.Context, _ *model.User) error { return nil }
func (m *mockForumUserRepo) Update(_ context.Context, _ *model.User) error { return nil }
func (m *mockForumUserRepo) IncrementStats(_ context.Context, _ int64, _, _ int64) error { return nil }
func (m *mockForumUserRepo) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) { return nil, 0, nil }
func (m *mockForumUserRepo) ListStaff(_ context.Context) ([]model.User, error) { return nil, nil }
func (m *mockForumUserRepo) UpdateLastAccess(_ context.Context, _ int64) error { return nil }

func TestForumService_ListCategories(t *testing.T) {
	svc := NewForumService(nil, &mockForumCategoryRepo{categories: []model.ForumCategory{{ID: 1, Name: "General", SortOrder: 1}, {ID: 2, Name: "Empty", SortOrder: 2}}}, &mockForumRepo{forums: []model.Forum{{ID: 1, CategoryID: 1, Name: "Announcements", MinGroupLevel: 0}, {ID: 2, CategoryID: 1, Name: "VIP Only", MinGroupLevel: 100}}}, nil, nil, nil, nil, nil)
	cats, err := svc.ListCategories(context.Background(), model.Permissions{Level: 5})
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if len(cats) != 1 { t.Fatalf("expected 1 category, got %d", len(cats)) }
	if len(cats[0].Forums) != 1 { t.Fatalf("expected 1 forum, got %d", len(cats[0].Forums)) }
	cats, err = svc.ListCategories(context.Background(), model.Permissions{Level: 200})
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if len(cats[0].Forums) != 2 { t.Fatalf("expected 2 forums, got %d", len(cats[0].Forums)) }
}

func TestForumService_GetForum_AccessDenied(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, Name: "VIP", MinGroupLevel: 100}}}, nil, nil, nil, nil, nil)
	if _, err := svc.GetForum(context.Background(), 1, model.Permissions{Level: 5}); !errors.Is(err, ErrForumAccessDenied) { t.Errorf("expected ErrForumAccessDenied, got %v", err) }
	if f, err := svc.GetForum(context.Background(), 1, model.Permissions{Level: 100}); err != nil { t.Fatalf("unexpected: %v", err) } else if f.Name != "VIP" { t.Errorf("expected VIP") }
}

func TestForumService_GetForum_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{}}, nil, nil, nil, nil, nil)
	if _, err := svc.GetForum(context.Background(), 999, model.Permissions{Level: 100}); !errors.Is(err, ErrForumNotFound) { t.Errorf("expected ErrForumNotFound, got %v", err) }
}

func TestForumService_CreateTopic_Success(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, Username: "alice", GroupName: "User"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	topic, post, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 5}, "Test Topic", "Hello world")
	if err != nil { t.Fatalf("unexpected: %v", err) }
	if topic.Title != "Test Topic" { t.Errorf("wrong title") }
	if post == nil { t.Fatal("nil post") }
}

func TestForumService_CreateTopic_EmptyTitle(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "", "body"); !errors.Is(err, ErrInvalidTopic) { t.Errorf("expected ErrInvalidTopic") }
}

func TestForumService_CreateTopic_EmptyBody(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "Title", ""); !errors.Is(err, ErrInvalidPost) { t.Errorf("expected ErrInvalidPost") }
}

func TestForumService_CreateTopic_UserCanForumFalse(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, nil, nil, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: false}}, nil, nil)
	if _, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 5}, "Title", "Body"); !errors.Is(err, ErrForumAccessDenied) { t.Errorf("expected ErrForumAccessDenied") }
}

func TestForumService_CreateTopic_MinPostLevel(t *testing.T) {
	// User below min_post_level should be denied
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0, MinPostLevel: 50}}}, nil, nil, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	if _, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 5}, "Title", "Body"); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied for user below min_post_level, got %v", err)
	}

	// User at min_post_level should succeed
	svc = NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0, MinPostLevel: 50}}}, &mockForumTopicRepo{}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, Username: "alice", GroupName: "User"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	topic, post, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 50}, "Title", "Body")
	if err != nil {
		t.Fatalf("expected success for user at min_post_level, got %v", err)
	}
	if topic == nil || post == nil {
		t.Fatal("expected non-nil topic and post")
	}

	// User above min_post_level should succeed
	svc = NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0, MinPostLevel: 50}}}, &mockForumTopicRepo{}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, Username: "alice", GroupName: "User"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	topic, post, err = svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 100}, "Title", "Body")
	if err != nil {
		t.Fatalf("expected success for user above min_post_level, got %v", err)
	}
	if topic == nil || post == nil {
		t.Fatal("expected non-nil topic and post")
	}
}

func TestForumService_CreatePost_MinPostLevel_DoesNotBlock(t *testing.T) {
	// Replies should NOT be blocked by min_post_level — only topic creation is gated
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0, MinPostLevel: 50}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, TopicID: 1, Username: "alice", GroupName: "User"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply body", nil)
	if err != nil {
		t.Fatalf("expected replies to succeed regardless of min_post_level, got %v", err)
	}
	if post == nil {
		t.Fatal("expected non-nil post")
	}
}

func TestForumService_CreatePost_TopicLocked(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, Locked: true}}}, nil, nil, nil, nil)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", nil); !errors.Is(err, ErrTopicLocked) { t.Errorf("expected ErrTopicLocked") }
}

func TestForumService_CreatePost_TopicLocked_StaffAllowed(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, Locked: true}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, TopicID: 1, Username: "admin", GroupName: "Admin"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 200, IsAdmin: true}, "Staff reply in locked topic", nil)
	if err != nil { t.Fatalf("expected staff to post in locked topic, got: %v", err) }
	if post == nil { t.Fatal("nil post") }
}

func TestForumService_CreatePost_Success(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, TopicID: 1, Username: "alice", GroupName: "User"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply body", nil)
	if err != nil { t.Fatalf("unexpected: %v", err) }
	if post == nil { t.Fatal("nil post") }
}

func TestForumService_CreatePost_EmptyBody(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{}, "", nil); !errors.Is(err, ErrInvalidPost) { t.Errorf("expected ErrInvalidPost") }
}

func TestForumService_CreatePost_CanForumFalse(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, nil, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: false}}, nil, nil)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply body", nil); !errors.Is(err, ErrForumAccessDenied) { t.Errorf("expected ErrForumAccessDenied, got %v", err) }
}

func TestForumService_CreatePost_InvalidReplyToPostID_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	replyTo := int64(999)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", &replyTo); !errors.Is(err, ErrInvalidReply) { t.Errorf("expected ErrInvalidReply, got %v", err) }
}

func TestForumService_CreatePost_InvalidReplyToPostID_DifferentTopic(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{50: {ID: 50, TopicID: 2}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	replyTo := int64(50)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", &replyTo); !errors.Is(err, ErrInvalidReply) { t.Errorf("expected ErrInvalidReply, got %v", err) }
}

func TestForumService_CreatePost_ValidReplyToPostID(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{50: {ID: 50, TopicID: 1}, 200: {ID: 200, TopicID: 1}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}}, nil, nil)
	replyTo := int64(50)
	if post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", &replyTo); err != nil { t.Fatalf("unexpected: %v", err) } else if post == nil { t.Fatal("nil post") }
}

func TestForumService_GetTopic_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{}}, nil, nil, nil, nil)
	if _, err := svc.GetTopic(context.Background(), 999, 1, model.Permissions{Level: 5}); !errors.Is(err, ErrTopicNotFound) { t.Errorf("expected ErrTopicNotFound") }
}

func TestForumService_GetTopic_ViewCountDebounce(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, ViewCount: 10}}}
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, topicRepo, nil, nil, nil, nil)
	topic, _ := svc.GetTopic(context.Background(), 1, 1, model.Permissions{Level: 5})
	if topic.ViewCount != 11 { t.Errorf("expected 11, got %d", topic.ViewCount) }
	topicRepo.topicByID[1].ViewCount = 10
	topic, _ = svc.GetTopic(context.Background(), 1, 1, model.Permissions{Level: 5})
	if topic.ViewCount != 10 { t.Errorf("expected 10 (debounce), got %d", topic.ViewCount) }
	topic, _ = svc.GetTopic(context.Background(), 1, 2, model.Permissions{Level: 5})
	if topic.ViewCount != 11 { t.Errorf("expected 11 for diff user, got %d", topic.ViewCount) }
}

func TestForumService_ListTopics(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, Name: "General", MinGroupLevel: 0}}}, &mockForumTopicRepo{topics: []model.ForumTopic{{ID: 1, Title: "Hello", Pinned: true}, {ID: 2, Title: "World"}}, total: 2}, nil, nil, nil, nil)
	forum, topics, total, err := svc.ListTopics(context.Background(), 1, model.Permissions{Level: 5}, 1, 25)
	if err != nil { t.Fatalf("unexpected: %v", err) }
	if forum.Name != "General" { t.Errorf("wrong name") }
	if len(topics) != 2 { t.Errorf("expected 2 topics") }
	if total != 2 { t.Errorf("expected total 2") }
}

// --- Search tests ---

func TestForumService_Search_Success(t *testing.T) {
	results := []model.ForumSearchResult{{PostID: 1, Body: "hello world", TopicID: 10, TopicTitle: "Greetings", ForumID: 1, ForumName: "General", UserID: 1, Username: "alice"}}
	svc := NewForumService(nil, nil, nil, nil, &mockForumPostRepo{searchResults: results, searchTotal: 1}, nil, nil, nil)
	got, total, err := svc.Search(context.Background(), "hello", model.Permissions{Level: 5}, nil, 1, 25)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if total != 1 { t.Errorf("expected total 1, got %d", total) }
	if len(got) != 1 { t.Errorf("expected 1 result, got %d", len(got)) }
	if got[0].PostID != 1 { t.Errorf("expected post ID 1") }
}

func TestForumService_Search_EmptyQuery(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.Search(context.Background(), "", model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch, got %v", err) }
	if _, _, err := svc.Search(context.Background(), "   ", model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch for whitespace-only query, got %v", err) }
}

func TestForumService_Search_QueryTooShort(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.Search(context.Background(), "a", model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch for single-char query, got %v", err) }
}

func TestForumService_Search_QueryTooLong(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	longQuery := strings.Repeat("a", 201)
	if _, _, err := svc.Search(context.Background(), longQuery, model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch, got %v", err) }
}

func TestForumService_Search_PaginationClamping(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, &mockForumPostRepo{searchResults: nil, searchTotal: 0}, nil, nil, nil)
	// Negative page and perPage should be clamped, not cause errors
	_, _, err := svc.Search(context.Background(), "test", model.Permissions{Level: 5}, nil, -1, -1)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
}

func TestForumService_Search_WithForumFilter(t *testing.T) {
	results := []model.ForumSearchResult{{PostID: 5, Body: "filtered result", TopicID: 20, TopicTitle: "Topic", ForumID: 2, ForumName: "Support", UserID: 1, Username: "bob"}}
	svc := NewForumService(nil, nil, nil, nil, &mockForumPostRepo{searchResults: results, searchTotal: 1}, nil, nil, nil)
	forumID := int64(2)
	got, total, err := svc.Search(context.Background(), "filtered", model.Permissions{Level: 5}, &forumID, 1, 25)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if total != 1 { t.Errorf("expected total 1, got %d", total) }
	if len(got) != 1 { t.Errorf("expected 1 result") }
}

// --- EditPost tests ---

func TestForumService_EditPost_AuthorSuccess(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body", Username: "alice", GroupName: "User"},
		},
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}},
		nil, nil, )
	post, err := svc.EditPost(context.Background(), 10, 5, model.Permissions{Level: 5}, "new body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post == nil {
		t.Fatal("expected non-nil post")
	}
	if postRepo.updated == nil {
		t.Fatal("expected Update to be called")
	}
	if postRepo.updated.Body != "new body" {
		t.Errorf("expected body 'new body', got %q", postRepo.updated.Body)
	}
	if postRepo.updated.EditedBy == nil || *postRepo.updated.EditedBy != 5 {
		t.Errorf("expected edited_by=5")
	}
}

func TestForumService_EditPost_StaffSuccess(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body", Username: "alice", GroupName: "User"},
		},
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo, nil, nil, nil, )
	// Staff user (different from post author) can edit
	_, err := svc.EditPost(context.Background(), 10, 99, model.Permissions{Level: 200, IsModerator: true}, "staff edit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postRepo.updated.Body != "staff edit" {
		t.Errorf("expected body 'staff edit', got %q", postRepo.updated.Body)
	}
}

func TestForumService_EditPost_Unauthorized(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body"},
		},
	}
	svc := NewForumService(nil, nil, nil, nil, postRepo, nil, nil, nil)
	_, err := svc.EditPost(context.Background(), 10, 99, model.Permissions{Level: 5}, "hacked")
	if !errors.Is(err, ErrPostEditDenied) {
		t.Errorf("expected ErrPostEditDenied, got %v", err)
	}
}

func TestForumService_EditPost_EmptyBody(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.EditPost(context.Background(), 10, 1, model.Permissions{}, "   ")
	if !errors.Is(err, ErrInvalidPost) {
		t.Errorf("expected ErrInvalidPost, got %v", err)
	}
}

func TestForumService_EditPost_NotFound(t *testing.T) {
	postRepo := &mockForumPostRepo{postByID: map[int64]*model.ForumPost{}}
	svc := NewForumService(nil, nil, nil, nil, postRepo, nil, nil, nil)
	_, err := svc.EditPost(context.Background(), 999, 1, model.Permissions{Level: 5}, "body")
	if !errors.Is(err, ErrPostNotFound) {
		t.Errorf("expected ErrPostNotFound, got %v", err)
	}
}

func TestForumService_EditPost_ForumAccessDenied(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body"},
		},
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 100}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}},
		nil, nil, )
	_, err := svc.EditPost(context.Background(), 10, 5, model.Permissions{Level: 5}, "body")
	if !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}
}

// --- DeletePost tests ---

func TestForumService_DeletePost_AuthorSuccess(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
		firstPostID: 1, // first post is ID 1, so post 10 can be deleted
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}},
		nil, nil, )
	err := svc.DeletePost(context.Background(), 10, 5, model.Permissions{Level: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postRepo.deleted != 10 {
		t.Errorf("expected Delete(10), got %d", postRepo.deleted)
	}
}

func TestForumService_DeletePost_StaffSuccess(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
		firstPostID: 1,
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo, nil, nil, nil, )
	err := svc.DeletePost(context.Background(), 10, 99, model.Permissions{Level: 200, IsAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForumService_DeletePost_FirstPostPrevented(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			1: {ID: 1, TopicID: 1, UserID: 5, Body: "opening post"},
		},
		firstPostID: 1, // this IS the first post
	}
	svc := NewForumService(nil, nil, nil, nil, postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}},
		nil, nil, )
	err := svc.DeletePost(context.Background(), 1, 5, model.Permissions{Level: 5})
	if !errors.Is(err, ErrCannotDeleteFirstPost) {
		t.Errorf("expected ErrCannotDeleteFirstPost, got %v", err)
	}
}

func TestForumService_DeletePost_Unauthorized(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
	}
	svc := NewForumService(nil, nil, nil, nil, postRepo, nil, nil, nil)
	err := svc.DeletePost(context.Background(), 10, 99, model.Permissions{Level: 5})
	if !errors.Is(err, ErrPostDeleteDenied) {
		t.Errorf("expected ErrPostDeleteDenied, got %v", err)
	}
}

func TestForumService_DeletePost_NotFound(t *testing.T) {
	postRepo := &mockForumPostRepo{postByID: map[int64]*model.ForumPost{}}
	svc := NewForumService(nil, nil, nil, nil, postRepo, nil, nil, nil)
	err := svc.DeletePost(context.Background(), 999, 1, model.Permissions{Level: 5})
	if !errors.Is(err, ErrPostNotFound) {
		t.Errorf("expected ErrPostNotFound, got %v", err)
	}
}

// --- EditPost locked topic tests ---

func TestForumService_EditPost_LockedTopic_NonStaffDenied(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body"},
		},
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, Locked: true}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}},
		nil, nil, )
	_, err := svc.EditPost(context.Background(), 10, 5, model.Permissions{Level: 5}, "new body")
	if !errors.Is(err, ErrTopicLocked) {
		t.Errorf("expected ErrTopicLocked, got %v", err)
	}
}

func TestForumService_EditPost_LockedTopic_StaffAllowed(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body", Username: "alice", GroupName: "User"},
		},
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, Locked: true}}},
		postRepo, nil, nil, nil, )
	_, err := svc.EditPost(context.Background(), 10, 99, model.Permissions{Level: 200, IsModerator: true}, "staff edit")
	if err != nil {
		t.Fatalf("expected staff to edit in locked topic, got: %v", err)
	}
}

func TestForumService_EditPost_CanForumFalse_NonStaffDenied(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body"},
		},
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: false}},
		nil, nil, )
	_, err := svc.EditPost(context.Background(), 10, 5, model.Permissions{Level: 5}, "new body")
	if !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied for can_forum=false, got %v", err)
	}
}

func TestForumService_EditPost_CanForumFalse_StaffAllowed(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "old body", Username: "alice", GroupName: "User"},
		},
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo, nil, nil, nil, // no user repo needed — staff bypasses can_forum check
	)
	_, err := svc.EditPost(context.Background(), 10, 99, model.Permissions{Level: 200, IsAdmin: true}, "admin edit")
	if err != nil {
		t.Fatalf("expected staff to bypass can_forum, got: %v", err)
	}
}

// --- DeletePost locked topic tests ---

func TestForumService_DeletePost_LockedTopic_NonStaffDenied(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
		firstPostID: 1,
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, Locked: true}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}},
		nil, nil, )
	err := svc.DeletePost(context.Background(), 10, 5, model.Permissions{Level: 5})
	if !errors.Is(err, ErrTopicLocked) {
		t.Errorf("expected ErrTopicLocked, got %v", err)
	}
}

func TestForumService_DeletePost_LockedTopic_StaffAllowed(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
		firstPostID: 1,
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, Locked: true}}},
		postRepo, nil, nil, nil, )
	err := svc.DeletePost(context.Background(), 10, 99, model.Permissions{Level: 200, IsAdmin: true})
	if err != nil {
		t.Fatalf("expected staff to delete in locked topic, got: %v", err)
	}
}

func TestForumService_DeletePost_CanForumFalse_NonStaffDenied(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
		firstPostID: 1,
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: false}},
		nil, nil, )
	err := svc.DeletePost(context.Background(), 10, 5, model.Permissions{Level: 5})
	if !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied for can_forum=false, got %v", err)
	}
}

func TestForumService_DeletePost_CanForumFalse_StaffAllowed(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
		firstPostID: 1,
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo, nil, nil, nil, // no user repo — staff bypasses can_forum
	)
	err := svc.DeletePost(context.Background(), 10, 99, model.Permissions{Level: 200, IsModerator: true})
	if err != nil {
		t.Fatalf("expected staff to bypass can_forum, got: %v", err)
	}
}

func TestForumService_DeletePost_ForumAccessDenied(t *testing.T) {
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			10: {ID: 10, TopicID: 1, UserID: 5, Body: "to delete"},
		},
		firstPostID: 1,
	}
	svc := NewForumService(nil, nil,
		&mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 100}}},
		&mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}},
		postRepo,
		&mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}},
		nil, nil, )
	err := svc.DeletePost(context.Background(), 10, 5, model.Permissions{Level: 5})
	if !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied for MinGroupLevel, got %v", err)
	}
}

// --- Moderation tests ---

var staffPerms = model.Permissions{Level: 100, IsAdmin: true}
var regularPerms = model.Permissions{Level: 5}

func TestForumService_LockTopic_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.LockTopic(context.Background(), 1, 0, staffPerms, event.Actor{}, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v, ok := topicRepo.lockedCalls[1]; !ok || !v {
		t.Error("expected SetLocked(1, true) to be called")
	}
}

func TestForumService_LockTopic_Unauthorized(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, UserID: 99}}}, nil, nil, nil, nil)
	if err := svc.LockTopic(context.Background(), 1, 0, regularPerms, event.Actor{}, ""); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}
}

func TestForumService_LockTopic_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{}}, nil, nil, nil, nil)
	if err := svc.LockTopic(context.Background(), 999, 0, staffPerms, event.Actor{}, ""); !errors.Is(err, ErrTopicNotFound) {
		t.Errorf("expected ErrTopicNotFound, got %v", err)
	}
}

func TestForumService_UnlockTopic_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, Locked: true}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.UnlockTopic(context.Background(), 1, staffPerms, event.Actor{}, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v, ok := topicRepo.lockedCalls[1]; !ok || v {
		t.Error("expected SetLocked(1, false) to be called")
	}
}

func TestForumService_UnlockTopic_Unauthorized(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := svc.UnlockTopic(context.Background(), 1, regularPerms, event.Actor{}, ""); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}
}

func TestForumService_PinTopic_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.PinTopic(context.Background(), 1, staffPerms, event.Actor{}, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v, ok := topicRepo.pinnedCalls[1]; !ok || !v {
		t.Error("expected SetPinned(1, true) to be called")
	}
}

func TestForumService_PinTopic_Unauthorized(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := svc.PinTopic(context.Background(), 1, regularPerms, event.Actor{}, ""); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}
}

func TestForumService_PinTopic_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{}}, nil, nil, nil, nil)
	if err := svc.PinTopic(context.Background(), 999, staffPerms, event.Actor{}, ""); !errors.Is(err, ErrTopicNotFound) {
		t.Errorf("expected ErrTopicNotFound, got %v", err)
	}
}

func TestForumService_UnpinTopic_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, Pinned: true}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.UnpinTopic(context.Background(), 1, staffPerms, event.Actor{}, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v, ok := topicRepo.pinnedCalls[1]; !ok || v {
		t.Error("expected SetPinned(1, false) to be called")
	}
}

func TestForumService_RenameTopic_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, Title: "Old Title"}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 99, staffPerms, "New Title", event.Actor{}, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if topicRepo.titleCalls[1] != "New Title" {
		t.Errorf("expected title 'New Title', got '%s'", topicRepo.titleCalls[1])
	}
}

func TestForumService_RenameTopic_EmptyTitle(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 99, staffPerms, "  ", event.Actor{}, ""); !errors.Is(err, ErrInvalidTopic) {
		t.Errorf("expected ErrInvalidTopic, got %v", err)
	}
}

func TestForumService_RenameTopic_TooLong(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	longTitle := strings.Repeat("a", 201)
	if err := svc.RenameTopic(context.Background(), 1, 99, staffPerms, longTitle, event.Actor{}, ""); !errors.Is(err, ErrInvalidTopic) {
		t.Errorf("expected ErrInvalidTopic, got %v", err)
	}
}

func TestForumService_RenameTopic_Unauthorized(t *testing.T) {
	// Non-author, non-staff user cannot rename
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, UserID: 10}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 99, regularPerms, "New Title", event.Actor{}, ""); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}
}

func TestForumService_RenameTopic_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{}}, nil, nil, nil, nil)
	if err := svc.RenameTopic(context.Background(), 999, 99, staffPerms, "Title", event.Actor{}, ""); !errors.Is(err, ErrTopicNotFound) {
		t.Errorf("expected ErrTopicNotFound, got %v", err)
	}
}

func TestForumService_RenameTopic_AuthorSuccess(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, UserID: 5, Title: "Old Title"}}}
	userRepo := &mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, userRepo, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 5, regularPerms, "Author Renamed", event.Actor{}, ""); err != nil {
		t.Fatalf("topic author should be able to rename: %v", err)
	}
	if topicRepo.titleCalls[1] != "Author Renamed" {
		t.Errorf("expected title 'Author Renamed', got '%s'", topicRepo.titleCalls[1])
	}
}

func TestForumService_RenameTopic_NonAuthorNonStaffDenied(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, UserID: 5}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 99, regularPerms, "Nope", event.Actor{}, ""); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied for non-author non-staff, got %v", err)
	}
}

func TestForumService_RenameTopic_AuthorLockedDenied(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, UserID: 5, Locked: true}}}
	userRepo := &mockForumUserRepo{user: &model.User{ID: 5, CanForum: true}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, userRepo, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 5, regularPerms, "Locked Rename", event.Actor{}, ""); !errors.Is(err, ErrTopicLocked) {
		t.Errorf("expected ErrTopicLocked for author on locked topic, got %v", err)
	}
}

func TestForumService_RenameTopic_StaffLockedAllowed(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, UserID: 5, Locked: true, Title: "Locked"}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 99, staffPerms, "Staff Rename", event.Actor{}, ""); err != nil {
		t.Fatalf("staff should be able to rename locked topic: %v", err)
	}
	if topicRepo.titleCalls[1] != "Staff Rename" {
		t.Errorf("expected title 'Staff Rename', got '%s'", topicRepo.titleCalls[1])
	}
}

func TestForumService_MoveTopic_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	forumRepo := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1}, 2: {ID: 2}}}
	svc := NewForumService(nil, nil, forumRepo, topicRepo, nil, nil, nil, nil)
	if err := svc.MoveTopic(context.Background(), 1, staffPerms, 2, event.Actor{}, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if topicRepo.forumIDCalls[1] != 2 {
		t.Errorf("expected forumID 2, got %d", topicRepo.forumIDCalls[1])
	}
	if len(forumRepo.recalculated) != 2 {
		t.Errorf("expected 2 recalculate calls, got %d", len(forumRepo.recalculated))
	}
}

func TestForumService_MoveTopic_SameForum(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.MoveTopic(context.Background(), 1, staffPerms, 1, event.Actor{}, ""); !errors.Is(err, ErrSameForum) {
		t.Errorf("expected ErrSameForum, got %v", err)
	}
}

func TestForumService_MoveTopic_TargetNotFound(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	forumRepo := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1}}}
	svc := NewForumService(nil, nil, forumRepo, topicRepo, nil, nil, nil, nil)
	if err := svc.MoveTopic(context.Background(), 1, staffPerms, 999, event.Actor{}, ""); !errors.Is(err, ErrForumNotFound) {
		t.Errorf("expected ErrForumNotFound, got %v", err)
	}
}

func TestForumService_MoveTopic_TopicNotFound(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{}}, nil, nil, nil, nil)
	if err := svc.MoveTopic(context.Background(), 999, staffPerms, 2, event.Actor{}, ""); !errors.Is(err, ErrTopicNotFound) {
		t.Errorf("expected ErrTopicNotFound, got %v", err)
	}
}

func TestForumService_MoveTopic_Unauthorized(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := svc.MoveTopic(context.Background(), 1, regularPerms, 2, event.Actor{}, ""); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}
}

func TestForumService_DeleteTopic_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	forumRepo := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1}}}
	svc := NewForumService(nil, nil, forumRepo, topicRepo, nil, nil, nil, nil)
	if err := svc.DeleteTopic(context.Background(), 1, 0, staffPerms, event.Actor{}, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(topicRepo.deletedIDs) != 1 || topicRepo.deletedIDs[0] != 1 {
		t.Error("expected topic 1 to be deleted")
	}
	if len(forumRepo.recalculated) != 1 || forumRepo.recalculated[0] != 1 {
		t.Error("expected forum 1 counts recalculated")
	}
}

func TestForumService_DeleteTopic_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{}}, nil, nil, nil, nil)
	if err := svc.DeleteTopic(context.Background(), 999, 0, staffPerms, event.Actor{}, ""); !errors.Is(err, ErrTopicNotFound) {
		t.Errorf("expected ErrTopicNotFound, got %v", err)
	}
}

func TestForumService_DeleteTopic_Unauthorized(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, UserID: 99}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.DeleteTopic(context.Background(), 1, 0, regularPerms, event.Actor{}, ""); !errors.Is(err, ErrTopicDeleteDenied) {
		t.Errorf("expected ErrTopicDeleteDenied, got %v", err)
	}
}

func TestForumService_LockTopic_ModeratorAllowed(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	modPerms := model.Permissions{Level: 50, IsModerator: true}
	if err := svc.LockTopic(context.Background(), 1, 0, modPerms, event.Actor{}, ""); err != nil {
		t.Fatalf("moderator should be allowed: %v", err)
	}
}

func TestForumService_RenameTopic_Unicode200Chars(t *testing.T) {
	// 200 Unicode characters (each is multi-byte but only 1 rune)
	title200 := strings.Repeat("\u00e9", 200) // e-acute, 2 bytes each = 400 bytes but 200 runes
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, Title: "Old"}}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)
	if err := svc.RenameTopic(context.Background(), 1, 99, staffPerms, title200, event.Actor{}, ""); err != nil {
		t.Fatalf("200 unicode chars should be allowed: %v", err)
	}
	title201 := strings.Repeat("\u00e9", 201)
	if err := svc.RenameTopic(context.Background(), 1, 99, staffPerms, title201, event.Actor{}, ""); !errors.Is(err, ErrInvalidTopic) {
		t.Errorf("201 unicode chars should fail, got %v", err)
	}
}

// --- Moderation hierarchy tests ---

type mockForumGroupRepo struct {
	groups map[int64]*model.Group
}

func (m *mockForumGroupRepo) GetByID(_ context.Context, id int64) (*model.Group, error) {
	if g, ok := m.groups[id]; ok {
		return g, nil
	}
	return nil, sql.ErrNoRows
}
func (m *mockForumGroupRepo) List(_ context.Context) ([]model.Group, error) { return nil, nil }

func TestForumService_ModHierarchy_ModeratorCannotModerateAdminTopic(t *testing.T) {
	// Topic author (user 10) is in admin group (group 1, IsAdmin=true)
	// Acting moderator (user 50) should be denied
	userRepo := &mockForumUserRepo{user: &model.User{ID: 10, GroupID: 1, CanForum: true}}
	groupRepo := &mockForumGroupRepo{groups: map[int64]*model.Group{
		1: {ID: 1, Name: "Administrator", IsAdmin: true},
	}}
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{
		1: {ID: 1, ForumID: 1, UserID: 10},
	}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, userRepo, groupRepo, nil)

	modPerms := model.Permissions{Level: 50, IsModerator: true}

	// LockTopic
	if err := svc.LockTopic(context.Background(), 1, 50, modPerms, event.Actor{ID: 50}, ""); !errors.Is(err, ErrModHierarchyDenied) {
		t.Errorf("LockTopic: expected ErrModHierarchyDenied, got %v", err)
	}

	// UnlockTopic
	if err := svc.UnlockTopic(context.Background(), 1, modPerms, event.Actor{ID: 50}, ""); !errors.Is(err, ErrModHierarchyDenied) {
		t.Errorf("UnlockTopic: expected ErrModHierarchyDenied, got %v", err)
	}

	// PinTopic
	if err := svc.PinTopic(context.Background(), 1, modPerms, event.Actor{ID: 50}, ""); !errors.Is(err, ErrModHierarchyDenied) {
		t.Errorf("PinTopic: expected ErrModHierarchyDenied, got %v", err)
	}

	// UnpinTopic
	if err := svc.UnpinTopic(context.Background(), 1, modPerms, event.Actor{ID: 50}, ""); !errors.Is(err, ErrModHierarchyDenied) {
		t.Errorf("UnpinTopic: expected ErrModHierarchyDenied, got %v", err)
	}

	// MoveTopic
	forumRepo := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1}, 2: {ID: 2}}}
	svc2 := NewForumService(nil, nil, forumRepo, topicRepo, nil, userRepo, groupRepo, nil)
	if err := svc2.MoveTopic(context.Background(), 1, modPerms, 2, event.Actor{ID: 50}, ""); !errors.Is(err, ErrModHierarchyDenied) {
		t.Errorf("MoveTopic: expected ErrModHierarchyDenied, got %v", err)
	}

	// DeleteTopic
	if err := svc.DeleteTopic(context.Background(), 1, 50, modPerms, event.Actor{ID: 50}, ""); !errors.Is(err, ErrModHierarchyDenied) {
		t.Errorf("DeleteTopic: expected ErrModHierarchyDenied, got %v", err)
	}
}

func TestForumService_ModHierarchy_AdminCanModerateModeratorTopic(t *testing.T) {
	// Topic author (user 10) is in moderator group (group 2, IsModerator=true)
	// Acting admin should succeed
	userRepo := &mockForumUserRepo{user: &model.User{ID: 10, GroupID: 2, CanForum: true}}
	groupRepo := &mockForumGroupRepo{groups: map[int64]*model.Group{
		2: {ID: 2, Name: "Moderator", IsModerator: true},
	}}
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{
		1: {ID: 1, ForumID: 1, UserID: 10},
	}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, userRepo, groupRepo, nil)

	adminPerms := model.Permissions{Level: 100, IsAdmin: true}
	if err := svc.LockTopic(context.Background(), 1, 99, adminPerms, event.Actor{ID: 99}, "admin action"); err != nil {
		t.Fatalf("admin should be able to moderate moderator's topic: %v", err)
	}
}

func TestForumService_OwnerSelfLock_WithinGracePeriod(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{
		1: {ID: 1, ForumID: 1, UserID: 5, CreatedAt: time.Now()},
	}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)

	// Owner (userID=5) can lock within 30 min grace period (no staff required)
	ownerPerms := model.Permissions{Level: 5}
	if err := svc.LockTopic(context.Background(), 1, 5, ownerPerms, event.Actor{ID: 5}, ""); err != nil {
		t.Fatalf("owner should be able to self-lock within grace period: %v", err)
	}
}

func TestForumService_OwnerSelfLock_AfterGracePeriod(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{
		1: {ID: 1, ForumID: 1, UserID: 5, CreatedAt: time.Now().Add(-31 * time.Minute)},
	}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)

	// Owner after 30 min grace period should be denied (not staff)
	ownerPerms := model.Permissions{Level: 5}
	if err := svc.LockTopic(context.Background(), 1, 5, ownerPerms, event.Actor{ID: 5}, ""); !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied after grace period, got %v", err)
	}
}

func TestForumService_OwnerSelfDelete_WithinGracePeriod(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{
		1: {ID: 1, ForumID: 1, UserID: 5, CreatedAt: time.Now()},
	}}
	forumRepo := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1}}}
	svc := NewForumService(nil, nil, forumRepo, topicRepo, nil, nil, nil, nil)

	// Owner (userID=5) can delete within 30 min grace period (no staff required)
	ownerPerms := model.Permissions{Level: 5}
	if err := svc.DeleteTopic(context.Background(), 1, 5, ownerPerms, event.Actor{ID: 5}, ""); err != nil {
		t.Fatalf("owner should be able to self-delete within grace period: %v", err)
	}
}

func TestForumService_OwnerSelfDelete_AfterGracePeriod(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{
		1: {ID: 1, ForumID: 1, UserID: 5, CreatedAt: time.Now().Add(-31 * time.Minute)},
	}}
	svc := NewForumService(nil, nil, nil, topicRepo, nil, nil, nil, nil)

	// Owner after 30 min grace period should be denied (not staff)
	ownerPerms := model.Permissions{Level: 5}
	if err := svc.DeleteTopic(context.Background(), 1, 5, ownerPerms, event.Actor{ID: 5}, ""); !errors.Is(err, ErrTopicDeleteDenied) {
		t.Errorf("expected ErrTopicDeleteDenied after grace period, got %v", err)
	}
}

// --- Admin CRUD tests ---

func TestAdminCreateCategory(t *testing.T) {
	bus := event.NewInMemoryBus()
	catRepo := &mockForumCategoryRepo{nextID: 1}
	svc := NewForumService(nil, catRepo, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{CanForum: true}}, nil, bus)

	cat, err := svc.AdminCreateCategory(context.Background(), CreateForumCategoryRequest{Name: "General", SortOrder: 1}, event.Actor{ID: 1, Username: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Name != "General" {
		t.Errorf("expected name General, got %s", cat.Name)
	}
	if cat.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestAdminCreateCategory_EmptyName(t *testing.T) {
	svc := NewForumService(nil, &mockForumCategoryRepo{}, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, nil)

	_, err := svc.AdminCreateCategory(context.Background(), CreateForumCategoryRequest{Name: "  "}, event.Actor{})
	if !errors.Is(err, ErrInvalidForumCategory) {
		t.Errorf("expected ErrInvalidForumCategory, got %v", err)
	}
}

func TestAdminUpdateCategory(t *testing.T) {
	bus := event.NewInMemoryBus()
	catRepo := &mockForumCategoryRepo{
		categories: []model.ForumCategory{{ID: 1, Name: "Old", SortOrder: 0}},
	}
	svc := NewForumService(nil, catRepo, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, bus)

	cat, err := svc.AdminUpdateCategory(context.Background(), 1, UpdateForumCategoryRequest{Name: "Updated", SortOrder: 5}, event.Actor{ID: 1, Username: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Name != "Updated" {
		t.Errorf("expected name Updated, got %s", cat.Name)
	}
	if cat.SortOrder != 5 {
		t.Errorf("expected sort_order 5, got %d", cat.SortOrder)
	}
}

func TestAdminUpdateCategory_NotFound(t *testing.T) {
	svc := NewForumService(nil, &mockForumCategoryRepo{}, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, nil)

	_, err := svc.AdminUpdateCategory(context.Background(), 999, UpdateForumCategoryRequest{Name: "Nope"}, event.Actor{})
	if !errors.Is(err, ErrForumCategoryNotFound) {
		t.Errorf("expected ErrForumCategoryNotFound, got %v", err)
	}
}

func TestAdminDeleteCategory(t *testing.T) {
	bus := event.NewInMemoryBus()
	catRepo := &mockForumCategoryRepo{
		categories: []model.ForumCategory{{ID: 1, Name: "ToDelete", SortOrder: 0}},
	}
	svc := NewForumService(nil, catRepo, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, bus)

	if err := svc.AdminDeleteCategory(context.Background(), 1, event.Actor{ID: 1, Username: "admin"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(catRepo.categories) != 0 {
		t.Errorf("expected 0 categories, got %d", len(catRepo.categories))
	}
}

func TestAdminDeleteCategory_HasForums(t *testing.T) {
	catRepo := &mockForumCategoryRepo{
		categories:  []model.ForumCategory{{ID: 1, Name: "WithForums", SortOrder: 0}},
		forumCounts: map[int64]int64{1: 3},
	}
	svc := NewForumService(nil, catRepo, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, nil)

	err := svc.AdminDeleteCategory(context.Background(), 1, event.Actor{})
	if !errors.Is(err, ErrForumCategoryHasForums) {
		t.Errorf("expected ErrForumCategoryHasForums, got %v", err)
	}
}

func TestAdminCreateForum(t *testing.T) {
	bus := event.NewInMemoryBus()
	catRepo := &mockForumCategoryRepo{categories: []model.ForumCategory{{ID: 1, Name: "General"}}}
	forumRepo := &mockForumRepo{nextID: 1, forumByID: make(map[int64]*model.Forum)}
	svc := NewForumService(nil, catRepo, forumRepo, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, bus)

	forum, err := svc.AdminCreateForum(context.Background(), CreateForumRequest{
		Name: "Announcements", Description: "Site news", CategoryID: 1, SortOrder: 1, MinGroupLevel: 0, MinPostLevel: 5,
	}, event.Actor{ID: 1, Username: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if forum.Name != "Announcements" {
		t.Errorf("expected name Announcements, got %s", forum.Name)
	}
	if forum.MinPostLevel != 5 {
		t.Errorf("expected min_post_level 5, got %d", forum.MinPostLevel)
	}
}

func TestAdminCreateForum_EmptyName(t *testing.T) {
	svc := NewForumService(nil, &mockForumCategoryRepo{categories: []model.ForumCategory{{ID: 1}}}, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, nil)

	_, err := svc.AdminCreateForum(context.Background(), CreateForumRequest{Name: "", CategoryID: 1}, event.Actor{})
	if !errors.Is(err, ErrInvalidForum) {
		t.Errorf("expected ErrInvalidForum, got %v", err)
	}
}

func TestAdminCreateForum_InvalidCategory(t *testing.T) {
	svc := NewForumService(nil, &mockForumCategoryRepo{}, &mockForumRepo{}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, nil)

	_, err := svc.AdminCreateForum(context.Background(), CreateForumRequest{Name: "Test", CategoryID: 999}, event.Actor{})
	if !errors.Is(err, ErrForumCategoryNotFound) {
		t.Errorf("expected ErrForumCategoryNotFound, got %v", err)
	}
}

func TestAdminUpdateForum(t *testing.T) {
	bus := event.NewInMemoryBus()
	catRepo := &mockForumCategoryRepo{categories: []model.ForumCategory{{ID: 1, Name: "General"}}}
	forumRepo := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, Name: "Old", CategoryID: 1}}}
	svc := NewForumService(nil, catRepo, forumRepo, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, bus)

	forum, err := svc.AdminUpdateForum(context.Background(), 1, UpdateForumRequest{
		Name: "Renamed", Description: "New desc", CategoryID: 1, SortOrder: 2,
	}, event.Actor{ID: 1, Username: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if forum.Name != "Renamed" {
		t.Errorf("expected name Renamed, got %s", forum.Name)
	}
}

func TestAdminUpdateForum_NotFound(t *testing.T) {
	svc := NewForumService(nil, &mockForumCategoryRepo{}, &mockForumRepo{forumByID: map[int64]*model.Forum{}}, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, nil)

	_, err := svc.AdminUpdateForum(context.Background(), 999, UpdateForumRequest{Name: "Nope", CategoryID: 1}, event.Actor{})
	if !errors.Is(err, ErrForumNotFound) {
		t.Errorf("expected ErrForumNotFound, got %v", err)
	}
}

func TestAdminDeleteForum(t *testing.T) {
	bus := event.NewInMemoryBus()
	forumRepo := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, Name: "ToDelete"}}}
	svc := NewForumService(nil, &mockForumCategoryRepo{}, forumRepo, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, bus)

	if err := svc.AdminDeleteForum(context.Background(), 1, event.Actor{ID: 1, Username: "admin"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdminDeleteForum_HasTopics(t *testing.T) {
	forumRepo := &mockForumRepo{
		forumByID:   map[int64]*model.Forum{1: {ID: 1, Name: "WithTopics"}},
		topicCounts: map[int64]int64{1: 5},
	}
	svc := NewForumService(nil, &mockForumCategoryRepo{}, forumRepo, &mockForumTopicRepo{}, &mockForumPostRepo{}, &mockForumUserRepo{user: &model.User{}}, nil, nil)

	err := svc.AdminDeleteForum(context.Background(), 1, event.Actor{})
	if !errors.Is(err, ErrForumHasTopics) {
		t.Errorf("expected ErrForumHasTopics, got %v", err)
	}
}

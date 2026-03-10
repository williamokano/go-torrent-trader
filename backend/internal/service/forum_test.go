package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- Mock repositories ---

type mockForumCategoryRepo struct {
	categories []model.ForumCategory
	err        error
}

func (m *mockForumCategoryRepo) List(_ context.Context) ([]model.ForumCategory, error) {
	return m.categories, m.err
}

type mockForumRepo struct {
	forums       []model.Forum
	forumByID    map[int64]*model.Forum
	listErr      error
	getErr       error
}

func (m *mockForumRepo) GetByID(_ context.Context, id int64) (*model.Forum, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if f, ok := m.forumByID[id]; ok {
		return f, nil
	}
	return nil, sql.ErrNoRows
}
func (m *mockForumRepo) ListByCategory(_ context.Context, _ int64) ([]model.Forum, error) {
	return m.forums, m.listErr
}
func (m *mockForumRepo) List(_ context.Context) ([]model.Forum, error) {
	return m.forums, m.listErr
}
func (m *mockForumRepo) IncrementTopicCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error  { return nil }
func (m *mockForumRepo) UpdateLastPost(_ context.Context, _ int64, _ int64) error    { return nil }

type mockForumTopicRepo struct {
	topics    []model.ForumTopic
	topicByID map[int64]*model.ForumTopic
	total     int64
	created   *model.ForumTopic
	listErr   error
	getErr    error
}

func (m *mockForumTopicRepo) GetByID(_ context.Context, id int64) (*model.ForumTopic, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if t, ok := m.topicByID[id]; ok {
		return t, nil
	}
	return nil, sql.ErrNoRows
}
func (m *mockForumTopicRepo) ListByForum(_ context.Context, _ int64, _, _ int) ([]model.ForumTopic, int64, error) {
	return m.topics, m.total, m.listErr
}
func (m *mockForumTopicRepo) Create(_ context.Context, topic *model.ForumTopic) error {
	topic.ID = 100
	topic.CreatedAt = time.Now()
	topic.UpdatedAt = time.Now()
	m.created = topic
	return nil
}
func (m *mockForumTopicRepo) IncrementViewCount(_ context.Context, _ int64) error                     { return nil }
func (m *mockForumTopicRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error              { return nil }
func (m *mockForumTopicRepo) UpdateLastPost(_ context.Context, _ int64, _ int64, _ time.Time) error { return nil }

type mockForumPostRepo struct {
	posts   []model.ForumPost
	postByID map[int64]*model.ForumPost
	total   int64
	listErr error
	getErr  error
}

func (m *mockForumPostRepo) GetByID(_ context.Context, id int64) (*model.ForumPost, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if p, ok := m.postByID[id]; ok {
		return p, nil
	}
	return nil, sql.ErrNoRows
}
func (m *mockForumPostRepo) ListByTopic(_ context.Context, _ int64, _, _ int) ([]model.ForumPost, int64, error) {
	return m.posts, m.total, m.listErr
}
func (m *mockForumPostRepo) Create(_ context.Context, post *model.ForumPost) error {
	post.ID = 200
	post.CreatedAt = time.Now()
	return nil
}
func (m *mockForumPostRepo) CountByUser(_ context.Context, _ int64) (int, error) {
	return 0, nil
}

type mockForumUserRepo struct {
	user *model.User
	err  error
}

func (m *mockForumUserRepo) GetByID(_ context.Context, _ int64) (*model.User, error) {
	return m.user, m.err
}
func (m *mockForumUserRepo) GetByUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockForumUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockForumUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockForumUserRepo) Count(_ context.Context) (int64, error) { return 0, nil }
func (m *mockForumUserRepo) Create(_ context.Context, _ *model.User) error { return nil }
func (m *mockForumUserRepo) Update(_ context.Context, _ *model.User) error { return nil }
func (m *mockForumUserRepo) IncrementStats(_ context.Context, _ int64, _, _ int64) error { return nil }
func (m *mockForumUserRepo) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockForumUserRepo) ListStaff(_ context.Context) ([]model.User, error) { return nil, nil }
func (m *mockForumUserRepo) UpdateLastAccess(_ context.Context, _ int64) error  { return nil }

// --- Tests ---

func TestForumService_ListCategories(t *testing.T) {
	catRepo := &mockForumCategoryRepo{
		categories: []model.ForumCategory{
			{ID: 1, Name: "General", SortOrder: 1},
			{ID: 2, Name: "Empty", SortOrder: 2},
		},
	}
	forumRepo := &mockForumRepo{
		forums: []model.Forum{
			{ID: 1, CategoryID: 1, Name: "Announcements", MinGroupLevel: 0},
			{ID: 2, CategoryID: 1, Name: "VIP Only", MinGroupLevel: 100},
		},
	}
	svc := NewForumService(catRepo, forumRepo, nil, nil, nil)

	// Normal user (level 5)
	perms := model.Permissions{Level: 5}
	cats, err := svc.ListCategories(context.Background(), perms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 category with 1 forum (VIP forum filtered, Empty category excluded)
	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	if len(cats[0].Forums) != 1 {
		t.Fatalf("expected 1 forum, got %d", len(cats[0].Forums))
	}
	if cats[0].Forums[0].Name != "Announcements" {
		t.Errorf("expected Announcements, got %s", cats[0].Forums[0].Name)
	}

	// Admin (level 200) should see VIP forum too
	adminPerms := model.Permissions{Level: 200}
	cats, err = svc.ListCategories(context.Background(), adminPerms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	if len(cats[0].Forums) != 2 {
		t.Fatalf("expected 2 forums, got %d", len(cats[0].Forums))
	}
}

func TestForumService_GetForum_AccessDenied(t *testing.T) {
	forumRepo := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			1: {ID: 1, Name: "VIP", MinGroupLevel: 100},
		},
	}
	svc := NewForumService(nil, forumRepo, nil, nil, nil)

	_, err := svc.GetForum(context.Background(), 1, model.Permissions{Level: 5})
	if !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}

	// With sufficient level
	forum, err := svc.GetForum(context.Background(), 1, model.Permissions{Level: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if forum.Name != "VIP" {
		t.Errorf("expected VIP, got %s", forum.Name)
	}
}

func TestForumService_GetForum_NotFound(t *testing.T) {
	forumRepo := &mockForumRepo{
		forumByID: map[int64]*model.Forum{},
	}
	svc := NewForumService(nil, forumRepo, nil, nil, nil)

	_, err := svc.GetForum(context.Background(), 999, model.Permissions{Level: 100})
	if !errors.Is(err, ErrForumNotFound) {
		t.Errorf("expected ErrForumNotFound, got %v", err)
	}
}

func TestForumService_CreateTopic_Success(t *testing.T) {
	forumRepo := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			1: {ID: 1, Name: "General", MinGroupLevel: 0},
		},
	}
	topicRepo := &mockForumTopicRepo{}
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			200: {ID: 200, Username: "alice", GroupName: "User"},
		},
	}
	userRepo := &mockForumUserRepo{
		user: &model.User{ID: 1, CanForum: true},
	}

	svc := NewForumService(nil, forumRepo, topicRepo, postRepo, userRepo)

	perms := model.Permissions{Level: 5}
	topic, post, err := svc.CreateTopic(context.Background(), 1, 1, perms, "Test Topic", "Hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic.Title != "Test Topic" {
		t.Errorf("expected title 'Test Topic', got '%s'", topic.Title)
	}
	if post == nil {
		t.Fatal("expected post to be non-nil")
	}
}

func TestForumService_CreateTopic_EmptyTitle(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil)

	_, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "", "body")
	if !errors.Is(err, ErrInvalidTopic) {
		t.Errorf("expected ErrInvalidTopic, got %v", err)
	}
}

func TestForumService_CreateTopic_EmptyBody(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil)

	_, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "Title", "")
	if !errors.Is(err, ErrInvalidPost) {
		t.Errorf("expected ErrInvalidPost, got %v", err)
	}
}

func TestForumService_CreateTopic_UserCanForumFalse(t *testing.T) {
	forumRepo := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			1: {ID: 1, Name: "General", MinGroupLevel: 0},
		},
	}
	userRepo := &mockForumUserRepo{
		user: &model.User{ID: 1, CanForum: false},
	}
	svc := NewForumService(nil, forumRepo, nil, nil, userRepo)

	_, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 5}, "Title", "Body")
	if !errors.Is(err, ErrForumAccessDenied) {
		t.Errorf("expected ErrForumAccessDenied, got %v", err)
	}
}

func TestForumService_CreatePost_TopicLocked(t *testing.T) {
	topicRepo := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{
			1: {ID: 1, ForumID: 1, Locked: true},
		},
	}
	forumRepo := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			1: {ID: 1, Name: "General", MinGroupLevel: 0},
		},
	}
	svc := NewForumService(nil, forumRepo, topicRepo, nil, nil)

	_, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", nil)
	if !errors.Is(err, ErrTopicLocked) {
		t.Errorf("expected ErrTopicLocked, got %v", err)
	}
}

func TestForumService_CreatePost_Success(t *testing.T) {
	topicRepo := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{
			1: {ID: 1, ForumID: 1, Locked: false},
		},
	}
	forumRepo := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			1: {ID: 1, Name: "General", MinGroupLevel: 0},
		},
	}
	postRepo := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			200: {ID: 200, Username: "alice", GroupName: "User"},
		},
	}
	userRepo := &mockForumUserRepo{
		user: &model.User{ID: 1, CanForum: true},
	}

	svc := NewForumService(nil, forumRepo, topicRepo, postRepo, userRepo)

	post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply body", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post == nil {
		t.Fatal("expected post to be non-nil")
	}
}

func TestForumService_CreatePost_EmptyBody(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil)

	_, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{}, "", nil)
	if !errors.Is(err, ErrInvalidPost) {
		t.Errorf("expected ErrInvalidPost, got %v", err)
	}
}

func TestForumService_GetTopic_NotFound(t *testing.T) {
	topicRepo := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{},
	}
	svc := NewForumService(nil, nil, topicRepo, nil, nil)

	_, err := svc.GetTopic(context.Background(), 999, model.Permissions{Level: 5})
	if !errors.Is(err, ErrTopicNotFound) {
		t.Errorf("expected ErrTopicNotFound, got %v", err)
	}
}

func TestForumService_ListTopics(t *testing.T) {
	forumRepo := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			1: {ID: 1, Name: "General", MinGroupLevel: 0},
		},
	}
	topicRepo := &mockForumTopicRepo{
		topics: []model.ForumTopic{
			{ID: 1, Title: "Hello", Pinned: true},
			{ID: 2, Title: "World"},
		},
		total: 2,
	}

	svc := NewForumService(nil, forumRepo, topicRepo, nil, nil)

	forum, topics, total, err := svc.ListTopics(context.Background(), 1, model.Permissions{Level: 5}, 1, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if forum.Name != "General" {
		t.Errorf("expected General, got %s", forum.Name)
	}
	if len(topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(topics))
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
}

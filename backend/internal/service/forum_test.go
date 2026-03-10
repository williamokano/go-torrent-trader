package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

type mockForumCategoryRepo struct{ categories []model.ForumCategory; err error }
func (m *mockForumCategoryRepo) List(_ context.Context) ([]model.ForumCategory, error) { return m.categories, m.err }

type mockForumRepo struct{ forums []model.Forum; forumByID map[int64]*model.Forum; listErr, getErr error }
func (m *mockForumRepo) GetByID(_ context.Context, id int64) (*model.Forum, error) { if m.getErr != nil { return nil, m.getErr }; if f, ok := m.forumByID[id]; ok { return f, nil }; return nil, sql.ErrNoRows }
func (m *mockForumRepo) ListByCategory(_ context.Context, _ int64) ([]model.Forum, error) { return m.forums, m.listErr }
func (m *mockForumRepo) List(_ context.Context) ([]model.Forum, error) { return m.forums, m.listErr }
func (m *mockForumRepo) IncrementTopicCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumRepo) UpdateLastPost(_ context.Context, _ int64, _ int64) error { return nil }

type mockForumTopicRepo struct{ topics []model.ForumTopic; topicByID map[int64]*model.ForumTopic; total int64; created *model.ForumTopic; listErr, getErr error }
func (m *mockForumTopicRepo) GetByID(_ context.Context, id int64) (*model.ForumTopic, error) { if m.getErr != nil { return nil, m.getErr }; if t, ok := m.topicByID[id]; ok { return t, nil }; return nil, sql.ErrNoRows }
func (m *mockForumTopicRepo) ListByForum(_ context.Context, _ int64, _, _ int) ([]model.ForumTopic, int64, error) { return m.topics, m.total, m.listErr }
func (m *mockForumTopicRepo) Create(_ context.Context, topic *model.ForumTopic) error { topic.ID = 100; topic.CreatedAt = time.Now(); topic.UpdatedAt = time.Now(); m.created = topic; return nil }
func (m *mockForumTopicRepo) IncrementViewCount(_ context.Context, _ int64) error { return nil }
func (m *mockForumTopicRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumTopicRepo) UpdateLastPost(_ context.Context, _ int64, _ int64, _ time.Time) error { return nil }

type mockForumPostRepo struct{ posts []model.ForumPost; postByID map[int64]*model.ForumPost; total int64; listErr, getErr error; searchResults []model.ForumSearchResult; searchTotal int64; searchErr error }
func (m *mockForumPostRepo) GetByID(_ context.Context, id int64) (*model.ForumPost, error) { if m.getErr != nil { return nil, m.getErr }; if p, ok := m.postByID[id]; ok { return p, nil }; return nil, sql.ErrNoRows }
func (m *mockForumPostRepo) ListByTopic(_ context.Context, _ int64, _, _ int) ([]model.ForumPost, int64, error) { return m.posts, m.total, m.listErr }
func (m *mockForumPostRepo) Create(_ context.Context, post *model.ForumPost) error { post.ID = 200; post.CreatedAt = time.Now(); return nil }
func (m *mockForumPostRepo) CountByUser(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *mockForumPostRepo) Search(_ context.Context, _ string, _ *int64, _ int, _, _ int) ([]model.ForumSearchResult, int64, error) { return m.searchResults, m.searchTotal, m.searchErr }

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
	svc := NewForumService(nil, &mockForumCategoryRepo{categories: []model.ForumCategory{{ID: 1, Name: "General", SortOrder: 1}, {ID: 2, Name: "Empty", SortOrder: 2}}}, &mockForumRepo{forums: []model.Forum{{ID: 1, CategoryID: 1, Name: "Announcements", MinGroupLevel: 0}, {ID: 2, CategoryID: 1, Name: "VIP Only", MinGroupLevel: 100}}}, nil, nil, nil)
	cats, err := svc.ListCategories(context.Background(), model.Permissions{Level: 5})
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if len(cats) != 1 { t.Fatalf("expected 1 category, got %d", len(cats)) }
	if len(cats[0].Forums) != 1 { t.Fatalf("expected 1 forum, got %d", len(cats[0].Forums)) }
	cats, err = svc.ListCategories(context.Background(), model.Permissions{Level: 200})
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if len(cats[0].Forums) != 2 { t.Fatalf("expected 2 forums, got %d", len(cats[0].Forums)) }
}

func TestForumService_GetForum_AccessDenied(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, Name: "VIP", MinGroupLevel: 100}}}, nil, nil, nil)
	if _, err := svc.GetForum(context.Background(), 1, model.Permissions{Level: 5}); !errors.Is(err, ErrForumAccessDenied) { t.Errorf("expected ErrForumAccessDenied, got %v", err) }
	if f, err := svc.GetForum(context.Background(), 1, model.Permissions{Level: 100}); err != nil { t.Fatalf("unexpected: %v", err) } else if f.Name != "VIP" { t.Errorf("expected VIP") }
}

func TestForumService_GetForum_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{}}, nil, nil, nil)
	if _, err := svc.GetForum(context.Background(), 999, model.Permissions{Level: 100}); !errors.Is(err, ErrForumNotFound) { t.Errorf("expected ErrForumNotFound, got %v", err) }
}

func TestForumService_CreateTopic_Success(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, Username: "alice", GroupName: "User"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}})
	topic, post, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 5}, "Test Topic", "Hello world")
	if err != nil { t.Fatalf("unexpected: %v", err) }
	if topic.Title != "Test Topic" { t.Errorf("wrong title") }
	if post == nil { t.Fatal("nil post") }
}

func TestForumService_CreateTopic_EmptyTitle(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "", "body"); !errors.Is(err, ErrInvalidTopic) { t.Errorf("expected ErrInvalidTopic") }
}

func TestForumService_CreateTopic_EmptyBody(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "Title", ""); !errors.Is(err, ErrInvalidPost) { t.Errorf("expected ErrInvalidPost") }
}

func TestForumService_CreateTopic_UserCanForumFalse(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, nil, nil, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: false}})
	if _, _, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{Level: 5}, "Title", "Body"); !errors.Is(err, ErrForumAccessDenied) { t.Errorf("expected ErrForumAccessDenied") }
}

func TestForumService_CreatePost_TopicLocked(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, Locked: true}}}, nil, nil)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", nil); !errors.Is(err, ErrTopicLocked) { t.Errorf("expected ErrTopicLocked") }
}

func TestForumService_CreatePost_Success(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: {ID: 200, TopicID: 1, Username: "alice", GroupName: "User"}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}})
	post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply body", nil)
	if err != nil { t.Fatalf("unexpected: %v", err) }
	if post == nil { t.Fatal("nil post") }
}

func TestForumService_CreatePost_EmptyBody(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{}, "", nil); !errors.Is(err, ErrInvalidPost) { t.Errorf("expected ErrInvalidPost") }
}

func TestForumService_CreatePost_CanForumFalse(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, nil, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: false}})
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply body", nil); !errors.Is(err, ErrForumAccessDenied) { t.Errorf("expected ErrForumAccessDenied, got %v", err) }
}

func TestForumService_CreatePost_InvalidReplyToPostID_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}})
	replyTo := int64(999)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", &replyTo); !errors.Is(err, ErrInvalidReply) { t.Errorf("expected ErrInvalidReply, got %v", err) }
}

func TestForumService_CreatePost_InvalidReplyToPostID_DifferentTopic(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{50: {ID: 50, TopicID: 2}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}})
	replyTo := int64(50)
	if _, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", &replyTo); !errors.Is(err, ErrInvalidReply) { t.Errorf("expected ErrInvalidReply, got %v", err) }
}

func TestForumService_CreatePost_ValidReplyToPostID(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}, &mockForumPostRepo{postByID: map[int64]*model.ForumPost{50: {ID: 50, TopicID: 1}, 200: {ID: 200, TopicID: 1}}}, &mockForumUserRepo{user: &model.User{ID: 1, CanForum: true}})
	replyTo := int64(50)
	if post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{Level: 5}, "Reply", &replyTo); err != nil { t.Fatalf("unexpected: %v", err) } else if post == nil { t.Fatal("nil post") }
}

func TestForumService_GetTopic_NotFound(t *testing.T) {
	svc := NewForumService(nil, nil, nil, &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{}}, nil, nil)
	if _, err := svc.GetTopic(context.Background(), 999, 1, model.Permissions{Level: 5}); !errors.Is(err, ErrTopicNotFound) { t.Errorf("expected ErrTopicNotFound") }
}

func TestForumService_GetTopic_ViewCountDebounce(t *testing.T) {
	topicRepo := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1, ViewCount: 10}}}
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}, topicRepo, nil, nil)
	topic, _ := svc.GetTopic(context.Background(), 1, 1, model.Permissions{Level: 5})
	if topic.ViewCount != 11 { t.Errorf("expected 11, got %d", topic.ViewCount) }
	topicRepo.topicByID[1].ViewCount = 10
	topic, _ = svc.GetTopic(context.Background(), 1, 1, model.Permissions{Level: 5})
	if topic.ViewCount != 10 { t.Errorf("expected 10 (debounce), got %d", topic.ViewCount) }
	topic, _ = svc.GetTopic(context.Background(), 1, 2, model.Permissions{Level: 5})
	if topic.ViewCount != 11 { t.Errorf("expected 11 for diff user, got %d", topic.ViewCount) }
}

func TestForumService_ListTopics(t *testing.T) {
	svc := NewForumService(nil, nil, &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, Name: "General", MinGroupLevel: 0}}}, &mockForumTopicRepo{topics: []model.ForumTopic{{ID: 1, Title: "Hello", Pinned: true}, {ID: 2, Title: "World"}}, total: 2}, nil, nil)
	forum, topics, total, err := svc.ListTopics(context.Background(), 1, model.Permissions{Level: 5}, 1, 25)
	if err != nil { t.Fatalf("unexpected: %v", err) }
	if forum.Name != "General" { t.Errorf("wrong name") }
	if len(topics) != 2 { t.Errorf("expected 2 topics") }
	if total != 2 { t.Errorf("expected total 2") }
}

func TestForumService_Search_Success(t *testing.T) {
	results := []model.ForumSearchResult{{PostID: 1, Body: "hello world", TopicID: 10, TopicTitle: "Greetings", ForumID: 1, ForumName: "General", UserID: 1, Username: "alice"}}
	svc := NewForumService(nil, nil, nil, nil, &mockForumPostRepo{searchResults: results, searchTotal: 1}, nil)
	got, total, err := svc.Search(context.Background(), "hello", model.Permissions{Level: 5}, nil, 1, 25)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if total != 1 { t.Errorf("expected total 1, got %d", total) }
	if len(got) != 1 { t.Errorf("expected 1 result, got %d", len(got)) }
	if got[0].PostID != 1 { t.Errorf("expected post ID 1") }
}

func TestForumService_Search_EmptyQuery(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.Search(context.Background(), "", model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch, got %v", err) }
	if _, _, err := svc.Search(context.Background(), "   ", model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch for whitespace-only query, got %v", err) }
}

func TestForumService_Search_QueryTooShort(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil)
	if _, _, err := svc.Search(context.Background(), "a", model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch for single-char query, got %v", err) }
}

func TestForumService_Search_QueryTooLong(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, nil, nil)
	longQuery := strings.Repeat("a", 201)
	if _, _, err := svc.Search(context.Background(), longQuery, model.Permissions{Level: 5}, nil, 1, 25); !errors.Is(err, ErrInvalidSearch) { t.Errorf("expected ErrInvalidSearch, got %v", err) }
}

func TestForumService_Search_PaginationClamping(t *testing.T) {
	svc := NewForumService(nil, nil, nil, nil, &mockForumPostRepo{searchResults: nil, searchTotal: 0}, nil)
	// Negative page and perPage should be clamped, not cause errors
	_, _, err := svc.Search(context.Background(), "test", model.Permissions{Level: 5}, nil, -1, -1)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
}

func TestForumService_Search_WithForumFilter(t *testing.T) {
	results := []model.ForumSearchResult{{PostID: 5, Body: "filtered result", TopicID: 20, TopicTitle: "Topic", ForumID: 2, ForumName: "Support", UserID: 1, Username: "bob"}}
	svc := NewForumService(nil, nil, nil, nil, &mockForumPostRepo{searchResults: results, searchTotal: 1}, nil)
	forumID := int64(2)
	got, total, err := svc.Search(context.Background(), "filtered", model.Permissions{Level: 5}, &forumID, 1, 25)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if total != 1 { t.Errorf("expected total 1, got %d", total) }
	if len(got) != 1 { t.Errorf("expected 1 result") }
}

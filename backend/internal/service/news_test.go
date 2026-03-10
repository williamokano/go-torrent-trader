package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// mockNewsRepo is an in-memory news repository for service tests.
type mockNewsRepo struct {
	mu       sync.Mutex
	articles []*model.NewsArticle
	nextID   int64
}

func newMockNewsRepo() *mockNewsRepo {
	return &mockNewsRepo{nextID: 1}
}

func (m *mockNewsRepo) Create(_ context.Context, a *model.NewsArticle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a.ID = m.nextID
	m.nextID++
	m.articles = append(m.articles, a)
	return nil
}

func (m *mockNewsRepo) GetByID(_ context.Context, id int64) (*model.NewsArticle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.articles {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockNewsRepo) GetPublishedByID(_ context.Context, id int64) (*model.NewsArticle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.articles {
		if a.ID == id && a.Published {
			return a, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockNewsRepo) Update(_ context.Context, a *model.NewsArticle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.articles {
		if existing.ID == a.ID {
			m.articles[i] = a
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockNewsRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.articles {
		if a.ID == id {
			m.articles = append(m.articles[:i], m.articles[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockNewsRepo) List(_ context.Context, opts repository.ListNewsOptions) ([]model.NewsArticle, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.NewsArticle
	for _, a := range m.articles {
		if opts.Published != nil && a.Published != *opts.Published {
			continue
		}
		result = append(result, *a)
	}
	return result, int64(len(result)), nil
}

func (m *mockNewsRepo) ListPublished(_ context.Context, _, _ int) ([]model.NewsArticle, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.NewsArticle
	for _, a := range m.articles {
		if a.Published {
			result = append(result, *a)
		}
	}
	return result, int64(len(result)), nil
}

// mockNewsUserRepo is a minimal user repo for news service tests.
type mockNewsUserRepo struct {
	mu    sync.Mutex
	users []*model.User
}

func newMockNewsUserRepo() *mockNewsUserRepo {
	return &mockNewsUserRepo{
		users: []*model.User{
			{ID: 1, Username: "admin"},
		},
	}
}

func (m *mockNewsUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockNewsUserRepo) GetByUsername(context.Context, string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockNewsUserRepo) GetByEmail(context.Context, string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockNewsUserRepo) GetByPasskey(context.Context, string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockNewsUserRepo) Count(context.Context) (int64, error) { return 0, nil }
func (m *mockNewsUserRepo) Create(context.Context, *model.User) error {
	return errors.New("not implemented")
}
func (m *mockNewsUserRepo) Update(context.Context, *model.User) error { return nil }
func (m *mockNewsUserRepo) IncrementStats(context.Context, int64, int64, int64) error {
	return nil
}
func (m *mockNewsUserRepo) List(context.Context, repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockNewsUserRepo) ListStaff(context.Context) ([]model.User, error) { return nil, nil }
func (m *mockNewsUserRepo) UpdateLastAccess(context.Context, int64) error    { return nil }

func TestNewsService_Create(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	article, err := svc.Create(context.Background(), CreateNewsRequest{
		Title:     "Test Article",
		Body:      "Test body content",
		Published: false,
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if article.Title != "Test Article" {
		t.Errorf("expected title 'Test Article', got %s", article.Title)
	}
	if article.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if article.Published {
		t.Error("expected unpublished")
	}
}

func TestNewsService_Create_EmptyTitle(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	_, err := svc.Create(context.Background(), CreateNewsRequest{
		Title: "",
		Body:  "Some body",
	}, 1)
	if !errors.Is(err, ErrInvalidNews) {
		t.Errorf("expected ErrInvalidNews, got %v", err)
	}
}

func TestNewsService_Create_EmptyBody(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	_, err := svc.Create(context.Background(), CreateNewsRequest{
		Title: "Title",
		Body:  "",
	}, 1)
	if !errors.Is(err, ErrInvalidNews) {
		t.Errorf("expected ErrInvalidNews, got %v", err)
	}
}

func TestNewsService_Create_TitleTooLong(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	longTitle := make([]byte, 501)
	for i := range longTitle {
		longTitle[i] = 'a'
	}

	_, err := svc.Create(context.Background(), CreateNewsRequest{
		Title: string(longTitle),
		Body:  "Some body",
	}, 1)
	if !errors.Is(err, ErrInvalidNews) {
		t.Errorf("expected ErrInvalidNews for long title, got %v", err)
	}
}

func TestNewsService_Create_BodyTooLong(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	longBody := make([]byte, 50001)
	for i := range longBody {
		longBody[i] = 'a'
	}

	_, err := svc.Create(context.Background(), CreateNewsRequest{
		Title: "Valid Title",
		Body:  string(longBody),
	}, 1)
	if !errors.Is(err, ErrInvalidNews) {
		t.Errorf("expected ErrInvalidNews for long body, got %v", err)
	}
}

func TestNewsService_Create_Published_EmitsEvent(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()

	var published bool
	bus.Subscribe(event.NewsPublished, func(_ context.Context, evt event.Event) error {
		e := evt.(*event.NewsPublishedEvent)
		if e.Title == "Breaking News" {
			published = true
		}
		return nil
	})

	svc := NewNewsService(newsRepo, userRepo, bus)
	_, err := svc.Create(context.Background(), CreateNewsRequest{
		Title:     "Breaking News",
		Body:      "Something happened",
		Published: true,
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !published {
		t.Error("expected NewsPublished event to be emitted")
	}
}

func TestNewsService_Update(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	article, _ := svc.Create(context.Background(), CreateNewsRequest{
		Title: "Original", Body: "Original body",
	}, 1)

	updated, err := svc.Update(context.Background(), article.ID, UpdateNewsRequest{
		Title:     "Updated",
		Body:      "Updated body",
		Published: true,
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Title != "Updated" {
		t.Errorf("expected title 'Updated', got %s", updated.Title)
	}
	if !updated.Published {
		t.Error("expected published to be true")
	}
}

func TestNewsService_Update_NotFound(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	_, err := svc.Update(context.Background(), 999, UpdateNewsRequest{
		Title: "Nope", Body: "Nope",
	}, 1)
	if !errors.Is(err, ErrNewsNotFound) {
		t.Errorf("expected ErrNewsNotFound, got %v", err)
	}
}

func TestNewsService_Delete(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	article, _ := svc.Create(context.Background(), CreateNewsRequest{
		Title: "To Delete", Body: "Content",
	}, 1)

	err := svc.Delete(context.Background(), article.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewsService_Delete_NotFound(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	err := svc.Delete(context.Background(), 999)
	if !errors.Is(err, ErrNewsNotFound) {
		t.Errorf("expected ErrNewsNotFound, got %v", err)
	}
}

func TestNewsService_ListPublished(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	_, _ = svc.Create(context.Background(), CreateNewsRequest{
		Title: "Published", Body: "Content", Published: true,
	}, 1)
	_, _ = svc.Create(context.Background(), CreateNewsRequest{
		Title: "Draft", Body: "Content", Published: false,
	}, 1)

	articles, total, err := svc.ListPublished(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 published article, got %d", total)
	}
	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}
	if articles[0].Title != "Published" {
		t.Errorf("expected title 'Published', got %s", articles[0].Title)
	}
}

func TestNewsService_GetPublished(t *testing.T) {
	newsRepo := newMockNewsRepo()
	userRepo := newMockNewsUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewNewsService(newsRepo, userRepo, bus)

	published, _ := svc.Create(context.Background(), CreateNewsRequest{
		Title: "Public", Body: "Content", Published: true,
	}, 1)
	draft, _ := svc.Create(context.Background(), CreateNewsRequest{
		Title: "Draft", Body: "Content", Published: false,
	}, 1)

	// Should get the published article
	article, err := svc.GetPublished(context.Background(), published.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if article.Title != "Public" {
		t.Errorf("expected title 'Public', got %s", article.Title)
	}

	// Should NOT get the draft
	_, err = svc.GetPublished(context.Background(), draft.ID)
	if !errors.Is(err, ErrNewsNotFound) {
		t.Errorf("expected ErrNewsNotFound for draft, got %v", err)
	}
}

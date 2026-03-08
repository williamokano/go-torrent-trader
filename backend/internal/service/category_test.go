package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// mockCategoryRepo is an in-memory category repository for service tests.
type mockCategoryRepo struct {
	mu             sync.Mutex
	categories     []*model.Category
	nextID         int64
	torrentCounts  map[int64]int64
}

func newMockCategoryRepo() *mockCategoryRepo {
	return &mockCategoryRepo{
		nextID:        1,
		torrentCounts: make(map[int64]int64),
	}
}

func (m *mockCategoryRepo) GetByID(_ context.Context, id int64) (*model.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.categories {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockCategoryRepo) List(_ context.Context) ([]model.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Category
	for _, c := range m.categories {
		result = append(result, *c)
	}
	return result, nil
}

func (m *mockCategoryRepo) Create(_ context.Context, cat *model.Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cat.ID = m.nextID
	m.nextID++
	m.categories = append(m.categories, cat)
	return nil
}

func (m *mockCategoryRepo) Update(_ context.Context, cat *model.Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.categories {
		if c.ID == cat.ID {
			m.categories[i] = cat
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockCategoryRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.categories {
		if c.ID == id {
			m.categories = append(m.categories[:i], m.categories[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockCategoryRepo) CountTorrentsByCategory(_ context.Context, categoryID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.torrentCounts[categoryID], nil
}

func TestCategoryService_List(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	// Create some categories
	_, _ = svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies", SortOrder: 1})
	_, _ = svc.Create(context.Background(), CreateCategoryRequest{Name: "Music", SortOrder: 2})

	cats, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cats))
	}
}

func TestCategoryService_Create(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	cat, err := svc.Create(context.Background(), CreateCategoryRequest{
		Name:      "Movies",
		SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Name != "Movies" {
		t.Errorf("expected name Movies, got %s", cat.Name)
	}
	if cat.Slug != "movies" {
		t.Errorf("expected slug movies, got %s", cat.Slug)
	}
	if cat.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestCategoryService_Create_WithCustomSlug(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	cat, err := svc.Create(context.Background(), CreateCategoryRequest{
		Name: "TV Shows",
		Slug: "tv-shows",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Slug != "tv-shows" {
		t.Errorf("expected slug tv-shows, got %s", cat.Slug)
	}
}

func TestCategoryService_Create_EmptyName(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	_, err := svc.Create(context.Background(), CreateCategoryRequest{Name: ""})
	if !errors.Is(err, ErrInvalidCategory) {
		t.Errorf("expected ErrInvalidCategory, got %v", err)
	}
}

func TestCategoryService_Create_WithParentID(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	parent, _ := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies", SortOrder: 1})

	parentID := parent.ID
	child, err := svc.Create(context.Background(), CreateCategoryRequest{
		Name:     "HD",
		ParentID: &parentID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != parentID {
		t.Errorf("expected parent_id %d, got %v", parentID, child.ParentID)
	}
}

func TestCategoryService_Update(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	cat, _ := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies", SortOrder: 1})

	updated, err := svc.Update(context.Background(), cat.ID, UpdateCategoryRequest{
		Name:      "Films",
		Slug:      "films",
		SortOrder: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "Films" {
		t.Errorf("expected name Films, got %s", updated.Name)
	}
	if updated.Slug != "films" {
		t.Errorf("expected slug films, got %s", updated.Slug)
	}
	if updated.SortOrder != 2 {
		t.Errorf("expected sort_order 2, got %d", updated.SortOrder)
	}
}

func TestCategoryService_Update_NotFound(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	_, err := svc.Update(context.Background(), 999, UpdateCategoryRequest{Name: "Nope"})
	if !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got %v", err)
	}
}

func TestCategoryService_Update_EmptyName(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	cat, _ := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies", SortOrder: 1})

	_, err := svc.Update(context.Background(), cat.ID, UpdateCategoryRequest{Name: ""})
	if !errors.Is(err, ErrInvalidCategory) {
		t.Errorf("expected ErrInvalidCategory, got %v", err)
	}
}

func TestCategoryService_Delete(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	cat, _ := svc.Create(context.Background(), CreateCategoryRequest{Name: "Temp", SortOrder: 99})

	err := svc.Delete(context.Background(), cat.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cats, _ := svc.List(context.Background())
	if len(cats) != 0 {
		t.Errorf("expected 0 categories after delete, got %d", len(cats))
	}
}

func TestCategoryService_Delete_NotFound(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	err := svc.Delete(context.Background(), 999)
	if !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got %v", err)
	}
}

func TestCategoryService_Delete_HasTorrents(t *testing.T) {
	repo := newMockCategoryRepo()
	svc := NewCategoryService(repo)

	cat, _ := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies", SortOrder: 1})

	// Simulate torrents in this category
	repo.mu.Lock()
	repo.torrentCounts[cat.ID] = 5
	repo.mu.Unlock()

	err := svc.Delete(context.Background(), cat.ID)
	if !errors.Is(err, ErrCategoryHasTorrents) {
		t.Errorf("expected ErrCategoryHasTorrents, got %v", err)
	}
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"Movies", "movies"},
		{"TV Shows", "tv-shows"},
		{"PC Games", "pc-games"},
		{"  Spaces  ", "spaces"},
		{"Already-Slugged", "already-slugged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateSlug(tt.name)
			if got != tt.expected {
				t.Errorf("generateSlug(%q) = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

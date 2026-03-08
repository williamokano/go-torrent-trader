package service_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock comment repo ---

type mockCommentRepo struct {
	mu       sync.Mutex
	comments []*model.Comment
	nextID   int64
}

func newMockCommentRepo() *mockCommentRepo {
	return &mockCommentRepo{nextID: 1}
}

func (m *mockCommentRepo) Create(_ context.Context, c *model.Comment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.ID = m.nextID
	m.nextID++
	m.comments = append(m.comments, c)
	return nil
}

func (m *mockCommentRepo) GetByID(_ context.Context, id int64) (*model.Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.comments {
		if c.ID == id {
			// Simulate JOIN by adding a username
			copy := *c
			if copy.Username == "" {
				copy.Username = "testuser"
			}
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockCommentRepo) ListByTorrent(_ context.Context, torrentID int64, page, perPage int) ([]model.Comment, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Comment
	for _, c := range m.comments {
		if c.TorrentID == torrentID {
			copy := *c
			if copy.Username == "" {
				copy.Username = "testuser"
			}
			result = append(result, copy)
		}
	}
	total := int64(len(result))
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (m *mockCommentRepo) Update(_ context.Context, c *model.Comment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.comments {
		if existing.ID == c.ID {
			m.comments[i].Body = c.Body
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockCommentRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.comments {
		if c.ID == id {
			m.comments = append(m.comments[:i], m.comments[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

// --- mock rating repo ---

type mockRatingRepo struct {
	mu      sync.Mutex
	ratings []*model.Rating
	nextID  int64
}

func newMockRatingRepo() *mockRatingRepo {
	return &mockRatingRepo{nextID: 1}
}

func (m *mockRatingRepo) Upsert(_ context.Context, r *model.Rating) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.ratings {
		if existing.TorrentID == r.TorrentID && existing.UserID == r.UserID {
			existing.Rating = r.Rating
			r.ID = existing.ID
			return nil
		}
	}
	r.ID = m.nextID
	m.nextID++
	m.ratings = append(m.ratings, r)
	return nil
}

func (m *mockRatingRepo) GetByTorrentAndUser(_ context.Context, torrentID, userID int64) (*model.Rating, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.ratings {
		if r.TorrentID == torrentID && r.UserID == userID {
			copy := *r
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockRatingRepo) GetStatsByTorrent(_ context.Context, torrentID int64) (float64, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sum, count int
	for _, r := range m.ratings {
		if r.TorrentID == torrentID {
			sum += r.Rating
			count++
		}
	}
	if count == 0 {
		return 0, 0, nil
	}
	return float64(sum) / float64(count), count, nil
}

// --- mock torrent repo (minimal, for existence checks) ---

type mockTorrentRepoForComment struct {
	mu       sync.Mutex
	torrents []*model.Torrent
}

func newMockTorrentRepoForComment() *mockTorrentRepoForComment {
	return &mockTorrentRepoForComment{
		torrents: []*model.Torrent{{ID: 1}, {ID: 2}},
	}
}

func (m *mockTorrentRepoForComment) GetByID(_ context.Context, id int64) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockTorrentRepoForComment) GetByInfoHash(context.Context, []byte) (*model.Torrent, error) {
	return nil, errors.New("not found")
}
func (m *mockTorrentRepoForComment) List(context.Context, repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	return nil, 0, nil
}
func (m *mockTorrentRepoForComment) Create(context.Context, *model.Torrent) error  { return nil }
func (m *mockTorrentRepoForComment) Update(context.Context, *model.Torrent) error  { return nil }
func (m *mockTorrentRepoForComment) Delete(context.Context, int64) error            { return nil }
func (m *mockTorrentRepoForComment) IncrementSeeders(context.Context, int64, int) error {
	return nil
}
func (m *mockTorrentRepoForComment) IncrementLeechers(context.Context, int64, int) error {
	return nil
}
func (m *mockTorrentRepoForComment) IncrementTimesCompleted(context.Context, int64) error {
	return nil
}
func (m *mockTorrentRepoForComment) ListByUploader(context.Context, int64, int) ([]model.Torrent, error) {
	return nil, nil
}

// --- helpers ---

func setupCommentService() *service.CommentService {
	return service.NewCommentService(
		newMockCommentRepo(),
		newMockRatingRepo(),
		newMockTorrentRepoForComment(),
		event.NewInMemoryBus(),
	)
}

// --- tests ---

func TestCreateComment_Success(t *testing.T) {
	svc := setupCommentService()
	comment, err := svc.CreateComment(context.Background(), 1, 10, "Great torrent!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if comment.Body != "Great torrent!" {
		t.Errorf("expected body 'Great torrent!', got %q", comment.Body)
	}
}

func TestCreateComment_EmptyBody(t *testing.T) {
	svc := setupCommentService()
	_, err := svc.CreateComment(context.Background(), 1, 10, "")
	if !errors.Is(err, service.ErrInvalidComment) {
		t.Errorf("expected ErrInvalidComment, got %v", err)
	}
}

func TestCreateComment_WhitespaceBody(t *testing.T) {
	svc := setupCommentService()
	_, err := svc.CreateComment(context.Background(), 1, 10, "   ")
	if !errors.Is(err, service.ErrInvalidComment) {
		t.Errorf("expected ErrInvalidComment, got %v", err)
	}
}

func TestCreateComment_TorrentNotFound(t *testing.T) {
	svc := setupCommentService()
	_, err := svc.CreateComment(context.Background(), 999, 10, "Hello")
	if !errors.Is(err, service.ErrTorrentNotFound) {
		t.Errorf("expected ErrTorrentNotFound, got %v", err)
	}
}

func TestListComments_Pagination(t *testing.T) {
	svc := setupCommentService()

	for i := 0; i < 5; i++ {
		if _, err := svc.CreateComment(context.Background(), 1, 10, "Comment"); err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}

	comments, total, err := svc.ListComments(context.Background(), 1, 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments on page, got %d", len(comments))
	}
}

func TestListComments_DefaultPagination(t *testing.T) {
	svc := setupCommentService()
	comments, total, err := svc.ListComments(context.Background(), 1, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}

func TestUpdateComment_AsAuthor(t *testing.T) {
	svc := setupCommentService()
	comment, _ := svc.CreateComment(context.Background(), 1, 10, "Original")

	updated, err := svc.UpdateComment(context.Background(), comment.ID, 10, model.Permissions{GroupID: 5}, "Edited")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Body != "Edited" {
		t.Errorf("expected body 'Edited', got %q", updated.Body)
	}
}

func TestUpdateComment_AsAdmin(t *testing.T) {
	svc := setupCommentService()
	comment, _ := svc.CreateComment(context.Background(), 1, 10, "Original")

	updated, err := svc.UpdateComment(context.Background(), comment.ID, 99, model.Permissions{GroupID: 1, IsAdmin: true}, "Admin edit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Body != "Admin edit" {
		t.Errorf("expected body 'Admin edit', got %q", updated.Body)
	}
}

func TestUpdateComment_Forbidden(t *testing.T) {
	svc := setupCommentService()
	comment, _ := svc.CreateComment(context.Background(), 1, 10, "Original")

	_, err := svc.UpdateComment(context.Background(), comment.ID, 99, model.Permissions{GroupID: 5}, "Hacked")
	if !errors.Is(err, service.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateComment_NotFound(t *testing.T) {
	svc := setupCommentService()
	_, err := svc.UpdateComment(context.Background(), 999, 10, model.Permissions{GroupID: 5}, "Ghost")
	if !errors.Is(err, service.ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound, got %v", err)
	}
}

func TestUpdateComment_EmptyBody(t *testing.T) {
	svc := setupCommentService()
	comment, _ := svc.CreateComment(context.Background(), 1, 10, "Original")

	_, err := svc.UpdateComment(context.Background(), comment.ID, 10, model.Permissions{GroupID: 5}, "")
	if !errors.Is(err, service.ErrInvalidComment) {
		t.Errorf("expected ErrInvalidComment, got %v", err)
	}
}

func TestDeleteComment_AsAdmin(t *testing.T) {
	svc := setupCommentService()
	comment, _ := svc.CreateComment(context.Background(), 1, 10, "To delete")

	err := svc.DeleteComment(context.Background(), comment.ID, 99, model.Permissions{GroupID: 1, IsAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteComment_NotStaff(t *testing.T) {
	svc := setupCommentService()
	comment, _ := svc.CreateComment(context.Background(), 1, 10, "To delete")

	err := svc.DeleteComment(context.Background(), comment.ID, 10, model.Permissions{GroupID: 5})
	if !errors.Is(err, service.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestDeleteComment_NotFound(t *testing.T) {
	svc := setupCommentService()
	err := svc.DeleteComment(context.Background(), 999, 99, model.Permissions{GroupID: 1, IsAdmin: true})
	if !errors.Is(err, service.ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound, got %v", err)
	}
}

func TestRateTorrent_Success(t *testing.T) {
	svc := setupCommentService()
	err := svc.RateTorrent(context.Background(), 1, 10, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRateTorrent_InvalidRating_Low(t *testing.T) {
	svc := setupCommentService()
	err := svc.RateTorrent(context.Background(), 1, 10, 0)
	if !errors.Is(err, service.ErrInvalidRating) {
		t.Errorf("expected ErrInvalidRating, got %v", err)
	}
}

func TestRateTorrent_InvalidRating_High(t *testing.T) {
	svc := setupCommentService()
	err := svc.RateTorrent(context.Background(), 1, 10, 6)
	if !errors.Is(err, service.ErrInvalidRating) {
		t.Errorf("expected ErrInvalidRating, got %v", err)
	}
}

func TestRateTorrent_TorrentNotFound(t *testing.T) {
	svc := setupCommentService()
	err := svc.RateTorrent(context.Background(), 999, 10, 3)
	if !errors.Is(err, service.ErrTorrentNotFound) {
		t.Errorf("expected ErrTorrentNotFound, got %v", err)
	}
}

func TestRateTorrent_Upsert(t *testing.T) {
	svc := setupCommentService()

	// Rate with 3
	if err := svc.RateTorrent(context.Background(), 1, 10, 3); err != nil {
		t.Fatalf("first rate: %v", err)
	}

	// Update to 5
	if err := svc.RateTorrent(context.Background(), 1, 10, 5); err != nil {
		t.Fatalf("upsert rate: %v", err)
	}

	stats, err := svc.GetRatingStats(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.Average != 5.0 {
		t.Errorf("expected average 5.0, got %f", stats.Average)
	}
	if stats.Count != 1 {
		t.Errorf("expected count 1, got %d", stats.Count)
	}
}

func TestGetRatingStats_WithUserRating(t *testing.T) {
	svc := setupCommentService()

	_ = svc.RateTorrent(context.Background(), 1, 10, 3)
	_ = svc.RateTorrent(context.Background(), 1, 20, 5)

	userID := int64(10)
	stats, err := svc.GetRatingStats(context.Background(), 1, &userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Average != 4.0 {
		t.Errorf("expected average 4.0, got %f", stats.Average)
	}
	if stats.Count != 2 {
		t.Errorf("expected count 2, got %d", stats.Count)
	}
	if stats.UserRating == nil || *stats.UserRating != 3 {
		t.Errorf("expected user_rating 3, got %v", stats.UserRating)
	}
}

func TestGetRatingStats_NoRatings(t *testing.T) {
	svc := setupCommentService()

	stats, err := svc.GetRatingStats(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Average != 0 {
		t.Errorf("expected average 0, got %f", stats.Average)
	}
	if stats.Count != 0 {
		t.Errorf("expected count 0, got %d", stats.Count)
	}
	if stats.UserRating != nil {
		t.Errorf("expected nil user_rating, got %v", stats.UserRating)
	}
}

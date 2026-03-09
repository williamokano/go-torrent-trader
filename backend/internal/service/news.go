package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrNewsNotFound = errors.New("news article not found")
	ErrInvalidNews  = errors.New("invalid news article")
)

// NewsService handles news article business logic.
type NewsService struct {
	news     repository.NewsRepository
	users    repository.UserRepository
	eventBus event.Bus
}

// NewNewsService creates a new NewsService.
func NewNewsService(news repository.NewsRepository, users repository.UserRepository, bus event.Bus) *NewsService {
	return &NewsService{
		news:     news,
		users:    users,
		eventBus: bus,
	}
}

// CreateNewsRequest holds the input for creating a news article.
type CreateNewsRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Published bool   `json:"published"`
}

// Create creates a new news article.
func (s *NewsService) Create(ctx context.Context, req CreateNewsRequest, authorID int64) (*model.NewsArticle, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidNews)
	}

	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, fmt.Errorf("%w: body is required", ErrInvalidNews)
	}

	article := &model.NewsArticle{
		Title:     title,
		Body:      body,
		AuthorID:  &authorID,
		Published: req.Published,
	}

	if err := s.news.Create(ctx, article); err != nil {
		return nil, fmt.Errorf("create news article: %w", err)
	}

	if req.Published {
		actor := s.actorFromUserID(ctx, authorID)
		s.eventBus.Publish(ctx, &event.NewsPublishedEvent{
			Base:      event.NewBase(event.NewsPublished, actor),
			ArticleID: article.ID,
			Title:     title,
		})
	}

	// Re-fetch to get joined data
	return s.news.GetByID(ctx, article.ID)
}

// UpdateNewsRequest holds the input for updating a news article.
type UpdateNewsRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Published bool   `json:"published"`
}

// Update updates an existing news article.
func (s *NewsService) Update(ctx context.Context, id int64, req UpdateNewsRequest, actorID int64) (*model.NewsArticle, error) {
	article, err := s.news.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNewsNotFound
		}
		return nil, fmt.Errorf("get news article: %w", err)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidNews)
	}

	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, fmt.Errorf("%w: body is required", ErrInvalidNews)
	}

	wasPublished := article.Published
	article.Title = title
	article.Body = body
	article.Published = req.Published

	if err := s.news.Update(ctx, article); err != nil {
		return nil, fmt.Errorf("update news article: %w", err)
	}

	// Publish event if article was just published
	if !wasPublished && req.Published {
		actor := s.actorFromUserID(ctx, actorID)
		s.eventBus.Publish(ctx, &event.NewsPublishedEvent{
			Base:      event.NewBase(event.NewsPublished, actor),
			ArticleID: id,
			Title:     title,
		})
	}

	return s.news.GetByID(ctx, id)
}

// Delete deletes a news article.
func (s *NewsService) Delete(ctx context.Context, id int64) error {
	if err := s.news.Delete(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNewsNotFound
		}
		return fmt.Errorf("delete news article: %w", err)
	}
	return nil
}

// List returns a paginated list of all news articles (admin view).
func (s *NewsService) List(ctx context.Context, opts repository.ListNewsOptions) ([]model.NewsArticle, int64, error) {
	return s.news.List(ctx, opts)
}

// ListPublished returns a paginated list of published news articles.
func (s *NewsService) ListPublished(ctx context.Context, page, perPage int) ([]model.NewsArticle, int64, error) {
	return s.news.ListPublished(ctx, page, perPage)
}

// GetPublished returns a single published news article by ID.
func (s *NewsService) GetPublished(ctx context.Context, id int64) (*model.NewsArticle, error) {
	article, err := s.news.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNewsNotFound
		}
		return nil, fmt.Errorf("get news article: %w", err)
	}
	if !article.Published {
		return nil, ErrNewsNotFound
	}
	return article, nil
}

func (s *NewsService) actorFromUserID(ctx context.Context, userID int64) event.Actor {
	actor := event.Actor{ID: userID}
	if u, err := s.users.GetByID(ctx, userID); err == nil {
		actor.Username = u.Username
	}
	return actor
}

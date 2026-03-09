package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrCategoryNotFound     = errors.New("category not found")
	ErrCategoryHasTorrents  = errors.New("category has torrents and cannot be deleted")
	ErrInvalidCategory      = errors.New("invalid category")
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// CategoryService handles category CRUD business logic.
type CategoryService struct {
	categories repository.CategoryRepository
}

// NewCategoryService creates a new CategoryService.
func NewCategoryService(categories repository.CategoryRepository) *CategoryService {
	return &CategoryService{categories: categories}
}

// List returns all categories ordered by sort_order and name.
func (s *CategoryService) List(ctx context.Context) ([]model.Category, error) {
	cats, err := s.categories.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return cats, nil
}

// CreateCategoryRequest holds the input for creating a category.
type CreateCategoryRequest struct {
	Name      string  `json:"name"`
	Slug      string  `json:"slug"`
	ParentID  *int64  `json:"parent_id"`
	ImageURL  *string `json:"image_url"`
	SortOrder int     `json:"sort_order"`
}

// Create creates a new category.
func (s *CategoryService) Create(ctx context.Context, req CreateCategoryRequest) (*model.Category, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidCategory)
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	}

	cat := &model.Category{
		Name:      name,
		Slug:      slug,
		ParentID:  req.ParentID,
		ImageURL:  req.ImageURL,
		SortOrder: req.SortOrder,
	}

	if err := s.categories.Create(ctx, cat); err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return cat, nil
}

// UpdateCategoryRequest holds the input for updating a category.
type UpdateCategoryRequest struct {
	Name      string  `json:"name"`
	Slug      string  `json:"slug"`
	ParentID  *int64  `json:"parent_id"`
	ImageURL  *string `json:"image_url"`
	SortOrder int     `json:"sort_order"`
}

// Update updates an existing category.
func (s *CategoryService) Update(ctx context.Context, id int64, req UpdateCategoryRequest) (*model.Category, error) {
	cat, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, ErrCategoryNotFound
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidCategory)
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	}

	cat.Name = name
	cat.Slug = slug
	cat.ParentID = req.ParentID
	cat.ImageURL = req.ImageURL
	cat.SortOrder = req.SortOrder

	if err := s.categories.Update(ctx, cat); err != nil {
		return nil, fmt.Errorf("update category: %w", err)
	}
	return cat, nil
}

// Delete deletes a category, but only if no torrents reference it.
func (s *CategoryService) Delete(ctx context.Context, id int64) error {
	_, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return ErrCategoryNotFound
	}

	count, err := s.categories.CountTorrentsByCategory(ctx, id)
	if err != nil {
		return fmt.Errorf("check torrents: %w", err)
	}
	if count > 0 {
		return ErrCategoryHasTorrents
	}

	if err := s.categories.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	return nil
}

// generateSlug creates a URL-friendly slug from a name.
func generateSlug(name string) string {
	s := strings.ToLower(name)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

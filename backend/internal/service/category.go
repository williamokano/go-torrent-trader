package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/metadata"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrCategoryNotFound    = errors.New("category not found")
	ErrCategoryHasTorrents = errors.New("category has torrents and cannot be deleted")
	ErrInvalidCategory     = errors.New("invalid category")
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
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	ParentID       *int64          `json:"parent_id"`
	ImageURL       *string         `json:"image_url"`
	SortOrder      int             `json:"sort_order"`
	MetadataSchema json.RawMessage `json:"metadata_schema"`
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

	if err := validateImageURL(req.ImageURL); err != nil {
		return nil, err
	}

	schema, err := canonicalizeSchema(req.MetadataSchema)
	if err != nil {
		return nil, err
	}

	cat := &model.Category{
		Name:           name,
		Slug:           slug,
		ParentID:       req.ParentID,
		ImageURL:       req.ImageURL,
		SortOrder:      req.SortOrder,
		MetadataSchema: schema,
	}

	if err := s.categories.Create(ctx, cat); err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return cat, nil
}

// UpdateCategoryRequest holds the input for updating a category.
type UpdateCategoryRequest struct {
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	ParentID       *int64          `json:"parent_id"`
	ImageURL       *string         `json:"image_url"`
	SortOrder      int             `json:"sort_order"`
	MetadataSchema json.RawMessage `json:"metadata_schema"`
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

	if err := validateImageURL(req.ImageURL); err != nil {
		return nil, err
	}

	schema, err := canonicalizeSchema(req.MetadataSchema)
	if err != nil {
		return nil, err
	}

	cat.Name = name
	cat.Slug = slug
	cat.ParentID = req.ParentID
	cat.ImageURL = req.ImageURL
	cat.SortOrder = req.SortOrder
	cat.MetadataSchema = schema

	if err := s.categories.Update(ctx, cat); err != nil {
		return nil, fmt.Errorf("update category: %w", err)
	}
	return cat, nil
}

// ReorderItem is one category's new placement (parent and sort order) in a
// batch reorder request.
type ReorderItem struct {
	ID        int64  `json:"id"`
	ParentID  *int64 `json:"parent_id"`
	SortOrder int    `json:"sort_order"`
}

// Reorder atomically updates the parent_id and sort_order of the given
// categories. It rejects a request that would leave the hierarchy with a cycle
// (a category becoming its own ancestor) or that references a parent not present
// in the request, so a drag-and-drop reorder can't corrupt the tree.
func (s *CategoryService) Reorder(ctx context.Context, items []ReorderItem) error {
	if len(items) == 0 {
		return nil
	}

	parentOf := make(map[int64]*int64, len(items))
	for _, it := range items {
		if _, dup := parentOf[it.ID]; dup {
			return fmt.Errorf("%w: category %d appears twice in reorder", ErrInvalidCategory, it.ID)
		}
		parentOf[it.ID] = it.ParentID
	}

	// Walk each node's parent chain: every referenced parent must be in the set,
	// and the chain must terminate at a root without revisiting a node.
	for _, it := range items {
		steps := 0
		for cur := it.ParentID; cur != nil; {
			if *cur == it.ID {
				return fmt.Errorf("%w: category %d cannot be its own ancestor", ErrInvalidCategory, it.ID)
			}
			next, ok := parentOf[*cur]
			if !ok {
				return fmt.Errorf("%w: parent %d is not part of the reorder", ErrInvalidCategory, *cur)
			}
			cur = next
			if steps++; steps > len(items) {
				return fmt.Errorf("%w: category hierarchy contains a cycle", ErrInvalidCategory)
			}
		}
	}

	placements := make([]repository.CategoryPlacement, len(items))
	for i, it := range items {
		placements[i] = repository.CategoryPlacement{
			ID:        it.ID,
			ParentID:  it.ParentID,
			SortOrder: it.SortOrder,
		}
	}
	if err := s.categories.Reorder(ctx, placements); err != nil {
		return fmt.Errorf("reorder categories: %w", err)
	}
	return nil
}

// canonicalizeSchema validates a category's submitted metadata schema and
// returns a normalized JSONB array to persist. An empty/absent schema becomes
// "[]". Malformed or invalid schemas surface as ErrInvalidCategory so the
// handler maps them to a 422.
func canonicalizeSchema(raw json.RawMessage) (json.RawMessage, error) {
	fields, err := metadata.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCategory, err)
	}
	if err := metadata.ValidateSchema(fields); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCategory, err)
	}
	if len(fields) == 0 {
		return json.RawMessage("[]"), nil
	}
	canonical, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCategory, err)
	}
	return canonical, nil
}

// ResolveSchema returns a category's effective metadata schema: its own fields
// merged with those inherited from its ancestors (root first, child overrides).
func (s *CategoryService) ResolveSchema(ctx context.Context, categoryID int64) ([]metadata.FieldDef, error) {
	return ResolveCategorySchema(ctx, s.categories, categoryID)
}

// ResolveCategorySchema walks the category's ancestor chain and merges each
// level's own schema into the effective schema. Shared by CategoryService,
// TorrentService, and handlers that hold only a category repository.
// Cycle-guarded.
func ResolveCategorySchema(ctx context.Context, repo repository.CategoryRepository, categoryID int64) ([]metadata.FieldDef, error) {
	var leafFirst [][]metadata.FieldDef
	seen := make(map[int64]bool)
	curID := categoryID
	for !seen[curID] { // loop stops if a malformed parent chain revisits a category
		seen[curID] = true

		cat, err := repo.GetByID(ctx, curID)
		if err != nil {
			if len(leafFirst) == 0 {
				return nil, ErrCategoryNotFound
			}
			break // a missing ancestor just stops the walk
		}

		fields, err := metadata.Parse(cat.MetadataSchema)
		if err != nil {
			return nil, err
		}
		leafFirst = append(leafFirst, fields)

		if cat.ParentID == nil {
			break
		}
		curID = *cat.ParentID
	}

	// Reverse to root-first so ancestors are merged before descendants.
	for i, j := 0, len(leafFirst)-1; i < j; i, j = i+1, j-1 {
		leafFirst[i], leafFirst[j] = leafFirst[j], leafFirst[i]
	}
	return metadata.Merge(leafFirst), nil
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
		// The GetByID above narrows the window, but the row can still vanish
		// before the delete lands (a concurrent delete). Map that to the same
		// not-found the caller already handles rather than a 500.
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCategoryNotFound
		}
		return fmt.Errorf("delete category: %w", err)
	}
	return nil
}

// validateImageURL checks that the image URL, if provided, is a valid HTTP(S) URL.
func validateImageURL(imageURL *string) error {
	if imageURL == nil || *imageURL == "" {
		return nil
	}
	u, err := url.Parse(*imageURL)
	if err != nil {
		return fmt.Errorf("%w: invalid image URL", ErrInvalidCategory)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: image_url must use http or https scheme", ErrInvalidCategory)
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

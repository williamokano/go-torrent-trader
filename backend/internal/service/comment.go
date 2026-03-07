package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrCommentNotFound = errors.New("comment not found")
	ErrInvalidComment  = errors.New("invalid comment")
	ErrInvalidRating   = errors.New("invalid rating")
)

// CommentService handles comment and rating business logic.
type CommentService struct {
	comments repository.CommentRepository
	ratings  repository.RatingRepository
	torrents repository.TorrentRepository
}

// NewCommentService creates a new CommentService.
func NewCommentService(
	comments repository.CommentRepository,
	ratings repository.RatingRepository,
	torrents repository.TorrentRepository,
) *CommentService {
	return &CommentService{
		comments: comments,
		ratings:  ratings,
		torrents: torrents,
	}
}

// CreateComment adds a comment to a torrent.
func (s *CommentService) CreateComment(ctx context.Context, torrentID, userID int64, body string) (*model.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: body cannot be empty", ErrInvalidComment)
	}

	// Verify the torrent exists.
	if _, err := s.torrents.GetByID(ctx, torrentID); err != nil {
		return nil, ErrTorrentNotFound
	}

	comment := &model.Comment{
		TorrentID: torrentID,
		UserID:    userID,
		Body:      body,
	}

	if err := s.comments.Create(ctx, comment); err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}

	// Re-fetch to get the username from the JOIN.
	created, err := s.comments.GetByID(ctx, comment.ID)
	if err != nil {
		return nil, fmt.Errorf("get created comment: %w", err)
	}

	return created, nil
}

// ListComments returns paginated comments for a torrent.
func (s *CommentService) ListComments(ctx context.Context, torrentID int64, page, perPage int) ([]model.Comment, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	return s.comments.ListByTorrent(ctx, torrentID, page, perPage)
}

// UpdateComment edits an existing comment. Only the author or an admin (groupID=1) may edit.
func (s *CommentService) UpdateComment(ctx context.Context, commentID, userID, groupID int64, body string) (*model.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: body cannot be empty", ErrInvalidComment)
	}

	comment, err := s.comments.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCommentNotFound
		}
		return nil, ErrCommentNotFound
	}

	isAdmin := groupID == 1
	isAuthor := comment.UserID == userID

	if !isAuthor && !isAdmin {
		return nil, ErrForbidden
	}

	comment.Body = body
	if err := s.comments.Update(ctx, comment); err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}

	return comment, nil
}

// DeleteComment removes a comment. Only admins (groupID=1) may delete.
func (s *CommentService) DeleteComment(ctx context.Context, commentID, groupID int64) error {
	isAdmin := groupID == 1
	if !isAdmin {
		return ErrForbidden
	}

	if err := s.comments.Delete(ctx, commentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCommentNotFound
		}
		return fmt.Errorf("delete comment: %w", err)
	}

	return nil
}

// RateTorrent sets or updates the user's rating for a torrent.
func (s *CommentService) RateTorrent(ctx context.Context, torrentID, userID int64, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("%w: rating must be between 1 and 5", ErrInvalidRating)
	}

	// Verify the torrent exists.
	if _, err := s.torrents.GetByID(ctx, torrentID); err != nil {
		return ErrTorrentNotFound
	}

	r := &model.Rating{
		TorrentID: torrentID,
		UserID:    userID,
		Rating:    rating,
	}

	if err := s.ratings.Upsert(ctx, r); err != nil {
		return fmt.Errorf("upsert rating: %w", err)
	}

	return nil
}

// GetRatingStats returns the aggregate rating stats for a torrent, including the user's own rating if authenticated.
func (s *CommentService) GetRatingStats(ctx context.Context, torrentID int64, userID *int64) (*model.RatingStats, error) {
	avg, count, err := s.ratings.GetStatsByTorrent(ctx, torrentID)
	if err != nil {
		return nil, fmt.Errorf("get rating stats: %w", err)
	}

	stats := &model.RatingStats{
		Average: avg,
		Count:   count,
	}

	if userID != nil {
		userRating, err := s.ratings.GetByTorrentAndUser(ctx, torrentID, *userID)
		if err == nil {
			stats.UserRating = &userRating.Rating
		}
		// If not found, UserRating stays nil — that's fine.
	}

	return stats, nil
}

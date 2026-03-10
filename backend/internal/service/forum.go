package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrForumNotFound   = errors.New("forum not found")
	ErrTopicNotFound   = errors.New("topic not found")
	ErrTopicLocked     = errors.New("topic is locked")
	ErrForumAccessDenied = errors.New("forum access denied")
	ErrInvalidTopic    = errors.New("invalid topic")
	ErrInvalidPost     = errors.New("invalid post")
)

// ForumService handles forum business logic.
type ForumService struct {
	categories repository.ForumCategoryRepository
	forums     repository.ForumRepository
	topics     repository.ForumTopicRepository
	posts      repository.ForumPostRepository
	users      repository.UserRepository
}

// NewForumService creates a new ForumService.
func NewForumService(
	categories repository.ForumCategoryRepository,
	forums repository.ForumRepository,
	topics repository.ForumTopicRepository,
	posts repository.ForumPostRepository,
	users repository.UserRepository,
) *ForumService {
	return &ForumService{
		categories: categories,
		forums:     forums,
		topics:     topics,
		posts:      posts,
		users:      users,
	}
}

// ListCategories returns all forum categories with their forums, filtered by user access level.
func (s *ForumService) ListCategories(ctx context.Context, perms model.Permissions) ([]model.ForumCategory, error) {
	categories, err := s.categories.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	forums, err := s.forums.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list forums: %w", err)
	}

	// Group forums by category, filtering by user's group level
	forumsByCategory := make(map[int64][]model.Forum)
	for _, f := range forums {
		if f.MinGroupLevel <= perms.Level {
			forumsByCategory[f.CategoryID] = append(forumsByCategory[f.CategoryID], f)
		}
	}

	// Attach forums to categories, excluding empty categories
	var result []model.ForumCategory
	for _, cat := range categories {
		if fs, ok := forumsByCategory[cat.ID]; ok && len(fs) > 0 {
			cat.Forums = fs
			result = append(result, cat)
		}
	}

	return result, nil
}

// GetForum returns a forum by ID with access check.
func (s *ForumService) GetForum(ctx context.Context, forumID int64, perms model.Permissions) (*model.Forum, error) {
	forum, err := s.forums.GetByID(ctx, forumID)
	if err != nil {
		return nil, ErrForumNotFound
	}

	if forum.MinGroupLevel > perms.Level {
		return nil, ErrForumAccessDenied
	}

	return forum, nil
}

// ListTopics returns paginated topics for a forum, pinned first.
func (s *ForumService) ListTopics(ctx context.Context, forumID int64, perms model.Permissions, page, perPage int) (*model.Forum, []model.ForumTopic, int64, error) {
	forum, err := s.GetForum(ctx, forumID, perms)
	if err != nil {
		return nil, nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	topics, total, err := s.topics.ListByForum(ctx, forumID, page, perPage)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("list topics: %w", err)
	}

	return forum, topics, total, nil
}

// GetTopic returns a topic by ID with access check and view count increment.
func (s *ForumService) GetTopic(ctx context.Context, topicID int64, perms model.Permissions) (*model.ForumTopic, error) {
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return nil, ErrTopicNotFound
	}

	// Check forum access
	forum, err := s.forums.GetByID(ctx, topic.ForumID)
	if err != nil {
		return nil, ErrForumNotFound
	}
	if forum.MinGroupLevel > perms.Level {
		return nil, ErrForumAccessDenied
	}

	// Increment view count (best effort, don't fail the request)
	_ = s.topics.IncrementViewCount(ctx, topicID)
	topic.ViewCount++

	return topic, nil
}

// ListPosts returns paginated posts for a topic.
func (s *ForumService) ListPosts(ctx context.Context, topicID int64, page, perPage int) ([]model.ForumPost, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	return s.posts.ListByTopic(ctx, topicID, page, perPage)
}

// CreateTopic creates a new topic with the first post in a forum.
func (s *ForumService) CreateTopic(ctx context.Context, forumID, userID int64, perms model.Permissions, title, body string) (*model.ForumTopic, *model.ForumPost, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)

	if title == "" {
		return nil, nil, fmt.Errorf("%w: title cannot be empty", ErrInvalidTopic)
	}
	if len(title) > 200 {
		return nil, nil, fmt.Errorf("%w: title too long", ErrInvalidTopic)
	}
	if body == "" {
		return nil, nil, fmt.Errorf("%w: body cannot be empty", ErrInvalidPost)
	}

	// Check forum access
	forum, err := s.forums.GetByID(ctx, forumID)
	if err != nil {
		return nil, nil, ErrForumNotFound
	}
	if forum.MinGroupLevel > perms.Level {
		return nil, nil, ErrForumAccessDenied
	}

	// Check user can_forum flag
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("get user: %w", err)
	}
	if !user.CanForum {
		return nil, nil, ErrForumAccessDenied
	}

	// Create topic
	topic := &model.ForumTopic{
		ForumID: forumID,
		UserID:  userID,
		Title:   title,
	}
	if err := s.topics.Create(ctx, topic); err != nil {
		return nil, nil, fmt.Errorf("create topic: %w", err)
	}

	// Create first post
	post := &model.ForumPost{
		TopicID: topic.ID,
		UserID:  userID,
		Body:    body,
	}
	if err := s.posts.Create(ctx, post); err != nil {
		return nil, nil, fmt.Errorf("create post: %w", err)
	}

	// Update denormalized counts
	_ = s.topics.IncrementPostCount(ctx, topic.ID, 1)
	_ = s.topics.UpdateLastPost(ctx, topic.ID, post.ID, post.CreatedAt)
	_ = s.forums.IncrementTopicCount(ctx, forumID, 1)
	_ = s.forums.IncrementPostCount(ctx, forumID, 1)
	_ = s.forums.UpdateLastPost(ctx, forumID, post.ID)

	topic.PostCount = 1
	topic.LastPostAt = &post.CreatedAt

	return topic, post, nil
}

// CreatePost creates a reply in a topic.
func (s *ForumService) CreatePost(ctx context.Context, topicID, userID int64, perms model.Permissions, body string, replyToPostID *int64) (*model.ForumPost, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: body cannot be empty", ErrInvalidPost)
	}

	// Check topic exists
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return nil, ErrTopicNotFound
	}

	if topic.Locked {
		return nil, ErrTopicLocked
	}

	// Check forum access
	forum, err := s.forums.GetByID(ctx, topic.ForumID)
	if err != nil {
		return nil, ErrForumNotFound
	}
	if forum.MinGroupLevel > perms.Level {
		return nil, ErrForumAccessDenied
	}

	// Check user can_forum flag
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if !user.CanForum {
		return nil, ErrForumAccessDenied
	}

	post := &model.ForumPost{
		TopicID:       topicID,
		UserID:        userID,
		Body:          body,
		ReplyToPostID: replyToPostID,
	}
	if err := s.posts.Create(ctx, post); err != nil {
		return nil, fmt.Errorf("create post: %w", err)
	}

	// Update denormalized counts
	_ = s.topics.IncrementPostCount(ctx, topicID, 1)
	_ = s.topics.UpdateLastPost(ctx, topicID, post.ID, post.CreatedAt)
	_ = s.forums.IncrementPostCount(ctx, topic.ForumID, 1)
	_ = s.forums.UpdateLastPost(ctx, topic.ForumID, post.ID)

	// Re-fetch with user info
	created, err := s.posts.GetByID(ctx, post.ID)
	if err != nil {
		return post, nil
	}
	return created, nil
}

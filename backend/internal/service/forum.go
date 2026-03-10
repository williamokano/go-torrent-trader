package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrForumNotFound     = errors.New("forum not found")
	ErrTopicNotFound     = errors.New("topic not found")
	ErrTopicLocked       = errors.New("topic is locked")
	ErrForumAccessDenied = errors.New("forum access denied")
	ErrInvalidTopic      = errors.New("invalid topic")
	ErrInvalidPost       = errors.New("invalid post")
	ErrInvalidReply      = errors.New("invalid reply reference")
	ErrInvalidSearch     = errors.New("invalid search query")
)

const viewCountDebounce = 15 * time.Minute

type ForumService struct {
	db         *sql.DB
	categories repository.ForumCategoryRepository
	forums     repository.ForumRepository
	topics     repository.ForumTopicRepository
	posts      repository.ForumPostRepository
	users      repository.UserRepository
	viewMu       sync.Mutex
	viewDebounce map[string]time.Time
}

func NewForumService(db *sql.DB, categories repository.ForumCategoryRepository, forums repository.ForumRepository, topics repository.ForumTopicRepository, posts repository.ForumPostRepository, users repository.UserRepository) *ForumService {
	return &ForumService{db: db, categories: categories, forums: forums, topics: topics, posts: posts, users: users, viewDebounce: make(map[string]time.Time)}
}

func (s *ForumService) ListCategories(ctx context.Context, perms model.Permissions) ([]model.ForumCategory, error) {
	categories, err := s.categories.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	forums, err := s.forums.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list forums: %w", err)
	}
	forumsByCategory := make(map[int64][]model.Forum)
	for _, f := range forums {
		if f.MinGroupLevel <= perms.Level {
			forumsByCategory[f.CategoryID] = append(forumsByCategory[f.CategoryID], f)
		}
	}
	var result []model.ForumCategory
	for _, cat := range categories {
		if fs, ok := forumsByCategory[cat.ID]; ok && len(fs) > 0 {
			cat.Forums = fs
			result = append(result, cat)
		}
	}
	return result, nil
}

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

func (s *ForumService) ListTopics(ctx context.Context, forumID int64, perms model.Permissions, page, perPage int) (*model.Forum, []model.ForumTopic, int64, error) {
	forum, err := s.GetForum(ctx, forumID, perms)
	if err != nil {
		return nil, nil, 0, err
	}
	if page <= 0 { page = 1 }
	if perPage <= 0 { perPage = 25 }
	if perPage > 100 { perPage = 100 }
	topics, total, err := s.topics.ListByForum(ctx, forumID, page, perPage)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("list topics: %w", err)
	}
	return forum, topics, total, nil
}

// GetTopic returns a topic by ID with access check and debounced view count increment.
// The view count is only incremented if the same user hasn't viewed this topic in the
// last 15 minutes, preventing easy view count gaming.
func (s *ForumService) GetTopic(ctx context.Context, topicID int64, userID int64, perms model.Permissions) (*model.ForumTopic, error) {
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return nil, ErrTopicNotFound
	}
	forum, err := s.forums.GetByID(ctx, topic.ForumID)
	if err != nil {
		return nil, ErrForumNotFound
	}
	if forum.MinGroupLevel > perms.Level {
		return nil, ErrForumAccessDenied
	}
	if s.shouldIncrementView(userID, topicID) {
		_ = s.topics.IncrementViewCount(ctx, topicID)
		topic.ViewCount++
	}
	return topic, nil
}

func (s *ForumService) shouldIncrementView(userID, topicID int64) bool {
	key := fmt.Sprintf("%d:%d", userID, topicID)
	now := time.Now()
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	if last, ok := s.viewDebounce[key]; ok && now.Sub(last) < viewCountDebounce {
		return false
	}
	s.viewDebounce[key] = now
	return true
}

func (s *ForumService) ListPosts(ctx context.Context, topicID int64, page, perPage int) ([]model.ForumPost, int64, error) {
	if page <= 0 { page = 1 }
	if perPage <= 0 { perPage = 25 }
	if perPage > 100 { perPage = 100 }
	return s.posts.ListByTopic(ctx, topicID, page, perPage)
}

// CreateTopic creates a new topic with the first post in a forum.
// All operations are wrapped in a single database transaction to ensure atomicity.
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
	forum, err := s.forums.GetByID(ctx, forumID)
	if err != nil {
		return nil, nil, ErrForumNotFound
	}
	if forum.MinGroupLevel > perms.Level {
		return nil, nil, ErrForumAccessDenied
	}
	// can_forum=false only blocks writing (CreateTopic, CreatePost).
	// Reading forums is always allowed if the user's group level meets min_group_level.
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("get user: %w", err)
	}
	if !user.CanForum {
		return nil, nil, ErrForumAccessDenied
	}
	var topic model.ForumTopic
	var post model.ForumPost
	if s.db != nil {
		err = repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "INSERT INTO forum_topics (forum_id, user_id, title) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at", forumID, userID, title).Scan(&topic.ID, &topic.CreatedAt, &topic.UpdatedAt); err != nil {
				return fmt.Errorf("create topic: %w", err)
			}
			topic.ForumID, topic.UserID, topic.Title = forumID, userID, title
			if err := tx.QueryRowContext(ctx, "INSERT INTO forum_posts (topic_id, user_id, body, reply_to_post_id) VALUES ($1, $2, $3, $4) RETURNING id, created_at", topic.ID, userID, body, nil).Scan(&post.ID, &post.CreatedAt); err != nil {
				return fmt.Errorf("create post: %w", err)
			}
			post.TopicID, post.UserID, post.Body = topic.ID, userID, body
			if _, err := tx.ExecContext(ctx, "UPDATE forum_topics SET post_count = post_count + 1, last_post_id = $1, last_post_at = $2, updated_at = NOW() WHERE id = $3", post.ID, post.CreatedAt, topic.ID); err != nil {
				return fmt.Errorf("update topic counts: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forums SET topic_count = topic_count + 1, post_count = post_count + 1, last_post_id = $1 WHERE id = $2", post.ID, forumID); err != nil {
				return fmt.Errorf("update forum counts: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	} else {
		tp := &model.ForumTopic{ForumID: forumID, UserID: userID, Title: title}
		if err := s.topics.Create(ctx, tp); err != nil { return nil, nil, fmt.Errorf("create topic: %w", err) }
		topic = *tp
		pp := &model.ForumPost{TopicID: topic.ID, UserID: userID, Body: body}
		if err := s.posts.Create(ctx, pp); err != nil { return nil, nil, fmt.Errorf("create post: %w", err) }
		post = *pp
		if err := s.topics.IncrementPostCount(ctx, topic.ID, 1); err != nil { return nil, nil, fmt.Errorf("update topic post count: %w", err) }
		if err := s.topics.UpdateLastPost(ctx, topic.ID, post.ID, post.CreatedAt); err != nil { return nil, nil, fmt.Errorf("update topic last post: %w", err) }
		if err := s.forums.IncrementTopicCount(ctx, forumID, 1); err != nil { return nil, nil, fmt.Errorf("update forum topic count: %w", err) }
		if err := s.forums.IncrementPostCount(ctx, forumID, 1); err != nil { return nil, nil, fmt.Errorf("update forum post count: %w", err) }
		if err := s.forums.UpdateLastPost(ctx, forumID, post.ID); err != nil { return nil, nil, fmt.Errorf("update forum last post: %w", err) }
	}
	topic.PostCount = 1
	topic.LastPostAt = &post.CreatedAt
	return &topic, &post, nil
}

// CreatePost creates a reply in a topic.
// All operations are wrapped in a single database transaction to ensure atomicity.
func (s *ForumService) CreatePost(ctx context.Context, topicID, userID int64, perms model.Permissions, body string, replyToPostID *int64) (*model.ForumPost, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: body cannot be empty", ErrInvalidPost)
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil { return nil, ErrTopicNotFound }
	if topic.Locked { return nil, ErrTopicLocked }
	forum, err := s.forums.GetByID(ctx, topic.ForumID)
	if err != nil { return nil, ErrForumNotFound }
	if forum.MinGroupLevel > perms.Level { return nil, ErrForumAccessDenied }
	// can_forum=false only blocks writing (CreateTopic, CreatePost).
	// Reading forums is always allowed if the user's group level meets min_group_level.
	user, err := s.users.GetByID(ctx, userID)
	if err != nil { return nil, fmt.Errorf("get user: %w", err) }
	if !user.CanForum { return nil, ErrForumAccessDenied }
	// Validate reply_to_post_id: referenced post must exist and belong to the same topic.
	if replyToPostID != nil {
		replyPost, rpErr := s.posts.GetByID(ctx, *replyToPostID)
		if rpErr != nil { return nil, fmt.Errorf("%w: referenced post not found", ErrInvalidReply) }
		if replyPost.TopicID != topicID { return nil, fmt.Errorf("%w: referenced post belongs to a different topic", ErrInvalidReply) }
	}
	var post model.ForumPost
	if s.db != nil {
		err = repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "INSERT INTO forum_posts (topic_id, user_id, body, reply_to_post_id) VALUES ($1, $2, $3, $4) RETURNING id, created_at", topicID, userID, body, replyToPostID).Scan(&post.ID, &post.CreatedAt); err != nil {
				return fmt.Errorf("create post: %w", err)
			}
			post.TopicID, post.UserID, post.Body, post.ReplyToPostID = topicID, userID, body, replyToPostID
			if _, err := tx.ExecContext(ctx, "UPDATE forum_topics SET post_count = post_count + 1, last_post_id = $1, last_post_at = $2, updated_at = NOW() WHERE id = $3", post.ID, post.CreatedAt, topicID); err != nil {
				return fmt.Errorf("update topic counts: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forums SET post_count = post_count + 1, last_post_id = $1 WHERE id = $2", post.ID, topic.ForumID); err != nil {
				return fmt.Errorf("update forum counts: %w", err)
			}
			return nil
		})
		if err != nil { return nil, err }
	} else {
		pp := &model.ForumPost{TopicID: topicID, UserID: userID, Body: body, ReplyToPostID: replyToPostID}
		if err := s.posts.Create(ctx, pp); err != nil { return nil, fmt.Errorf("create post: %w", err) }
		post = *pp
		if err := s.topics.IncrementPostCount(ctx, topicID, 1); err != nil { return nil, fmt.Errorf("update topic post count: %w", err) }
		if err := s.topics.UpdateLastPost(ctx, topicID, post.ID, post.CreatedAt); err != nil { return nil, fmt.Errorf("update topic last post: %w", err) }
		if err := s.forums.IncrementPostCount(ctx, topic.ForumID, 1); err != nil { return nil, fmt.Errorf("update forum post count: %w", err) }
		if err := s.forums.UpdateLastPost(ctx, topic.ForumID, post.ID); err != nil { return nil, fmt.Errorf("update forum last post: %w", err) }
	}
	created, err := s.posts.GetByID(ctx, post.ID)
	if err != nil { return &post, nil }
	return created, nil
}

// Search performs full-text search across forum posts and topics, filtering
// results by forum access level so users only see content they're allowed to view.
func (s *ForumService) Search(ctx context.Context, query string, perms model.Permissions, forumID *int64, page, perPage int) ([]model.ForumSearchResult, int64, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, 0, fmt.Errorf("%w: query cannot be empty", ErrInvalidSearch)
	}
	if utf8.RuneCountInString(query) < 2 {
		return nil, 0, fmt.Errorf("%w: query must be at least 2 characters", ErrInvalidSearch)
	}
	if utf8.RuneCountInString(query) > 200 {
		return nil, 0, fmt.Errorf("%w: query too long", ErrInvalidSearch)
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
	return s.posts.Search(ctx, query, forumID, perms.Level, page, perPage)
}

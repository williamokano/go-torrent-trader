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
	ErrForumNotFound         = errors.New("forum not found")
	ErrTopicNotFound         = errors.New("topic not found")
	ErrTopicLocked           = errors.New("topic is locked")
	ErrForumAccessDenied     = errors.New("forum access denied")
	ErrInvalidTopic          = errors.New("invalid topic")
	ErrInvalidPost           = errors.New("invalid post")
	ErrInvalidReply          = errors.New("invalid reply reference")
	ErrInvalidSearch         = errors.New("invalid search query")
	ErrPostNotFound          = errors.New("post not found")
	ErrPostEditDenied        = errors.New("not authorized to edit this post")
	ErrPostDeleteDenied      = errors.New("not authorized to delete this post")
	ErrCannotDeleteFirstPost = errors.New("cannot delete the first post of a topic; delete the topic instead")
	ErrSameForum             = errors.New("topic is already in this forum")
	ErrTopicDeleteDenied     = errors.New("topic delete denied")
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

// EditPost updates a forum post body. Only the post author or staff can edit.
func (s *ForumService) EditPost(ctx context.Context, postID int64, userID int64, perms model.Permissions, body string) (*model.ForumPost, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: body cannot be empty", ErrInvalidPost)
	}

	post, err := s.posts.GetByID(ctx, postID)
	if err != nil {
		return nil, ErrPostNotFound
	}

	// Authorization: post author or staff
	if post.UserID != userID && !perms.IsStaff() {
		return nil, ErrPostEditDenied
	}

	// can_forum check: non-staff users with can_forum=false cannot edit posts
	if !perms.IsStaff() {
		user, userErr := s.users.GetByID(ctx, userID)
		if userErr != nil {
			return nil, fmt.Errorf("get user: %w", userErr)
		}
		if !user.CanForum {
			return nil, ErrForumAccessDenied
		}
	}

	// Check forum access via topic
	topic, err := s.topics.GetByID(ctx, post.TopicID)
	if err != nil {
		return nil, ErrTopicNotFound
	}

	// Locked topic check: non-staff cannot edit in locked topics
	if topic.Locked && !perms.IsStaff() {
		return nil, ErrTopicLocked
	}

	forum, err := s.forums.GetByID(ctx, topic.ForumID)
	if err != nil {
		return nil, ErrForumNotFound
	}
	if forum.MinGroupLevel > perms.Level {
		return nil, ErrForumAccessDenied
	}

	post.Body = body
	post.EditedBy = &userID
	if err := s.posts.Update(ctx, post); err != nil {
		return nil, fmt.Errorf("update post: %w", err)
	}

	// Re-fetch to get updated edited_at and denormalized fields
	updated, err := s.posts.GetByID(ctx, postID)
	if err != nil {
		return post, nil
	}
	return updated, nil
}

// DeletePost deletes a forum post and updates topic/forum counters.
// The first post of a topic cannot be deleted — the topic must be deleted instead.
func (s *ForumService) DeletePost(ctx context.Context, postID int64, userID int64, perms model.Permissions) error {
	post, err := s.posts.GetByID(ctx, postID)
	if err != nil {
		return ErrPostNotFound
	}

	// Authorization: post author or staff
	if post.UserID != userID && !perms.IsStaff() {
		return ErrPostDeleteDenied
	}

	// can_forum check: non-staff users with can_forum=false cannot delete posts
	if !perms.IsStaff() {
		user, userErr := s.users.GetByID(ctx, userID)
		if userErr != nil {
			return fmt.Errorf("get user: %w", userErr)
		}
		if !user.CanForum {
			return ErrForumAccessDenied
		}
	}

	// Check if this is the first post in the topic
	firstPostID, err := s.posts.GetFirstPostIDByTopic(ctx, post.TopicID)
	if err != nil {
		return fmt.Errorf("get first post: %w", err)
	}
	if post.ID == firstPostID {
		return ErrCannotDeleteFirstPost
	}

	// Get topic for forum ID (needed for counter updates and locked check)
	topic, err := s.topics.GetByID(ctx, post.TopicID)
	if err != nil {
		return ErrTopicNotFound
	}

	// Locked topic check: non-staff cannot delete in locked topics
	if topic.Locked && !perms.IsStaff() {
		return ErrTopicLocked
	}

	// Forum access check (MinGroupLevel)
	forum, err := s.forums.GetByID(ctx, topic.ForumID)
	if err != nil {
		return ErrForumNotFound
	}
	if forum.MinGroupLevel > perms.Level {
		return ErrForumAccessDenied
	}

	if s.db != nil {
		return repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM forum_posts WHERE id = $1", post.ID); err != nil {
				return fmt.Errorf("delete post: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forum_topics SET post_count = GREATEST(post_count - 1, 0) WHERE id = $1", post.TopicID); err != nil {
				return fmt.Errorf("decrement topic post count: %w", err)
			}
			// Recalculate topic last_post using a single atomic subquery
			if _, err := tx.ExecContext(ctx, `
				UPDATE forum_topics SET
					last_post_id = sub.id,
					last_post_at = sub.created_at,
					updated_at = NOW()
				FROM (
					SELECT id, created_at FROM forum_posts
					WHERE topic_id = $1 ORDER BY created_at DESC LIMIT 1
				) sub
				WHERE forum_topics.id = $1`, post.TopicID); err != nil {
				return fmt.Errorf("recalculate topic last post: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forums SET post_count = GREATEST(post_count - 1, 0) WHERE id = $1", topic.ForumID); err != nil {
				return fmt.Errorf("decrement forum post count: %w", err)
			}
			// Recalculate forum last_post using a single atomic subquery
			if _, err := tx.ExecContext(ctx, `
				UPDATE forums SET last_post_id = sub.id
				FROM (
					SELECT p.id FROM forum_posts p
					JOIN forum_topics t ON t.id = p.topic_id
					WHERE t.forum_id = $1
					ORDER BY p.created_at DESC LIMIT 1
				) sub
				WHERE forums.id = $1`, topic.ForumID); err != nil {
				return fmt.Errorf("recalculate forum last post: %w", err)
			}
			return nil
		})
	}

	// Non-transactional path (used in tests with nil db)
	if err := s.posts.Delete(ctx, post.ID); err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	if err := s.topics.IncrementPostCount(ctx, post.TopicID, -1); err != nil {
		return fmt.Errorf("decrement topic post count: %w", err)
	}
	if err := s.topics.RecalculateLastPost(ctx, post.TopicID); err != nil {
		return fmt.Errorf("recalculate topic last post: %w", err)
	}
	if err := s.forums.IncrementPostCount(ctx, topic.ForumID, -1); err != nil {
		return fmt.Errorf("decrement forum post count: %w", err)
	}
	if err := s.forums.RecalculateLastPost(ctx, topic.ForumID); err != nil {
		return fmt.Errorf("recalculate forum last post: %w", err)
	}
	return nil
}

func (s *ForumService) requireStaff(perms model.Permissions) error {
	if !perms.IsStaff() {
		return ErrForumAccessDenied
	}
	return nil
}

// LockTopic locks a topic so no new replies can be posted. Staff/admin only.
func (s *ForumService) LockTopic(ctx context.Context, topicID int64, perms model.Permissions) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	if _, err := s.topics.GetByID(ctx, topicID); err != nil {
		return ErrTopicNotFound
	}
	return s.topics.SetLocked(ctx, topicID, true)
}

// UnlockTopic unlocks a previously locked topic. Staff/admin only.
func (s *ForumService) UnlockTopic(ctx context.Context, topicID int64, perms model.Permissions) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	if _, err := s.topics.GetByID(ctx, topicID); err != nil {
		return ErrTopicNotFound
	}
	return s.topics.SetLocked(ctx, topicID, false)
}

// PinTopic pins a topic to the top of its forum listing. Staff/admin only.
func (s *ForumService) PinTopic(ctx context.Context, topicID int64, perms model.Permissions) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	if _, err := s.topics.GetByID(ctx, topicID); err != nil {
		return ErrTopicNotFound
	}
	return s.topics.SetPinned(ctx, topicID, true)
}

// UnpinTopic removes the pin from a topic. Staff/admin only.
func (s *ForumService) UnpinTopic(ctx context.Context, topicID int64, perms model.Permissions) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	if _, err := s.topics.GetByID(ctx, topicID); err != nil {
		return ErrTopicNotFound
	}
	return s.topics.SetPinned(ctx, topicID, false)
}

// RenameTopic changes a topic's title. Staff/admin only.
func (s *ForumService) RenameTopic(ctx context.Context, topicID int64, perms model.Permissions, title string) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("%w: title cannot be empty", ErrInvalidTopic)
	}
	if len(title) > 200 {
		return fmt.Errorf("%w: title too long", ErrInvalidTopic)
	}
	if _, err := s.topics.GetByID(ctx, topicID); err != nil {
		return ErrTopicNotFound
	}
	return s.topics.UpdateTitle(ctx, topicID, title)
}

// MoveTopic moves a topic to a different forum. Staff/admin only.
// Uses a transaction to update topic forum_id and recalculate counts on both forums.
func (s *ForumService) MoveTopic(ctx context.Context, topicID int64, perms model.Permissions, targetForumID int64) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}
	if topic.ForumID == targetForumID {
		return ErrSameForum
	}
	if _, err := s.forums.GetByID(ctx, targetForumID); err != nil {
		return ErrForumNotFound
	}
	oldForumID := topic.ForumID
	if s.db != nil {
		return repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE forum_topics SET forum_id = $1, updated_at = NOW() WHERE id = $2", targetForumID, topicID); err != nil {
				return fmt.Errorf("move topic: %w", err)
			}
			// Recalculate old forum counts
			if _, err := tx.ExecContext(ctx, `
				UPDATE forums SET
					topic_count = COALESCE((SELECT COUNT(*) FROM forum_topics WHERE forum_id = $1), 0),
					post_count = COALESCE((SELECT COUNT(*) FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1), 0),
					last_post_id = (SELECT fp.id FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1 ORDER BY fp.created_at DESC LIMIT 1)
				WHERE id = $1`, oldForumID); err != nil {
				return fmt.Errorf("recalculate old forum: %w", err)
			}
			// Recalculate new forum counts
			if _, err := tx.ExecContext(ctx, `
				UPDATE forums SET
					topic_count = COALESCE((SELECT COUNT(*) FROM forum_topics WHERE forum_id = $1), 0),
					post_count = COALESCE((SELECT COUNT(*) FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1), 0),
					last_post_id = (SELECT fp.id FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1 ORDER BY fp.created_at DESC LIMIT 1)
				WHERE id = $1`, targetForumID); err != nil {
				return fmt.Errorf("recalculate new forum: %w", err)
			}
			return nil
		})
	}
	// Non-transactional fallback (tests without DB)
	if err := s.topics.UpdateForumID(ctx, topicID, targetForumID); err != nil {
		return fmt.Errorf("move topic: %w", err)
	}
	if err := s.forums.RecalculateCounts(ctx, oldForumID); err != nil {
		return fmt.Errorf("recalculate old forum: %w", err)
	}
	return s.forums.RecalculateCounts(ctx, targetForumID)
}

// DeleteTopic deletes a topic and all its posts. Staff/admin only.
// Uses a transaction to delete the topic (CASCADE deletes posts) and recalculate forum counts.
func (s *ForumService) DeleteTopic(ctx context.Context, topicID int64, perms model.Permissions) error {
	if err := s.requireStaff(perms); err != nil {
		return ErrForumAccessDenied
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}
	forumID := topic.ForumID
	if s.db != nil {
		return repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM forum_topics WHERE id = $1", topicID); err != nil {
				return fmt.Errorf("delete topic: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE forums SET
					topic_count = COALESCE((SELECT COUNT(*) FROM forum_topics WHERE forum_id = $1), 0),
					post_count = COALESCE((SELECT COUNT(*) FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1), 0),
					last_post_id = (SELECT fp.id FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1 ORDER BY fp.created_at DESC LIMIT 1)
				WHERE id = $1`, forumID); err != nil {
				return fmt.Errorf("recalculate forum: %w", err)
			}
			return nil
		})
	}
	// Non-transactional fallback (tests without DB)
	if err := s.topics.Delete(ctx, topicID); err != nil {
		return fmt.Errorf("delete topic: %w", err)
	}
	return s.forums.RecalculateCounts(ctx, forumID)
}

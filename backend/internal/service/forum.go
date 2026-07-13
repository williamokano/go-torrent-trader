package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrForumNotFound            = errors.New("forum not found")
	ErrTopicNotFound            = errors.New("topic not found")
	ErrTopicLocked              = errors.New("topic is locked")
	ErrForumAccessDenied        = errors.New("forum access denied")
	ErrInvalidTopic             = errors.New("invalid topic")
	ErrInvalidPost              = errors.New("invalid post")
	ErrInvalidReply             = errors.New("invalid reply reference")
	ErrInvalidSearch            = errors.New("invalid search query")
	ErrPostNotFound             = errors.New("post not found")
	ErrPostEditDenied           = errors.New("not authorized to edit this post")
	ErrPostDeleteDenied         = errors.New("not authorized to delete this post")
	ErrCannotDeleteFirstPost    = errors.New("cannot delete the first post of a topic; delete the topic instead")
	ErrSameForum                = errors.New("topic is already in this forum")
	ErrTopicDeleteDenied        = errors.New("topic delete denied")
	ErrForumCategoryNotFound    = errors.New("forum category not found")
	ErrForumCategoryHasForums   = errors.New("forum category has forums and cannot be deleted")
	ErrInvalidForumCategory     = errors.New("invalid forum category")
	ErrInvalidForum             = errors.New("invalid forum")
	ErrForumHasTopics           = errors.New("forum has topics and cannot be deleted")
	ErrModHierarchyDenied       = errors.New("insufficient permissions: cannot moderate topics by higher-ranked users")
	ErrPostNotDeleted           = errors.New("post is not deleted")
)

const viewCountDebounce = 15 * time.Minute
const ownerGracePeriod = 30 * time.Minute

type ForumService struct {
	db         *sql.DB
	categories repository.ForumCategoryRepository
	forums     repository.ForumRepository
	topics     repository.ForumTopicRepository
	posts      repository.ForumPostRepository
	users      repository.UserRepository
	groups     repository.GroupRepository
	eventBus     event.Bus
	viewMu       sync.Mutex
	viewDebounce map[string]time.Time
}

func NewForumService(db *sql.DB, categories repository.ForumCategoryRepository, forums repository.ForumRepository, topics repository.ForumTopicRepository, posts repository.ForumPostRepository, users repository.UserRepository, groups repository.GroupRepository, eventBus event.Bus) *ForumService {
	return &ForumService{db: db, categories: categories, forums: forums, topics: topics, posts: posts, users: users, groups: groups, eventBus: eventBus, viewDebounce: make(map[string]time.Time)}
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

// GetFirstPostID returns the ID of the first post in a topic by insertion order,
// regardless of deletion status. Used to prevent deleting the opening post.
func (s *ForumService) GetFirstPostID(ctx context.Context, topicID int64) (int64, error) {
	return s.posts.GetFirstPostIDByTopic(ctx, topicID)
}

// CreateTopic creates a new topic with the first post in a forum.
// All operations are wrapped in a single database transaction to ensure atomicity.
func (s *ForumService) CreateTopic(ctx context.Context, forumID, userID int64, perms model.Permissions, title, body string) (*model.ForumTopic, *model.ForumPost, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		return nil, nil, fmt.Errorf("%w: title cannot be empty", ErrInvalidTopic)
	}
	if utf8.RuneCountInString(title) > 200 {
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
	if forum.MinPostLevel > perms.Level {
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

	actor := event.Actor{ID: userID, Username: user.Username}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumTopicCreatedEvent{
			Base:        event.NewBase(event.ForumTopicCreated, actor),
			TopicID:     topic.ID,
			TopicTitle:  title,
			ForumID:     forumID,
			FirstPostID: post.ID,
		})
		s.eventBus.Publish(ctx, &event.ForumPostCreatedEvent{
			Base:       event.NewBase(event.ForumPostCreated, actor),
			PostID:     post.ID,
			TopicID:    topic.ID,
			TopicTitle: title,
			ForumID:    forumID,
			Body:       body,
		})
	}

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
	if topic.Locked && !perms.IsStaff() { return nil, ErrTopicLocked }
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

	if s.eventBus != nil {
		// Resolve reply-to user ID for the notification listener
		var replyToUserID *int64
		if replyToPostID != nil {
			if rp, rpErr := s.posts.GetByID(ctx, *replyToPostID); rpErr == nil {
				replyToUserID = &rp.UserID
			}
		}
		s.eventBus.Publish(ctx, &event.ForumPostCreatedEvent{
			Base:          event.NewBase(event.ForumPostCreated, event.Actor{ID: userID, Username: user.Username}),
			PostID:        post.ID,
			TopicID:       topicID,
			TopicTitle:    topic.Title,
			ForumID:       topic.ForumID,
			Body:          body,
			ReplyToPostID: replyToPostID,
			ReplyToUserID: replyToUserID,
		})
	}

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

	// Skip update if body hasn't changed
	if body == post.Body {
		return post, nil
	}

	// Record edit history before updating
	edit := &model.ForumPostEdit{
		PostID:   postID,
		EditedBy: &userID,
		OldBody:  post.Body,
		NewBody:  body,
	}
	if err := s.posts.CreateEdit(ctx, edit); err != nil {
		return nil, fmt.Errorf("creating edit history: %w", err)
	}

	post.Body = body
	post.EditedBy = &userID
	if err := s.posts.Update(ctx, post); err != nil {
		return nil, fmt.Errorf("update post: %w", err)
	}

	// Re-fetch to get updated edited_at and denormalized fields
	updated, err := s.posts.GetByID(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("re-fetch post after edit: %w", err)
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
			if _, err := tx.ExecContext(ctx, "UPDATE forum_posts SET deleted_at = NOW(), deleted_by = $2 WHERE id = $1", post.ID, userID); err != nil {
				return fmt.Errorf("soft delete post: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forum_topics SET post_count = GREATEST(post_count - 1, 0) WHERE id = $1", post.TopicID); err != nil {
				return fmt.Errorf("decrement topic post count: %w", err)
			}
			// Recalculate topic last_post using a single atomic subquery (excluding soft-deleted)
			if _, err := tx.ExecContext(ctx, `
				UPDATE forum_topics SET
					last_post_id = sub.id,
					last_post_at = sub.created_at,
					updated_at = NOW()
				FROM (
					SELECT id, created_at FROM forum_posts
					WHERE topic_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1
				) sub
				WHERE forum_topics.id = $1`, post.TopicID); err != nil {
				return fmt.Errorf("recalculate topic last post: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forums SET post_count = GREATEST(post_count - 1, 0) WHERE id = $1", topic.ForumID); err != nil {
				return fmt.Errorf("decrement forum post count: %w", err)
			}
			// Recalculate forum last_post using a single atomic subquery (excluding soft-deleted)
			if _, err := tx.ExecContext(ctx, `
				UPDATE forums SET last_post_id = sub.id
				FROM (
					SELECT p.id FROM forum_posts p
					JOIN forum_topics t ON t.id = p.topic_id
					WHERE t.forum_id = $1 AND p.deleted_at IS NULL
					ORDER BY p.created_at DESC LIMIT 1
				) sub
				WHERE forums.id = $1`, topic.ForumID); err != nil {
				return fmt.Errorf("recalculate forum last post: %w", err)
			}
			return nil
		})
	}

	// Non-transactional path (used in tests with nil db)
	if err := s.posts.SoftDelete(ctx, post.ID, userID); err != nil {
		return fmt.Errorf("soft delete post: %w", err)
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

// RestorePost restores a soft-deleted forum post. Staff only.
// It increments topic/forum counters and recalculates last post.
func (s *ForumService) RestorePost(ctx context.Context, postID int64, userID int64, perms model.Permissions) error {
	if !perms.IsStaff() {
		return ErrForumAccessDenied
	}

	post, err := s.posts.GetByID(ctx, postID)
	if err != nil {
		return ErrPostNotFound
	}
	if post.DeletedAt == nil {
		return ErrPostNotDeleted
	}

	topic, err := s.topics.GetByID(ctx, post.TopicID)
	if err != nil {
		return ErrTopicNotFound
	}

	if s.db != nil {
		return repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE forum_posts SET deleted_at = NULL, deleted_by = NULL WHERE id = $1", post.ID); err != nil {
				return fmt.Errorf("restore post: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forum_topics SET post_count = post_count + 1 WHERE id = $1", post.TopicID); err != nil {
				return fmt.Errorf("increment topic post count: %w", err)
			}
			// Recalculate topic last_post (excluding soft-deleted)
			if _, err := tx.ExecContext(ctx, `
				UPDATE forum_topics SET
					last_post_id = sub.id,
					last_post_at = sub.created_at,
					updated_at = NOW()
				FROM (
					SELECT id, created_at FROM forum_posts
					WHERE topic_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1
				) sub
				WHERE forum_topics.id = $1`, post.TopicID); err != nil {
				return fmt.Errorf("recalculate topic last post: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE forums SET post_count = post_count + 1 WHERE id = $1", topic.ForumID); err != nil {
				return fmt.Errorf("increment forum post count: %w", err)
			}
			// Recalculate forum last_post (excluding soft-deleted)
			if _, err := tx.ExecContext(ctx, `
				UPDATE forums SET last_post_id = sub.id
				FROM (
					SELECT p.id FROM forum_posts p
					JOIN forum_topics t ON t.id = p.topic_id
					WHERE t.forum_id = $1 AND p.deleted_at IS NULL
					ORDER BY p.created_at DESC LIMIT 1
				) sub
				WHERE forums.id = $1`, topic.ForumID); err != nil {
				return fmt.Errorf("recalculate forum last post: %w", err)
			}
			return nil
		})
	}

	// Non-transactional path (used in tests with nil db)
	if err := s.posts.Restore(ctx, post.ID); err != nil {
		return fmt.Errorf("restore post: %w", err)
	}
	if err := s.topics.IncrementPostCount(ctx, post.TopicID, 1); err != nil {
		return fmt.Errorf("increment topic post count: %w", err)
	}
	if err := s.topics.RecalculateLastPost(ctx, post.TopicID); err != nil {
		return fmt.Errorf("recalculate topic last post: %w", err)
	}
	if err := s.forums.IncrementPostCount(ctx, topic.ForumID, 1); err != nil {
		return fmt.Errorf("increment forum post count: %w", err)
	}
	if err := s.forums.RecalculateLastPost(ctx, topic.ForumID); err != nil {
		return fmt.Errorf("recalculate forum last post: %w", err)
	}
	return nil
}

// ListPostEdits returns the edit history for a forum post. Staff only.
func (s *ForumService) ListPostEdits(ctx context.Context, postID int64, perms model.Permissions) ([]model.ForumPostEdit, error) {
	if !perms.IsStaff() {
		return nil, ErrForumAccessDenied
	}

	if _, err := s.posts.GetByID(ctx, postID); err != nil {
		return nil, ErrPostNotFound
	}

	edits, err := s.posts.ListEdits(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("list post edits: %w", err)
	}
	return edits, nil
}

func (s *ForumService) requireStaff(perms model.Permissions) error {
	if !perms.IsStaff() {
		return ErrForumAccessDenied
	}
	return nil
}

// checkModHierarchy ensures a non-admin moderator cannot moderate topics
// created by admin-level users. Admins bypass this check entirely.
// If users or groups repositories are nil (e.g. in tests), the check is skipped.
func (s *ForumService) checkModHierarchy(ctx context.Context, perms model.Permissions, topicAuthorID int64) error {
	if perms.IsAdmin {
		return nil
	}
	if s.users == nil || s.groups == nil {
		log.Printf("WARNING: hierarchy check skipped, users or groups repository is nil")
		return nil
	}
	// Look up the topic author, then their group to check IsAdmin.
	author, err := s.users.GetByID(ctx, topicAuthorID)
	if err != nil {
		// Author deleted or not found — allow moderation
		return nil
	}
	authorGroup, gErr := s.groups.GetByID(ctx, author.GroupID)
	if gErr != nil {
		return fmt.Errorf("get author group: %w", gErr)
	}
	if authorGroup.IsAdmin {
		return ErrModHierarchyDenied
	}
	return nil
}

// CanModerate checks whether the given user/perms can moderate the specified topic.
// Returns true if the user is staff and passes the hierarchy check, or if the owner
// is within the grace period.
func (s *ForumService) CanModerate(ctx context.Context, topic *model.ForumTopic, userID int64, perms model.Permissions) bool {
	if userID == topic.UserID && perms.CanForum && time.Since(topic.CreatedAt) <= ownerGracePeriod {
		return true
	}
	if !perms.IsStaff() {
		return false
	}
	if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
		return false
	}
	return true
}

// LockTopic locks a topic so no new replies can be posted.
// Staff can lock any topic (subject to hierarchy). Topic owners can lock their own
// topics within the grace period (30 minutes from creation).
func (s *ForumService) LockTopic(ctx context.Context, topicID int64, userID int64, perms model.Permissions, actor event.Actor, reason string) error {
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}

	// Owner self-action: allow lock within grace period without staff
	if userID == topic.UserID && !perms.IsStaff() && perms.CanForum && time.Since(topic.CreatedAt) <= ownerGracePeriod {
		// Owner can lock within grace period — skip staff and hierarchy checks
	} else {
		if err := s.requireStaff(perms); err != nil {
			return err
		}
		if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
			return err
		}
	}

	if err := s.topics.SetLocked(ctx, topicID, true); err != nil {
		return err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumTopicLockedEvent{
			Base:       event.NewBase(event.ForumTopicLocked, actor),
			TopicID:    topicID,
			TopicTitle: topic.Title,
			Reason:     reason,
		})
	}
	return nil
}

// UnlockTopic unlocks a previously locked topic. Staff/admin only.
func (s *ForumService) UnlockTopic(ctx context.Context, topicID int64, perms model.Permissions, actor event.Actor, reason string) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}
	if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
		return err
	}
	if err := s.topics.SetLocked(ctx, topicID, false); err != nil {
		return err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumTopicUnlockedEvent{
			Base:       event.NewBase(event.ForumTopicUnlocked, actor),
			TopicID:    topicID,
			TopicTitle: topic.Title,
			Reason:     reason,
		})
	}
	return nil
}

// PinTopic pins a topic to the top of its forum listing. Staff/admin only.
func (s *ForumService) PinTopic(ctx context.Context, topicID int64, perms model.Permissions, actor event.Actor, reason string) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}
	if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
		return err
	}
	if err := s.topics.SetPinned(ctx, topicID, true); err != nil {
		return err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumTopicPinnedEvent{
			Base:       event.NewBase(event.ForumTopicPinned, actor),
			TopicID:    topicID,
			TopicTitle: topic.Title,
			Reason:     reason,
		})
	}
	return nil
}

// UnpinTopic removes the pin from a topic. Staff/admin only.
func (s *ForumService) UnpinTopic(ctx context.Context, topicID int64, perms model.Permissions, actor event.Actor, reason string) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}
	if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
		return err
	}
	if err := s.topics.SetPinned(ctx, topicID, false); err != nil {
		return err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumTopicUnpinnedEvent{
			Base:       event.NewBase(event.ForumTopicUnpinned, actor),
			TopicID:    topicID,
			TopicTitle: topic.Title,
			Reason:     reason,
		})
	}
	return nil
}

// RenameTopic changes a topic's title. The topic author or staff can rename.
// Non-staff authors cannot rename locked topics and must have can_forum privilege.
func (s *ForumService) RenameTopic(ctx context.Context, topicID int64, userID int64, perms model.Permissions, title string, actor event.Actor, reason string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("%w: title cannot be empty", ErrInvalidTopic)
	}
	if utf8.RuneCountInString(title) > 200 {
		return fmt.Errorf("%w: title too long", ErrInvalidTopic)
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}

	// Authorization: topic author or staff
	if topic.UserID != userID && !perms.IsStaff() {
		return ErrForumAccessDenied
	}

	// Non-staff checks
	if !perms.IsStaff() {
		// can_forum check
		user, userErr := s.users.GetByID(ctx, userID)
		if userErr != nil {
			return fmt.Errorf("get user: %w", userErr)
		}
		if !user.CanForum {
			return ErrForumAccessDenied
		}
		// Locked topic check: author cannot rename locked topics
		if topic.Locked {
			return ErrTopicLocked
		}
	} else {
		// Staff: check hierarchy
		if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
			return err
		}
	}

	oldTitle := topic.Title
	if err := s.topics.UpdateTitle(ctx, topicID, title); err != nil {
		return err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumTopicRenamedEvent{
			Base:     event.NewBase(event.ForumTopicRenamed, actor),
			TopicID:  topicID,
			OldTitle: oldTitle,
			NewTitle: title,
			Reason:   reason,
		})
	}
	return nil
}

// MoveTopic moves a topic to a different forum. Staff/admin only.
// Uses a transaction to update topic forum_id and recalculate counts on both forums.
// Note: staff can intentionally move topics to forums with higher MinGroupLevel,
// which may hide the topic from its original author. This is expected moderation behavior.
func (s *ForumService) MoveTopic(ctx context.Context, topicID int64, perms model.Permissions, targetForumID int64, actor event.Actor, reason string) error {
	if err := s.requireStaff(perms); err != nil {
		return err
	}
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}
	if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
		return err
	}
	if topic.ForumID == targetForumID {
		return ErrSameForum
	}
	if _, err := s.forums.GetByID(ctx, targetForumID); err != nil {
		return ErrForumNotFound
	}
	oldForumID := topic.ForumID
	publishEvent := func() {
		if s.eventBus != nil {
			s.eventBus.Publish(ctx, &event.ForumTopicMovedEvent{
				Base:       event.NewBase(event.ForumTopicMoved, actor),
				TopicID:    topicID,
				TopicTitle: topic.Title,
				OldForumID: oldForumID,
				NewForumID: targetForumID,
				Reason:     reason,
			})
		}
	}
	if s.db != nil {
		err := repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
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
		if err != nil {
			return err
		}
		publishEvent()
		return nil
	}
	// Non-transactional fallback (tests without DB)
	if err := s.topics.UpdateForumID(ctx, topicID, targetForumID); err != nil {
		return fmt.Errorf("move topic: %w", err)
	}
	if err := s.forums.RecalculateCounts(ctx, oldForumID); err != nil {
		return fmt.Errorf("recalculate old forum: %w", err)
	}
	if err := s.forums.RecalculateCounts(ctx, targetForumID); err != nil {
		return err
	}
	publishEvent()
	return nil
}

// DeleteTopic deletes a topic and all its posts.
// Staff can delete any topic (subject to hierarchy). Topic owners can delete their own
// topics within the grace period (30 minutes from creation).
func (s *ForumService) DeleteTopic(ctx context.Context, topicID int64, userID int64, perms model.Permissions, actor event.Actor, reason string) error {
	topic, err := s.topics.GetByID(ctx, topicID)
	if err != nil {
		return ErrTopicNotFound
	}

	// Owner self-action: allow delete within grace period without staff
	if userID == topic.UserID && !perms.IsStaff() && perms.CanForum && time.Since(topic.CreatedAt) <= ownerGracePeriod {
		// Owner can delete within grace period — skip staff and hierarchy checks
	} else {
		if err := s.requireStaff(perms); err != nil {
			return ErrTopicDeleteDenied
		}
		if err := s.checkModHierarchy(ctx, perms, topic.UserID); err != nil {
			return err
		}
	}

	forumID := topic.ForumID
	topicTitle := topic.Title
	publishEvent := func() {
		if s.eventBus != nil {
			s.eventBus.Publish(ctx, &event.ForumTopicDeletedEvent{
				Base:       event.NewBase(event.ForumTopicDeleted, actor),
				TopicID:    topicID,
				TopicTitle: topicTitle,
				ForumID:    forumID,
				Reason:     reason,
			})
		}
	}
	if s.db != nil {
		err := repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
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
		if err != nil {
			return err
		}
		publishEvent()
		return nil
	}
	// Non-transactional fallback (tests without DB)
	if err := s.topics.Delete(ctx, topicID); err != nil {
		return fmt.Errorf("delete topic: %w", err)
	}
	if err := s.forums.RecalculateCounts(ctx, forumID); err != nil {
		return err
	}
	publishEvent()
	return nil
}

// --- Admin CRUD methods ---

// CreateForumCategoryRequest holds the input for creating a forum category.
type CreateForumCategoryRequest struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// AdminListCategories returns all forum categories (no permission filtering).
func (s *ForumService) AdminListCategories(ctx context.Context) ([]model.ForumCategory, error) {
	return s.categories.List(ctx)
}

// AdminCreateCategory creates a new forum category.
func (s *ForumService) AdminCreateCategory(ctx context.Context, req CreateForumCategoryRequest, actor event.Actor) (*model.ForumCategory, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidForumCategory)
	}
	if utf8.RuneCountInString(name) > 200 {
		return nil, fmt.Errorf("%w: name too long", ErrInvalidForumCategory)
	}

	cat := &model.ForumCategory{
		Name:      name,
		SortOrder: req.SortOrder,
	}
	if err := s.categories.Create(ctx, cat); err != nil {
		return nil, fmt.Errorf("create forum category: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumCategoryCreatedEvent{
			Base:         event.NewBase(event.ForumCategoryCreated, actor),
			CategoryID:   cat.ID,
			CategoryName: cat.Name,
		})
	}
	return cat, nil
}

// UpdateForumCategoryRequest holds the input for updating a forum category.
type UpdateForumCategoryRequest struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// AdminUpdateCategory updates an existing forum category.
func (s *ForumService) AdminUpdateCategory(ctx context.Context, id int64, req UpdateForumCategoryRequest, actor event.Actor) (*model.ForumCategory, error) {
	cat, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, ErrForumCategoryNotFound
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidForumCategory)
	}
	if utf8.RuneCountInString(name) > 200 {
		return nil, fmt.Errorf("%w: name too long", ErrInvalidForumCategory)
	}

	cat.Name = name
	cat.SortOrder = req.SortOrder
	if err := s.categories.Update(ctx, cat); err != nil {
		return nil, fmt.Errorf("update forum category: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumCategoryUpdatedEvent{
			Base:         event.NewBase(event.ForumCategoryUpdated, actor),
			CategoryID:   cat.ID,
			CategoryName: cat.Name,
		})
	}
	return cat, nil
}

// AdminDeleteCategory deletes a forum category if it has no forums.
// Uses a transaction (when db is available) to make the count + delete atomic,
// preventing a TOCTOU race where a forum could be added between the check and delete.
func (s *ForumService) AdminDeleteCategory(ctx context.Context, id int64, actor event.Actor) error {
	cat, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return ErrForumCategoryNotFound
	}

	if s.db != nil {
		err = repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			var count int64
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM forums WHERE category_id = $1", id).Scan(&count); err != nil {
				return fmt.Errorf("check forums: %w", err)
			}
			if count > 0 {
				return ErrForumCategoryHasForums
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM forum_categories WHERE id = $1", id); err != nil {
				return fmt.Errorf("delete forum category: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		// Non-transactional fallback (used in tests with nil db)
		count, err := s.categories.CountForumsByCategory(ctx, id)
		if err != nil {
			return fmt.Errorf("check forums: %w", err)
		}
		if count > 0 {
			return ErrForumCategoryHasForums
		}
		if err := s.categories.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete forum category: %w", err)
		}
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumCategoryDeletedEvent{
			Base:         event.NewBase(event.ForumCategoryDeleted, actor),
			CategoryID:   id,
			CategoryName: cat.Name,
		})
	}
	return nil
}

// CreateForumRequest holds the input for creating a forum.
type CreateForumRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	CategoryID    int64  `json:"category_id"`
	SortOrder     int    `json:"sort_order"`
	MinGroupLevel int    `json:"min_group_level"`
	MinPostLevel  int    `json:"min_post_level"`
}

// AdminListForums returns all forums (no permission filtering).
func (s *ForumService) AdminListForums(ctx context.Context) ([]model.Forum, error) {
	return s.forums.List(ctx)
}

// AdminCreateForum creates a new forum.
func (s *ForumService) AdminCreateForum(ctx context.Context, req CreateForumRequest, actor event.Actor) (*model.Forum, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidForum)
	}
	if utf8.RuneCountInString(name) > 200 {
		return nil, fmt.Errorf("%w: name too long", ErrInvalidForum)
	}
	if req.CategoryID <= 0 {
		return nil, fmt.Errorf("%w: category_id is required", ErrInvalidForum)
	}
	if req.MinGroupLevel < 0 {
		return nil, fmt.Errorf("%w: min_group_level cannot be negative", ErrInvalidForum)
	}
	if req.MinPostLevel < 0 {
		return nil, fmt.Errorf("%w: min_post_level cannot be negative", ErrInvalidForum)
	}

	// Verify category exists
	if _, err := s.categories.GetByID(ctx, req.CategoryID); err != nil {
		return nil, ErrForumCategoryNotFound
	}

	forum := &model.Forum{
		Name:          name,
		Description:   strings.TrimSpace(req.Description),
		CategoryID:    req.CategoryID,
		SortOrder:     req.SortOrder,
		MinGroupLevel: req.MinGroupLevel,
		MinPostLevel:  req.MinPostLevel,
	}
	if err := s.forums.Create(ctx, forum); err != nil {
		return nil, fmt.Errorf("create forum: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumCreatedEvent{
			Base:      event.NewBase(event.ForumCreated, actor),
			ForumID:   forum.ID,
			ForumName: forum.Name,
		})
	}
	return forum, nil
}

// UpdateForumRequest holds the input for updating a forum.
type UpdateForumRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	CategoryID    int64  `json:"category_id"`
	SortOrder     int    `json:"sort_order"`
	MinGroupLevel int    `json:"min_group_level"`
	MinPostLevel  int    `json:"min_post_level"`
}

// AdminUpdateForum updates an existing forum.
func (s *ForumService) AdminUpdateForum(ctx context.Context, id int64, req UpdateForumRequest, actor event.Actor) (*model.Forum, error) {
	forum, err := s.forums.GetByID(ctx, id)
	if err != nil {
		return nil, ErrForumNotFound
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidForum)
	}
	if utf8.RuneCountInString(name) > 200 {
		return nil, fmt.Errorf("%w: name too long", ErrInvalidForum)
	}
	if req.CategoryID <= 0 {
		return nil, fmt.Errorf("%w: category_id is required", ErrInvalidForum)
	}
	if req.MinGroupLevel < 0 {
		return nil, fmt.Errorf("%w: min_group_level cannot be negative", ErrInvalidForum)
	}
	if req.MinPostLevel < 0 {
		return nil, fmt.Errorf("%w: min_post_level cannot be negative", ErrInvalidForum)
	}

	// Verify category exists
	if _, err := s.categories.GetByID(ctx, req.CategoryID); err != nil {
		return nil, ErrForumCategoryNotFound
	}

	forum.Name = name
	forum.Description = strings.TrimSpace(req.Description)
	forum.CategoryID = req.CategoryID
	forum.SortOrder = req.SortOrder
	forum.MinGroupLevel = req.MinGroupLevel
	forum.MinPostLevel = req.MinPostLevel

	if err := s.forums.Update(ctx, forum); err != nil {
		return nil, fmt.Errorf("update forum: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumUpdatedEvent{
			Base:      event.NewBase(event.ForumUpdated, actor),
			ForumID:   forum.ID,
			ForumName: forum.Name,
		})
	}
	return forum, nil
}

// AdminDeleteForum deletes a forum if it has no topics.
// Uses a transaction (when db is available) to make the count + delete atomic,
// preventing a TOCTOU race where a topic could be added between the check and delete.
func (s *ForumService) AdminDeleteForum(ctx context.Context, id int64, actor event.Actor) error {
	forum, err := s.forums.GetByID(ctx, id)
	if err != nil {
		return ErrForumNotFound
	}

	if s.db != nil {
		err = repository.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
			var count int64
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM forum_topics WHERE forum_id = $1", id).Scan(&count); err != nil {
				return fmt.Errorf("check topics: %w", err)
			}
			if count > 0 {
				return ErrForumHasTopics
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM forums WHERE id = $1", id); err != nil {
				return fmt.Errorf("delete forum: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		// Non-transactional fallback (used in tests with nil db)
		count, err := s.forums.CountTopicsByForum(ctx, id)
		if err != nil {
			return fmt.Errorf("check topics: %w", err)
		}
		if count > 0 {
			return ErrForumHasTopics
		}
		if err := s.forums.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete forum: %w", err)
		}
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.ForumDeletedEvent{
			Base:      event.NewBase(event.ForumDeleted, actor),
			ForumID:   id,
			ForumName: forum.Name,
		})
	}
	return nil
}

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Happy-path and authorisation coverage for the forum handlers, driven through a
// real ForumService whose db is nil — the service then takes its non-transactional
// branch and every write lands on these stubs.

// --- forum repository stubs -------------------------------------------------

type stubForumCategoryRepo struct {
	byID       map[int64]*model.ForumCategory
	categories []model.ForumCategory
	created    []*model.ForumCategory
	updated    []*model.ForumCategory
	deleted    []int64
	forumCount int64

	createErr error
	listErr   error
	deleteErr error

	nextID int64
}

func (s *stubForumCategoryRepo) GetByID(_ context.Context, id int64) (*model.ForumCategory, error) {
	c, ok := s.byID[id]
	if !ok {
		return nil, errors.New("forum category not found")
	}
	return c, nil
}
func (s *stubForumCategoryRepo) List(_ context.Context) ([]model.ForumCategory, error) {
	return s.categories, s.listErr
}
func (s *stubForumCategoryRepo) Create(_ context.Context, cat *model.ForumCategory) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.nextID == 0 {
		s.nextID = 1
	}
	cat.ID = s.nextID
	s.nextID++
	s.created = append(s.created, cat)
	return nil
}
func (s *stubForumCategoryRepo) Update(_ context.Context, cat *model.ForumCategory) error {
	s.updated = append(s.updated, cat)
	return nil
}
func (s *stubForumCategoryRepo) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubForumCategoryRepo) CountForumsByCategory(_ context.Context, _ int64) (int64, error) {
	return s.forumCount, nil
}

type stubForumRepo struct {
	forums     map[int64]*model.Forum
	list       []model.Forum
	created    []*model.Forum
	updated    []*model.Forum
	deleted    []int64
	topicCount int64

	getErr    error
	listErr   error
	createErr error
	deleteErr error

	nextID int64
}

func (s *stubForumRepo) GetByID(_ context.Context, id int64) (*model.Forum, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	f, ok := s.forums[id]
	if !ok {
		return nil, errors.New("forum not found")
	}
	return f, nil
}
func (s *stubForumRepo) ListByCategory(_ context.Context, _ int64) ([]model.Forum, error) {
	return nil, nil
}
func (s *stubForumRepo) List(_ context.Context) ([]model.Forum, error) { return s.list, s.listErr }
func (s *stubForumRepo) Create(_ context.Context, forum *model.Forum) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.nextID == 0 {
		s.nextID = 10
	}
	forum.ID = s.nextID
	s.nextID++
	s.created = append(s.created, forum)
	return nil
}
func (s *stubForumRepo) Update(_ context.Context, forum *model.Forum) error {
	s.updated = append(s.updated, forum)
	return nil
}
func (s *stubForumRepo) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubForumRepo) CountTopicsByForum(_ context.Context, _ int64) (int64, error) {
	return s.topicCount, nil
}
func (s *stubForumRepo) IncrementTopicCount(_ context.Context, _ int64, _ int) error { return nil }
func (s *stubForumRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error  { return nil }
func (s *stubForumRepo) UpdateLastPost(_ context.Context, _ int64, _ int64) error    { return nil }
func (s *stubForumRepo) RecalculateLastPost(_ context.Context, _ int64) error        { return nil }
func (s *stubForumRepo) RecalculateCounts(_ context.Context, _ int64) error          { return nil }

type stubForumTopicRepo struct {
	topics  map[int64]*model.ForumTopic
	list    []model.ForumTopic
	total   int64
	created []*model.ForumTopic

	locked   map[int64]bool
	pinned   map[int64]bool
	titles   map[int64]string
	moved    map[int64]int64
	deleted  []int64
	viewed   []int64
	lastPage int
	lastPer  int

	createErr error
	listErr   error
	deleteErr error

	nextID int64
}

func newStubForumTopicRepo() *stubForumTopicRepo {
	return &stubForumTopicRepo{
		topics: map[int64]*model.ForumTopic{},
		locked: map[int64]bool{},
		pinned: map[int64]bool{},
		titles: map[int64]string{},
		moved:  map[int64]int64{},
		nextID: 100,
	}
}

func (s *stubForumTopicRepo) GetByID(_ context.Context, id int64) (*model.ForumTopic, error) {
	t, ok := s.topics[id]
	if !ok {
		return nil, errors.New("topic not found")
	}
	return t, nil
}
func (s *stubForumTopicRepo) ListByForum(_ context.Context, _ int64, page, perPage int) ([]model.ForumTopic, int64, error) {
	s.lastPage, s.lastPer = page, perPage
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.list, s.total, nil
}
func (s *stubForumTopicRepo) Create(_ context.Context, topic *model.ForumTopic) error {
	if s.createErr != nil {
		return s.createErr
	}
	topic.ID = s.nextID
	s.nextID++
	topic.CreatedAt = time.Now()
	s.created = append(s.created, topic)
	stored := *topic
	s.topics[topic.ID] = &stored
	return nil
}
func (s *stubForumTopicRepo) IncrementViewCount(_ context.Context, id int64) error {
	s.viewed = append(s.viewed, id)
	return nil
}
func (s *stubForumTopicRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error { return nil }
func (s *stubForumTopicRepo) UpdateLastPost(_ context.Context, _ int64, _ int64, _ time.Time) error {
	return nil
}
func (s *stubForumTopicRepo) RecalculateLastPost(_ context.Context, _ int64) error { return nil }
func (s *stubForumTopicRepo) SetLocked(_ context.Context, id int64, locked bool) error {
	s.locked[id] = locked
	return nil
}
func (s *stubForumTopicRepo) SetPinned(_ context.Context, id int64, pinned bool) error {
	s.pinned[id] = pinned
	return nil
}
func (s *stubForumTopicRepo) UpdateTitle(_ context.Context, id int64, title string) error {
	s.titles[id] = title
	return nil
}
func (s *stubForumTopicRepo) UpdateForumID(_ context.Context, id int64, forumID int64) error {
	s.moved[id] = forumID
	return nil
}
func (s *stubForumTopicRepo) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}

type stubForumPostRepo struct {
	posts       map[int64]*model.ForumPost
	list        []model.ForumPost
	total       int64
	firstPostID int64
	edits       []model.ForumPostEdit
	search      []model.ForumSearchResult
	searchTotal int64

	created   []*model.ForumPost
	updated   []*model.ForumPost
	softDel   []int64
	restored  []int64
	editsSave []*model.ForumPostEdit

	createErr error
	listErr   error
	searchErr error
	updateErr error

	nextID int64
}

func newStubForumPostRepo() *stubForumPostRepo {
	return &stubForumPostRepo{posts: map[int64]*model.ForumPost{}, nextID: 500}
}

func (s *stubForumPostRepo) GetByID(_ context.Context, id int64) (*model.ForumPost, error) {
	p, ok := s.posts[id]
	if !ok {
		return nil, errors.New("post not found")
	}
	return p, nil
}
func (s *stubForumPostRepo) ListByTopic(_ context.Context, _ int64, _, _ int) ([]model.ForumPost, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.list, s.total, nil
}
func (s *stubForumPostRepo) Create(_ context.Context, post *model.ForumPost) error {
	if s.createErr != nil {
		return s.createErr
	}
	post.ID = s.nextID
	s.nextID++
	post.CreatedAt = time.Now()
	s.created = append(s.created, post)
	stored := *post
	s.posts[post.ID] = &stored
	return nil
}
func (s *stubForumPostRepo) Update(_ context.Context, post *model.ForumPost) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = append(s.updated, post)
	return nil
}
func (s *stubForumPostRepo) Delete(_ context.Context, _ int64) error { return nil }
func (s *stubForumPostRepo) CountByUser(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (s *stubForumPostRepo) Search(_ context.Context, _ string, _ *int64, _ int, _, _ int) ([]model.ForumSearchResult, int64, error) {
	if s.searchErr != nil {
		return nil, 0, s.searchErr
	}
	return s.search, s.searchTotal, nil
}
func (s *stubForumPostRepo) GetFirstPostIDByTopic(_ context.Context, _ int64) (int64, error) {
	return s.firstPostID, nil
}
func (s *stubForumPostRepo) SoftDelete(_ context.Context, id int64, _ int64) error {
	s.softDel = append(s.softDel, id)
	return nil
}
func (s *stubForumPostRepo) Restore(_ context.Context, id int64) error {
	s.restored = append(s.restored, id)
	return nil
}
func (s *stubForumPostRepo) CreateEdit(_ context.Context, edit *model.ForumPostEdit) error {
	s.editsSave = append(s.editsSave, edit)
	return nil
}
func (s *stubForumPostRepo) ListEdits(_ context.Context, _ int64) ([]model.ForumPostEdit, error) {
	return s.edits, nil
}

type stubGroupRepo struct {
	groups map[int64]*model.Group
}

func (s *stubGroupRepo) GetByID(_ context.Context, id int64) (*model.Group, error) {
	g, ok := s.groups[id]
	if !ok {
		return nil, errors.New("group not found")
	}
	return g, nil
}
func (s *stubGroupRepo) List(_ context.Context) ([]model.Group, error) { return nil, nil }

// --- harness ----------------------------------------------------------------

type forumDeps struct {
	categories *stubForumCategoryRepo
	forums     *stubForumRepo
	topics     *stubForumTopicRepo
	posts      *stubForumPostRepo
	users      *stubUserRepo
	groups     *stubGroupRepo
}

// newForumDeps wires a forum with id 1 (open to everyone), a member (id 7) and a
// topic (id 100) authored by that member.
func newForumDeps() *forumDeps {
	return &forumDeps{
		categories: &stubForumCategoryRepo{byID: map[int64]*model.ForumCategory{
			1: {ID: 1, Name: "Main"},
		}},
		forums: &stubForumRepo{forums: map[int64]*model.Forum{
			1: {ID: 1, CategoryID: 1, Name: "General", MinGroupLevel: 0, MinPostLevel: 0},
			2: {ID: 2, CategoryID: 1, Name: "Staff only", MinGroupLevel: 90, MinPostLevel: 90},
		}},
		topics: newStubForumTopicRepo(),
		posts:  newStubForumPostRepo(),
		users: newStubUserRepo(
			&model.User{ID: 7, Username: "member", GroupID: 5, CanForum: true},
			&model.User{ID: 1, Username: "admin", GroupID: 1, CanForum: true},
		),
		groups: &stubGroupRepo{groups: map[int64]*model.Group{
			1: {ID: 1, Name: "Admin", IsAdmin: true},
			5: {ID: 5, Name: "User"},
		}},
	}
}

func (d *forumDeps) handler() *ForumHandler {
	return NewForumHandler(service.NewForumService(
		nil, d.categories, d.forums, d.topics, d.posts, d.users, d.groups, event.NewInMemoryBus(),
	))
}

// memberPerms is a plain member who may read and write in the forums.
func memberPerms() model.Permissions {
	return model.Permissions{Level: 20, CanForum: true, Username: "member"}
}

func staffPerms() model.Permissions {
	return model.Permissions{Level: 90, CanForum: true, IsModerator: true, Username: "mod"}
}

// --- list forums / get forum ------------------------------------------------

func TestListForumsHidesForumsAboveTheUsersLevel(t *testing.T) {
	d := newForumDeps()
	d.categories.categories = []model.ForumCategory{{ID: 1, Name: "Main"}}
	d.forums.list = []model.Forum{
		{ID: 1, CategoryID: 1, Name: "General", MinGroupLevel: 0},
		{ID: 2, CategoryID: 1, Name: "Staff only", MinGroupLevel: 90},
	}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums", nil), 7, memberPerms())
	w := httptest.NewRecorder()
	h.HandleListForums(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	cats := decodeBody(t, w)["categories"].([]interface{})
	if len(cats) != 1 {
		t.Fatalf("categories = %v, want 1", cats)
	}
	forums := cats[0].(map[string]interface{})["forums"].([]interface{})
	if len(forums) != 1 {
		t.Fatalf("forums = %v, want only the one the member may see", forums)
	}
	if forums[0].(map[string]interface{})["name"] != "General" {
		t.Errorf("visible forum = %v, want \"General\" — the staff forum must not be listed",
			forums[0].(map[string]interface{})["name"])
	}
}

func TestListForumsSurfacesStoreFailure(t *testing.T) {
	d := newForumDeps()
	d.categories.listErr = errStub
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums", nil), 7, memberPerms())
	w := httptest.NewRecorder()
	h.HandleListForums(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetForumReturnsForum(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/1", nil), 7, memberPerms())
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetForum(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	forum := decodeBody(t, w)["forum"].(map[string]interface{})
	for _, key := range []string{"id", "category_id", "name", "description", "topic_count", "post_count", "min_group_level", "min_post_level"} {
		if _, present := forum[key]; !present {
			t.Errorf("forum response is missing key %q", key)
		}
	}
}

func TestGetForumForbiddenAboveUsersLevel(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/2", nil), 7, memberPerms())
	req = withURLParam(req, "id", "2")
	w := httptest.NewRecorder()
	h.HandleGetForum(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — the forum requires a higher group level", w.Code)
	}
}

func TestGetForumReturns404WhenMissing(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/99", nil), 7, memberPerms())
	req = withURLParam(req, "id", "99")
	w := httptest.NewRecorder()
	h.HandleGetForum(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- list topics ------------------------------------------------------------

func TestListTopicsRejectsInvalidForumID(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/x/topics", nil), 7, memberPerms())
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleListTopics(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListTopicsReturnsTopicsAndCreatePermission(t *testing.T) {
	d := newForumDeps()
	d.topics.list = []model.ForumTopic{{ID: 100, ForumID: 1, UserID: 7, Title: "Hello", Username: "member"}}
	d.topics.total = 1
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/1/topics", nil), 7, memberPerms())
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleListTopics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	if items, ok := body["topics"].([]interface{}); !ok || len(items) != 1 {
		t.Fatalf("topics = %v, want 1 item", body["topics"])
	}
	if body["can_create_topic"] != true {
		t.Error("can_create_topic = false, want true for an authenticated member above min_post_level")
	}
	if body["page"].(float64) != 1 || body["per_page"].(float64) != 25 {
		t.Errorf("page/per_page = %v/%v, want 1/25", body["page"], body["per_page"])
	}
}

// An anonymous visitor may read an open forum but must not be told they can post.
func TestListTopicsDeniesTopicCreationToAnonymousVisitors(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	// No user in context at all.
	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/forums/1/topics", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleListTopics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if decodeBody(t, w)["can_create_topic"] != false {
		t.Error("can_create_topic = true for an anonymous visitor, want false")
	}
}

func TestListTopicsForbiddenAboveUsersLevel(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/2/topics", nil), 7, memberPerms())
	req = withURLParam(req, "id", "2")
	w := httptest.NewRecorder()
	h.HandleListTopics(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// --- get topic --------------------------------------------------------------

func TestGetTopicReturnsPostsAndBumpsTheViewCount(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Title: "Hello", CreatedAt: time.Now()}
	d.posts.firstPostID = 500
	d.posts.list = []model.ForumPost{
		{ID: 500, TopicID: 100, UserID: 7, Body: "first", Username: "member"},
		{ID: 501, TopicID: 100, UserID: 9, Body: "reply", Username: "other"},
	}
	d.posts.total = 2
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.topics.viewed) != 1 || d.topics.viewed[0] != 100 {
		t.Errorf("view count bumped for %v, want [100]", d.topics.viewed)
	}
	body := decodeBody(t, w)
	posts := body["posts"].([]interface{})
	if len(posts) != 2 {
		t.Fatalf("posts = %v, want 2", posts)
	}
	if posts[0].(map[string]interface{})["is_first_post"] != true {
		t.Error("the opening post is not flagged is_first_post")
	}
	if posts[1].(map[string]interface{})["is_first_post"] != false {
		t.Error("a reply is wrongly flagged as the first post")
	}
	// The author is inside the 30-minute grace period, so they may moderate.
	if body["can_moderate"] != true {
		t.Error("can_moderate = false, want true for the topic owner within the grace period")
	}
}

// A soft-deleted post's body must not be sent to non-staff readers.
func TestGetTopicHidesDeletedPostBodiesFromMembers(t *testing.T) {
	d := newForumDeps()
	deletedAt := time.Now()
	deletedBy := int64(1)
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9, CreatedAt: time.Now().Add(-time.Hour)}
	d.posts.list = []model.ForumPost{
		{ID: 501, TopicID: 100, UserID: 9, Body: "naughty", DeletedAt: &deletedAt, DeletedBy: &deletedBy},
	}
	d.posts.total = 1
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	post := body["posts"].([]interface{})[0].(map[string]interface{})
	if post["body"] != "" {
		t.Errorf("body = %q, want it redacted for a non-staff reader", post["body"])
	}
	if post["is_deleted"] != true {
		t.Error("is_deleted = false, want true")
	}
	if _, leaked := post["deleted_by"]; leaked {
		t.Error("deleted_by leaked to a non-staff reader")
	}
	if body["can_moderate"] != false {
		t.Error("can_moderate = true for a plain member on someone else's topic")
	}
}

func TestGetTopicShowsDeletedPostBodiesToStaff(t *testing.T) {
	d := newForumDeps()
	deletedAt := time.Now()
	deletedBy := int64(1)
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, CreatedAt: time.Now().Add(-time.Hour)}
	d.posts.list = []model.ForumPost{
		{ID: 501, TopicID: 100, UserID: 7, Body: "naughty", DeletedAt: &deletedAt, DeletedBy: &deletedBy},
	}
	d.posts.total = 1
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	body := decodeBody(t, w)
	post := body["posts"].([]interface{})[0].(map[string]interface{})
	if post["body"] != "naughty" {
		t.Errorf("body = %q, want staff to see the original text", post["body"])
	}
	if post["deleted_by"] != float64(1) {
		t.Errorf("deleted_by = %v, want 1 for staff", post["deleted_by"])
	}
	if body["can_moderate"] != true {
		t.Error("can_moderate = false for a moderator on a member's topic")
	}
}

// A moderator must not be able to moderate a topic opened by an admin.
func TestGetTopicDeniesModerationOfAdminTopics(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 1, CreatedAt: time.Now().Add(-time.Hour)}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 9, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if decodeBody(t, w)["can_moderate"] != false {
		t.Error("can_moderate = true — a moderator must not outrank an admin author")
	}
}

func TestGetTopicReturns404WhenMissing(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/404", nil), 7, memberPerms())
	req = withURLParam(req, "id", "404")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetTopicSurfacesPostListFailure(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.listErr = errStub
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- create topic / post ----------------------------------------------------

func TestCreateTopicStoresTopicAndOpeningPost(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	body := `{"title":"  New topic  ","body":"  Hello world  "}`
	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/1/topics", strings.NewReader(body)), 7, memberPerms())
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleCreateTopic(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(d.topics.created) != 1 || d.topics.created[0].Title != "New topic" {
		t.Fatalf("created topics = %+v, want one with a trimmed title", d.topics.created)
	}
	if d.topics.created[0].UserID != 7 {
		t.Errorf("topic user_id = %d, want the authenticated user 7", d.topics.created[0].UserID)
	}
	if len(d.posts.created) != 1 || d.posts.created[0].Body != "Hello world" {
		t.Fatalf("created posts = %+v, want one with a trimmed body", d.posts.created)
	}

	resp := decodeBody(t, w)
	if _, ok := resp["topic"]; !ok {
		t.Error("response is missing the topic")
	}
	if _, ok := resp["post"]; !ok {
		t.Error("response is missing the opening post")
	}
}

func TestCreateTopicRejectsEmptyTitleAndBody(t *testing.T) {
	for _, body := range []string{`{"title":"   ","body":"b"}`, `{"title":"t","body":"   "}`} {
		d := newForumDeps()
		h := d.handler()

		req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/1/topics", strings.NewReader(body)), 7, memberPerms())
		req = withURLParam(req, "id", "1")
		w := httptest.NewRecorder()
		h.HandleCreateTopic(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
	}
}

// A user whose forum privileges were revoked may still read, but must not post.
func TestCreateTopicForbiddenWhenForumPrivilegeRevoked(t *testing.T) {
	d := newForumDeps()
	d.users.users[7].CanForum = false
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/1/topics",
		strings.NewReader(`{"title":"t","body":"b"}`)), 7, memberPerms())
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleCreateTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — can_forum=false must block writing", w.Code)
	}
	if len(d.topics.created) != 0 {
		t.Error("a restricted user managed to create a topic")
	}
}

func TestCreateTopicForbiddenBelowMinPostLevel(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/2/topics",
		strings.NewReader(`{"title":"t","body":"b"}`)), 7, memberPerms())
	req = withURLParam(req, "id", "2")
	w := httptest.NewRecorder()
	h.HandleCreateTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestCreatePostStoresReply(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9}
	d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 9}
	h := d.handler()

	body := `{"body":"my reply","reply_to_post_id":500}`
	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/posts", strings.NewReader(body)), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleCreatePost(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(d.posts.created) != 1 {
		t.Fatalf("created %d posts, want 1", len(d.posts.created))
	}
	created := d.posts.created[0]
	if created.UserID != 7 || created.TopicID != 100 || created.Body != "my reply" {
		t.Errorf("created post = %+v, want a reply by user 7 in topic 100", created)
	}
	if created.ReplyToPostID == nil || *created.ReplyToPostID != 500 {
		t.Errorf("reply_to_post_id = %v, want 500", created.ReplyToPostID)
	}
}

// reply_to_post_id must point at a post in the same topic.
func TestCreatePostRejectsReplyToForeignPost(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9}
	d.posts.posts[900] = &model.ForumPost{ID: 900, TopicID: 999, UserID: 9} // different topic
	h := d.handler()

	body := `{"body":"reply","reply_to_post_id":900}`
	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/posts", strings.NewReader(body)), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleCreatePost(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a reply pointing at another topic's post", w.Code)
	}
}

func TestCreatePostRejectedInLockedTopic(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9, Locked: true}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/posts",
		strings.NewReader(`{"body":"reply"}`)), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleCreatePost(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — the topic is locked", w.Code)
	}
	if len(d.posts.created) != 0 {
		t.Error("a reply was stored in a locked topic")
	}
}

// Staff may still reply in a locked topic.
func TestCreatePostAllowedForStaffInLockedTopic(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9, Locked: true}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/posts",
		strings.NewReader(`{"body":"mod note"}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleCreatePost(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for staff in a locked topic; body: %s", w.Code, w.Body.String())
	}
}

func TestCreatePostRejectsEmptyBody(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/posts",
		strings.NewReader(`{"body":"   "}`)), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleCreatePost(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- edit / delete / restore posts ------------------------------------------

func TestEditPostRecordsHistoryAndUpdatesBody(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 7, Body: "old text"}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/posts/500",
		strings.NewReader(`{"body":"new text"}`)), 7, memberPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.posts.editsSave) != 1 {
		t.Fatalf("recorded %d edits, want 1 — edit history must be kept", len(d.posts.editsSave))
	}
	edit := d.posts.editsSave[0]
	// BE-9.23: a diff against the previous version is stored, not a full
	// old_body/new_body snapshot (the diff algorithm's round-trip
	// correctness is covered in the service package's own tests).
	if edit.Diff == "" {
		t.Errorf("edit = %+v, want a non-empty diff recorded", edit)
	}
	if edit.OldBody != "" || edit.NewBody != "" {
		t.Errorf("edit = %+v, want no full-body snapshot stored", edit)
	}
	if len(d.posts.updated) != 1 || d.posts.updated[0].Body != "new text" {
		t.Errorf("updated = %+v, want the post body replaced", d.posts.updated)
	}
}

// When staff edits on behalf of another user, an optional reason travels
// through to the stored edit record (BE-9.23). It's staff-only visible —
// enforced by ListPostEdits already being staff-gated, not by this endpoint.
func TestEditPostByStaffStoresOptionalReason(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 7, Body: "old text"}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/posts/500",
		strings.NewReader(`{"body":"new text","reason":"cleaned up formatting"}`)), 1, staffPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.posts.editsSave) != 1 {
		t.Fatalf("recorded %d edits, want 1", len(d.posts.editsSave))
	}
	edit := d.posts.editsSave[0]
	if edit.Reason == nil || *edit.Reason != "cleaned up formatting" {
		t.Errorf("Reason = %v, want %q", edit.Reason, "cleaned up formatting")
	}
	if edit.EditedBy == nil || *edit.EditedBy != 1 {
		t.Errorf("EditedBy = %v, want the staff member's ID (1), not the post author's", edit.EditedBy)
	}
}

// Editing someone else's post is only for staff.
func TestEditPostForbiddenForOtherPeoplesPosts(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9}
	d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 9, Body: "theirs"}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/posts/500",
		strings.NewReader(`{"body":"vandalised"}`)), 7, memberPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if len(d.posts.updated) != 0 {
		t.Error("someone else's post was modified")
	}
}

func TestEditPostReturns404WhenMissing(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/posts/500",
		strings.NewReader(`{"body":"x"}`)), 7, memberPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestEditPostRejectsEmptyBody(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/posts/500",
		strings.NewReader(`{"body":"  "}`)), 7, memberPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeletePostSoftDeletesReply(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.posts[501] = &model.ForumPost{ID: 501, TopicID: 100, UserID: 7, Body: "oops"}
	d.posts.firstPostID = 500 // 501 is a reply, not the opener
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/posts/501", nil), 7, memberPerms())
	req = withURLParam(req, "id", "501")
	w := httptest.NewRecorder()
	h.HandleDeletePost(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(d.posts.softDel) != 1 || d.posts.softDel[0] != 501 {
		t.Errorf("soft-deleted %v, want [501]", d.posts.softDel)
	}
}

// The opening post cannot be deleted — the topic must go instead.
func TestDeletePostRefusesToDeleteTheOpeningPost(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 7}
	d.posts.firstPostID = 500
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/posts/500", nil), 7, memberPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleDeletePost(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for the first post of a topic", w.Code)
	}
	if len(d.posts.softDel) != 0 {
		t.Error("the opening post was deleted anyway")
	}
}

func TestDeletePostForbiddenForOtherPeoplesPosts(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9}
	d.posts.posts[501] = &model.ForumPost{ID: 501, TopicID: 100, UserID: 9}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/posts/501", nil), 7, memberPerms())
	req = withURLParam(req, "id", "501")
	w := httptest.NewRecorder()
	h.HandleDeletePost(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestRestorePostRequiresAuth(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/forums/posts/501/restore", nil), "id", "501")
	w := httptest.NewRecorder()
	h.HandleRestorePost(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRestorePostRejectsInvalidID(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/posts/0/restore", nil), 1, staffPerms())
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleRestorePost(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRestorePostForbiddenForNonStaff(t *testing.T) {
	d := newForumDeps()
	deletedAt := time.Now()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.posts[501] = &model.ForumPost{ID: 501, TopicID: 100, UserID: 7, DeletedAt: &deletedAt}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/posts/501/restore", nil), 7, memberPerms())
	req = withURLParam(req, "id", "501")
	w := httptest.NewRecorder()
	h.HandleRestorePost(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — restoring is staff-only", w.Code)
	}
	if len(d.posts.restored) != 0 {
		t.Error("a member restored a deleted post")
	}
}

func TestRestorePostRestoresSoftDeletedPost(t *testing.T) {
	d := newForumDeps()
	deletedAt := time.Now()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.posts[501] = &model.ForumPost{ID: 501, TopicID: 100, UserID: 7, DeletedAt: &deletedAt}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/posts/501/restore", nil), 1, staffPerms())
	req = withURLParam(req, "id", "501")
	w := httptest.NewRecorder()
	h.HandleRestorePost(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(d.posts.restored) != 1 || d.posts.restored[0] != 501 {
		t.Errorf("restored %v, want [501]", d.posts.restored)
	}
}

// Restoring a post that was never deleted is a client error.
func TestRestorePostRejectsLivePost(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	d.posts.posts[501] = &model.ForumPost{ID: 501, TopicID: 100, UserID: 7}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/posts/501/restore", nil), 1, staffPerms())
	req = withURLParam(req, "id", "501")
	w := httptest.NewRecorder()
	h.HandleRestorePost(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a post that is not deleted", w.Code)
	}
}

// --- edit history -----------------------------------------------------------

func TestListPostEditsRejectsInvalidID(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/posts/x/edits", nil), 1, staffPerms())
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleListPostEdits(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// Edit history reveals what was said before a redaction — staff only.
func TestListPostEditsForbiddenForNonStaff(t *testing.T) {
	d := newForumDeps()
	d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 7}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/posts/500/edits", nil), 7, memberPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleListPostEdits(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — edit history is staff-only", w.Code)
	}
}

func TestListPostEditsReturnsHistoryToStaff(t *testing.T) {
	d := newForumDeps()
	d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 7}
	d.posts.edits = []model.ForumPostEdit{{ID: 1, PostID: 500, OldBody: "old", NewBody: "new", IsSnapshot: true}}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/posts/500/edits", nil), 1, staffPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleListPostEdits(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	edits, ok := decodeBody(t, w)["edits"].([]interface{})
	if !ok || len(edits) != 1 {
		t.Fatalf("edits = %v, want 1 item", edits)
	}
}

func TestListPostEditsReturns404WhenPostMissing(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/posts/500/edits", nil), 1, staffPerms())
	req = withURLParam(req, "id", "500")
	w := httptest.NewRecorder()
	h.HandleListPostEdits(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- moderation actions -----------------------------------------------------

func TestLockAndUnlockTopic(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, CreatedAt: time.Now().Add(-time.Hour)}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/lock",
		strings.NewReader(`{"reason":"off topic"}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleLockTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("lock: status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !d.topics.locked[100] {
		t.Error("the topic was not locked")
	}

	req = withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/unlock",
		strings.NewReader(`{"reason":"resolved"}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w = httptest.NewRecorder()
	h.HandleUnlockTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unlock: status = %d, want 200", w.Code)
	}
	if d.topics.locked[100] {
		t.Error("the topic was not unlocked")
	}
}

func TestLockTopicForbiddenForNonStaffOnOtherPeoplesTopics(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9, CreatedAt: time.Now()}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/lock", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleLockTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if d.topics.locked[100] {
		t.Error("a member locked someone else's topic")
	}
}

// The author may lock their own thread within the 30-minute grace period.
func TestLockTopicAllowedForOwnerWithinGracePeriod(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, CreatedAt: time.Now()}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/lock", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleLockTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !d.topics.locked[100] {
		t.Error("the owner could not lock their own fresh topic")
	}
}

func TestUnlockTopicForbiddenForNonStaff(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Locked: true, CreatedAt: time.Now()}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/unlock", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleUnlockTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — unlocking is staff-only", w.Code)
	}
}

func TestPinAndUnpinTopic(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/pin", nil), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandlePinTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("pin: status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !d.topics.pinned[100] {
		t.Error("the topic was not pinned")
	}

	req = withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/unpin", nil), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w = httptest.NewRecorder()
	h.HandleUnpinTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unpin: status = %d, want 200", w.Code)
	}
	if d.topics.pinned[100] {
		t.Error("the topic was not unpinned")
	}
}

func TestPinTopicForbiddenForNonStaff(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/pin", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandlePinTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — pinning is staff-only even for the topic owner", w.Code)
	}
}

func TestRenameTopicRequiresAuth(t *testing.T) {
	d := newForumDeps()
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/forums/topics/100/title",
		strings.NewReader(`{"title":"new"}`)), "id", "100")
	w := httptest.NewRecorder()
	h.HandleRenameTopic(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRenameTopicUpdatesTitle(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Title: "old"}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/topics/100/title",
		strings.NewReader(`{"title":"new title","reason":"typo"}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleRenameTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if d.topics.titles[100] != "new title" {
		t.Errorf("title = %q, want \"new title\"", d.topics.titles[100])
	}
}

func TestRenameTopicRejectsEmptyTitle(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Title: "old"}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/topics/100/title",
		strings.NewReader(`{"title":"   "}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleRenameTopic(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestMoveTopicUpdatesForumID(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/move",
		strings.NewReader(`{"forum_id":2,"reason":"wrong board"}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleMoveTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if d.topics.moved[100] != 2 {
		t.Errorf("moved to forum %d, want 2", d.topics.moved[100])
	}
}

// Moving a topic into the forum it already lives in is a no-op error.
func TestMoveTopicRejectsSameForum(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/move",
		strings.NewReader(`{"forum_id":1}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleMoveTopic(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestMoveTopicForbiddenForNonStaff(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/move",
		strings.NewReader(`{"forum_id":2}`)), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleMoveTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if _, moved := d.topics.moved[100]; moved {
		t.Error("a member moved a topic")
	}
}

func TestDeleteTopicRemovesTopic(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, CreatedAt: time.Now().Add(-time.Hour)}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/topics/100",
		strings.NewReader(`{"reason":"spam"}`)), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleDeleteTopic(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(d.topics.deleted) != 1 || d.topics.deleted[0] != 100 {
		t.Errorf("deleted %v, want [100]", d.topics.deleted)
	}
}

func TestDeleteTopicForbiddenForNonStaff(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9, CreatedAt: time.Now().Add(-time.Hour)}
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/topics/100", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleDeleteTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if len(d.topics.deleted) != 0 {
		t.Error("a member deleted someone else's topic")
	}
}

// --- search -----------------------------------------------------------------

func TestSearchForumReturnsResultsWithDeepLinkPages(t *testing.T) {
	d := newForumDeps()
	d.posts.search = []model.ForumSearchResult{
		{PostID: 1, TopicID: 100, TopicTitle: "T", ForumID: 1, Username: "member",
			Snippet: "a !!MARK_START!!hit!!MARK_END!! here", PostNumber: 26},
		{PostID: 2, TopicID: 100, ForumID: 1, PostNumber: 1},
	}
	d.posts.searchTotal = 2
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/search?q=hit", nil), 7, memberPerms())
	w := httptest.NewRecorder()
	h.HandleSearchForum(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items := decodeBody(t, w)["results"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("results = %v, want 2", items)
	}

	first := items[0].(map[string]interface{})
	// Post 26 with 25 posts per page lives on page 2.
	if first["page"] != float64(2) {
		t.Errorf("page = %v, want 2 for post number 26", first["page"])
	}
	if first["snippet"] != "a <mark>hit</mark> here" {
		t.Errorf("snippet = %v, want the markers turned into <mark> tags", first["snippet"])
	}
	if items[1].(map[string]interface{})["page"] != float64(1) {
		t.Errorf("page = %v, want 1 for the first post", items[1].(map[string]interface{})["page"])
	}
}

func TestSearchForumSurfacesStoreFailure(t *testing.T) {
	d := newForumDeps()
	d.posts.searchErr = errStub
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/search?q=hit", nil), 7, memberPerms())
	w := httptest.NewRecorder()
	h.HandleSearchForum(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

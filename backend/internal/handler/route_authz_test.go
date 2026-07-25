package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// Authorization is attached in router.go, not in the handlers:
//
//	r.Route("/admin", ...)
//	  r.Group(func(r chi.Router) { r.Use(mw.RequireAdmin); ... })
//	  r.Group(func(r chi.Router) { r.Use(mw.RequireStaff); ... })
//
// Every other handler test in this package calls the handler function directly
// with an injected context, which bypasses middleware entirely. That means a
// route registered one brace outside its guard is a privilege escalation that
// NO existing test can see.
//
// So this walks the real router and asserts the guard for every admin and staff
// route that is actually registered — including ones added in the future, since
// the route table is enumerated rather than hard-coded.

// stubDashboardRepo is the one repository with no internal stub yet (the
// existing one lives in the external handler_test package).
type stubDashboardRepo struct{}

func (s *stubDashboardRepo) GetStats(_ context.Context) (*repository.DashboardStats, error) {
	return &repository.DashboardStats{}, nil
}

// The connector repositories are inert here: this test only cares that the
// routes register (a nil service silently drops them) and that the guard in
// front of them holds. Behaviour lives in admin_connectors_test.go.

type nopConnectorRepo struct{}

func (nopConnectorRepo) Create(context.Context, *model.NotificationConnector) error { return nil }
func (nopConnectorRepo) GetByID(context.Context, int64) (*model.NotificationConnector, error) {
	return nil, sql.ErrNoRows
}
func (nopConnectorRepo) List(context.Context) ([]model.NotificationConnector, error) {
	return nil, nil
}
func (nopConnectorRepo) ListEnabled(context.Context) ([]model.NotificationConnector, error) {
	return nil, nil
}
func (nopConnectorRepo) Update(context.Context, *model.NotificationConnector) error { return nil }
func (nopConnectorRepo) Delete(context.Context, int64) error                        { return sql.ErrNoRows }
func (nopConnectorRepo) CountByKind(context.Context, string) (int64, error)         { return 0, nil }

type nopConnectorDeliveryRepo struct{}

func (nopConnectorDeliveryRepo) InsertPending(context.Context, *model.ConnectorDelivery) (bool, error) {
	return false, nil
}
func (nopConnectorDeliveryRepo) ListDue(context.Context, int64, time.Time, int) ([]model.ConnectorDelivery, error) {
	return nil, nil
}
func (nopConnectorDeliveryRepo) ClaimForDelivery(context.Context, int64, time.Time, time.Time) (bool, error) {
	return false, nil
}
func (nopConnectorDeliveryRepo) CountSentSince(context.Context, int64, time.Time) (int64, error) {
	return 0, nil
}
func (nopConnectorDeliveryRepo) MarkSent(context.Context, int64, string) error { return nil }
func (nopConnectorDeliveryRepo) MarkFailedAttempt(context.Context, int64, int, string, *time.Time) error {
	return nil
}
func (nopConnectorDeliveryRepo) ListByInstance(context.Context, int64, int, int) ([]model.ConnectorDelivery, int64, error) {
	return nil, 0, nil
}
func (nopConnectorDeliveryRepo) LatestStatusByInstance(context.Context) (map[int64]model.ConnectorDelivery, error) {
	return nil, nil
}
func (nopConnectorDeliveryRepo) InstancesWithDue(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (nopConnectorDeliveryRepo) DeleteOld(context.Context, time.Time) (int64, error) { return 0, nil }

// fullRouterDeps wires every service so that every route registers. A nil
// service silently drops its routes from the table — which would make this test
// pass by simply not looking at them.
func fullRouterDeps(t *testing.T) *Deps {
	t.Helper()

	bus := event.NewInMemoryBus()
	users := newStubUserRepo(&model.User{ID: 1, Username: "member", CanChat: true})
	groups := &stubGroupRepo{}
	sessions := testutil.NewMemorySessionStore()

	chatSvc := service.NewChatService(&stubChatMessageRepo{}, &stubChatMuteRepo{}, users, bus)
	settingsSvc := service.NewSiteSettingsService(newStubSiteSettingsRepo(), bus)
	torrentSvc := service.NewTorrentService(nil, &stubTorrentRepo{}, users, &stubStorage{},
		service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, bus, &stubReseedRepo{})

	// StatsCache needs a real (mocked) DB/Redis pair — no stub interface exists
	// for it, since the service type itself is concrete, not an interface.
	statsDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = statsDB.Close() })
	statsRedis := miniredis.RunT(t)
	statsRDB := redis.NewClient(&redis.Options{Addr: statsRedis.Addr()})
	t.Cleanup(func() { _ = statsRDB.Close() })

	return &Deps{
		AuthService: service.NewAuthService(users, sessions,
			testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost", bus),
		SessionStore:        sessions,
		UserService:         service.NewUserService(users, sessions, groups, &stubPeerRepo{}, &stubTorrentRepo{}),
		TorrentService:      torrentSvc,
		AdminService:        service.NewAdminService(users, groups, bus),
		ReportService:       service.NewReportService(newStubReportRepo(), &stubTorrentRepo{}, users, bus),
		WarningService:      service.NewWarningService(newStubWarningRepo(), users, newStubMessageRepo(), bus),
		SiteSettingsService: settingsSvc,
		BanService:          service.NewBanService(&stubBanRepo{}, bus),
		CategoryService:     service.NewCategoryService(&stubCategoryRepo{}),
		RestrictionService:  service.NewRestrictionService(newStubRestrictionRepo(), users, bus),
		NewsService:         service.NewNewsService(newStubNewsRepo(), users, bus),
		ActivityLogService:  service.NewActivityLogService(&stubActivityLogRepo{}),
		ChatService:         chatSvc,
		ChatHub:             NewChatHub(chatSvc, sessions, settingsSvc, bus, nil),
		ForumService: service.NewForumService(nil, &stubForumCategoryRepo{}, &stubForumRepo{},
			newStubForumTopicRepo(), newStubForumPostRepo(), users, groups, bus),
		CheatFlagRepo: &stubCheatFlagRepo{},
		DashboardRepo: &stubDashboardRepo{},
		BackupService: &backupManagerStub{},
		ConnectorService: service.NewConnectorService(nopConnectorRepo{}, nopConnectorDeliveryRepo{},
			connector.NewRegistry(), bus, "http://localhost"),
		AnnounceHub: NewAnnounceHub(sessions, &stubFeedAccess{allowed: true}),
		UserRepo:    users,
		StatsCache:  service.NewStatsCache(statsDB, statsRDB, 30*time.Second),
	}
}

// route is one row of the router's table.
type route struct {
	method  string
	pattern string
}

var chiParam = regexp.MustCompile(`\{[^}]+\}`)

// concrete turns "/api/v1/admin/users/{id}" into a requestable path. The value
// is irrelevant: authorization is rejected before the handler parses anything.
func (r route) concrete() string {
	return chiParam.ReplaceAllString(r.pattern, "1")
}

// walkRoutes enumerates every route the router actually registered.
func walkRoutes(t *testing.T, router chi.Router) []route {
	t.Helper()

	var routes []route
	err := chi.Walk(router, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, route{method: method, pattern: pattern})
		return nil
	})
	if err != nil {
		t.Fatalf("walking routes: %v", err)
	}
	return routes
}

// request issues a request through the full middleware stack.
func request(t *testing.T, router chi.Router, r route, token string) int {
	t.Helper()

	path := r.concrete()
	if token != "" {
		path += "?token=" + token
	}
	req := httptest.NewRequest(r.method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

// login registers a session and returns its bearer token.
func login(t *testing.T, sessions *testutil.MemorySessionStore, userID int64, perms model.Permissions) string {
	t.Helper()

	token := fmt.Sprintf("token-%d-%v", userID, perms.IsAdmin)
	err := sessions.Create(&service.Session{
		UserID:      userID,
		AccessToken: token,
		Permissions: perms,
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("creating session: %v", err)
	}
	return token
}

// --- the tests --------------------------------------------------------------

// Every /admin route must refuse an unauthenticated caller. If one answers
// anything else, it is reachable without a session.
func TestEveryAdminRouteRejectsAnonymous(t *testing.T) {
	deps := fullRouterDeps(t)
	router := NewRouter(deps)

	var checked int
	for _, r := range walkRoutes(t, router) {
		if !strings.HasPrefix(r.pattern, "/api/v1/admin") {
			continue
		}
		checked++

		if code := request(t, router, r, ""); code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d for an anonymous caller, want 401 — this route is not behind auth",
				r.method, r.pattern, code)
		}
	}

	// Guard the guard: if the walk finds nothing, the test above is vacuous.
	if checked < 20 {
		t.Fatalf("only %d admin routes found — the router is under-wired and this test proves nothing", checked)
	}
	t.Logf("checked %d admin routes", checked)
}

// A logged-in ordinary member must not reach any /admin route. This is the
// privilege-escalation check: a route registered outside the RequireAdmin /
// RequireStaff group would answer 2xx/4xx-other here instead of 403.
func TestEveryAdminRouteRejectsOrdinaryMember(t *testing.T) {
	deps := fullRouterDeps(t)
	router := NewRouter(deps)
	sessions := deps.SessionStore.(*testutil.MemorySessionStore)

	member := login(t, sessions, 1, model.Permissions{Level: 10}) // not admin, not staff

	var checked int
	for _, r := range walkRoutes(t, router) {
		if !strings.HasPrefix(r.pattern, "/api/v1/admin") {
			continue
		}
		checked++

		code := request(t, router, r, member)
		if code != http.StatusForbidden {
			t.Errorf("%s %s = %d for an ordinary member, want 403 — this route is not behind RequireAdmin/RequireStaff",
				r.method, r.pattern, code)
		}
	}

	if checked < 20 {
		t.Fatalf("only %d admin routes found — the router is under-wired and this test proves nothing", checked)
	}
	t.Logf("checked %d admin routes against an ordinary member", checked)
}

// The routes that are public are public on purpose. Pinning them means an
// accidental move *into* an auth group is caught too — a silent outage rather
// than a silent hole, but still a regression.
func TestPublicRoutesStayPublic(t *testing.T) {
	router := NewRouter(fullRouterDeps(t))

	public := []route{
		{http.MethodGet, "/healthz"},
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/api/v1/auth/forgot-password"},
		{http.MethodGet, "/api/v1/auth/registration-mode"},
	}

	for _, r := range public {
		// A 401 alone is not proof of a guard: /auth/login legitimately answers
		// 401 for bad credentials. The auth middleware is distinguishable by its
		// error code, so key on that rather than the status.
		if code := rejectedByAuthMiddleware(t, router, r); code {
			t.Errorf("%s %s is behind the auth middleware — it is meant to be reachable without a session",
				r.method, r.pattern)
		}
	}
}

// rejectedByAuthMiddleware reports whether the request was turned away by
// RequireAuth (error code "unauthorized") rather than by the handler itself.
func rejectedByAuthMiddleware(t *testing.T, router chi.Router, r route) bool {
	t.Helper()

	req := httptest.NewRequest(r.method, r.concrete(), strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		return false
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		return false
	}
	return body.Error.Code == "unauthorized"
}

// An authenticated non-admin must still reach ordinary member routes: the
// checks above would also pass if RequireAdmin were accidentally applied to
// everything.
func TestMemberRoutesRemainReachableForMembers(t *testing.T) {
	deps := fullRouterDeps(t)
	router := NewRouter(deps)
	sessions := deps.SessionStore.(*testutil.MemorySessionStore)

	member := login(t, sessions, 1, model.Permissions{Level: 10})

	for _, r := range []route{
		{http.MethodGet, "/api/v1/notifications"},
		{http.MethodGet, "/api/v1/messages/inbox"},
		{http.MethodGet, "/api/v1/stats"},
	} {
		code := request(t, router, r, member)
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Errorf("%s %s = %d for a logged-in member, want it reachable", r.method, r.pattern, code)
		}
	}
}

// Site stats leak membership/activity counts, so on a private tracker with no
// anonymous browsing (docs/NOT_PORTING.md §9) an anonymous caller must be
// rejected by the auth middleware, not just answer something non-2xx.
func TestStatsRouteRejectsAnonymous(t *testing.T) {
	router := NewRouter(fullRouterDeps(t))

	r := route{http.MethodGet, "/api/v1/stats"}
	if !rejectedByAuthMiddleware(t, router, r) {
		t.Errorf("%s %s is reachable without a session — it must require auth", r.method, r.pattern)
	}
}

// The live announcement stream is registered outside the auth middleware group
// because EventSource cannot set an Authorization header, so the handler is
// solely responsible for rejecting an anonymous caller. A route that opts out of
// the middleware is exactly the one worth pinning here.
func TestAnnounceStreamRequiresASession(t *testing.T) {
	deps := fullRouterDeps(t)
	router := NewRouter(deps)

	for _, path := range []string{
		"/api/v1/announce-stream",
		"/api/v1/announce-stream?token=nonsense",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s = %d, want 401", path, w.Code)
		}
	}
}

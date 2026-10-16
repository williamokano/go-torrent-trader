package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// sessionQueryTimeout bounds each statement. service.SessionStore takes no
// context, so this is the only thing stopping a stalled database from holding
// an authenticated request — every one of which reads a session — forever.
const sessionQueryTimeout = 5 * time.Second

// SessionStore implements service.SessionStore on the sessions table.
//
// It stores token hashes, never tokens (see migration 092). A lookup hashes the
// presented token and puts it back on the returned Session, so callers see the
// same Session the Redis store returned.
//
// A session records who is signed in, not what they may do. Every lookup reads
// the member's account and class as they are now (#291): a disabled account has
// no live sessions, and a member's permissions are their current group's, the
// moment either changes — whichever code path changed it. A permission snapshot
// taken at login, as sessions used to carry, outlived bans and demotions for as
// long as the client kept refreshing.
type SessionStore struct {
	db *sql.DB
}

var _ service.SessionStore = (*SessionStore)(nil)

// NewSessionStore returns a PostgreSQL-backed session store.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func sessionTokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func sessionCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), sessionQueryTimeout)
}

// sessionColumns are the columns a session row is written with.
const sessionColumns = `user_id, device_name, ip,
	created_at, last_active_at, access_expires_at, refresh_expires_at`

// liveSessionSelect reads a session together with the account and class it
// belongs to, so a lookup answers with current authority. The join on users
// with enabled = true is what makes a ban take effect on the next request; the
// join on groups is what makes a demotion, a promotion or an edit to the class
// itself do the same.
const liveSessionSelect = `SELECT s.user_id, s.device_name, s.ip,
	s.created_at, s.last_active_at, s.access_expires_at, s.refresh_expires_at,
	u.username,
	g.id, g.name, g.level, g.can_upload, g.can_download, g.can_invite,
	g.can_comment, g.can_forum, g.is_admin, g.is_moderator, g.is_immune,
	g.can_self_approve
	FROM sessions s
	JOIN users u ON u.id = s.user_id AND u.enabled = true
	JOIN groups g ON g.id = u.group_id
	WHERE `

// Create stores a new session.
func (s *SessionStore) Create(session *service.Session) error {
	ctx, cancel := sessionCtx()
	defer cancel()
	return insertSession(ctx, s.db, session)
}

// execer is the part of *sql.DB and *sql.Tx that insertSession needs.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertSession writes a session row. Session.GroupID and Permissions are not
// stored: lookups derive them from the member's current account and class.
func insertSession(ctx context.Context, db execer, session *service.Session) error {
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions
		(access_token_hash, refresh_token_hash, `+sessionColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		sessionTokenHash(session.AccessToken), sessionTokenHash(session.RefreshToken),
		session.UserID, session.DeviceName, session.IP,
		session.CreatedAt, session.LastActive, session.ExpiresAt, session.RefreshExpiresAt,
	); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetByAccessToken returns the session whose access token this is, or nil if
// there is none or the access token has expired.
func (s *SessionStore) GetByAccessToken(token string) (*service.Session, error) {
	sess, err := s.get(`s.access_token_hash = $1 AND s.access_expires_at > $2`, token)
	if sess != nil {
		sess.AccessToken = token
	}
	return sess, err
}

// GetByRefreshToken returns the session whose refresh token this is, or nil if
// there is none or the refresh token has expired. It works after the access
// token has expired, which is what refreshing is for.
func (s *SessionStore) GetByRefreshToken(token string) (*service.Session, error) {
	sess, err := s.get(`s.refresh_token_hash = $1 AND s.refresh_expires_at > $2`, token)
	if sess != nil {
		sess.RefreshToken = token
	}
	return sess, err
}

// get reads one live session. Expiry is compared against the application's
// clock, not the database's NOW(): the expiry times were computed from it
// (AuthService sets them from time.Now()), and mixing the two would shift
// every expiry by whatever the clocks disagree.
func (s *SessionStore) get(where, token string) (*service.Session, error) {
	ctx, cancel := sessionCtx()
	defer cancel()

	var (
		sess  service.Session
		group model.Group
	)
	err := s.db.QueryRowContext(ctx, liveSessionSelect+where,
		sessionTokenHash(token), time.Now(),
	).Scan(&sess.UserID, &sess.DeviceName, &sess.IP,
		&sess.CreatedAt, &sess.LastActive, &sess.ExpiresAt, &sess.RefreshExpiresAt,
		&sess.Permissions.Username,
		&group.ID, &group.Name, &group.Level, &group.CanUpload, &group.CanDownload, &group.CanInvite,
		&group.CanComment, &group.CanForum, &group.IsAdmin, &group.IsModerator, &group.IsImmune,
		&group.CanSelfApprove)
	if errors.Is(err, sql.ErrNoRows) {
		// No such session — or the account behind it is disabled, which is the
		// same answer: there is nobody signed in.
		return nil, nil
	}
	if err != nil {
		// Not "no session": the caller refuses the request either way, but
		// with a retryable error rather than a 401 that logs the member out.
		return nil, fmt.Errorf("%w: read session: %v", service.ErrSessionStoreUnavailable, err)
	}
	username := sess.Permissions.Username
	sess.Permissions = model.PermissionsFromGroup(&group)
	sess.Permissions.Username = username
	sess.GroupID = group.ID
	return &sess, nil
}

// Delete revokes the session with this access token.
func (s *SessionStore) Delete(accessToken string) {
	s.exec("delete session", `DELETE FROM sessions WHERE access_token_hash = $1`, sessionTokenHash(accessToken))
}

// DeleteByRefreshToken revokes the session with this refresh token, including
// after its access token has expired (#231).
func (s *SessionStore) DeleteByRefreshToken(refreshToken string) {
	s.exec("delete session by refresh token",
		`DELETE FROM sessions WHERE refresh_token_hash = $1`, sessionTokenHash(refreshToken))
}

// DeleteByUserID revokes every session the user has.
//
// One row per session, keyed by user, is what makes "log out all devices" reach
// every device: there is no index entry to fall out of step with the
// credentials, which is how the Redis store's version of this missed sessions
// whose access token had expired (#231).
func (s *SessionStore) DeleteByUserID(userID int64) {
	s.exec("delete user sessions", `DELETE FROM sessions WHERE user_id = $1`, userID)
}

// DeleteByUserIDExcept revokes every session the user has except the one with
// this access token.
func (s *SessionStore) DeleteByUserIDExcept(userID int64, keepAccessToken string) {
	s.exec("delete other user sessions",
		`DELETE FROM sessions WHERE user_id = $1 AND access_token_hash <> $2`,
		userID, sessionTokenHash(keepAccessToken))
}

// Rotate replaces the session holding oldRefreshToken with newSession, in one
// transaction.
//
// It inserts only if its own DELETE removed the old row. That is what makes a
// refresh single-use under concurrency: of two refreshes racing with the same
// token, the second DELETE waits on the first's row lock, then finds the row
// gone and removes nothing — so it returns ErrInvalidToken instead of minting a
// second pair. The same check stops a refresh from resurrecting a session that
// "log out all devices" or a ban deleted between the caller's lookup and here.
// (The Redis store inserted regardless, and had both holes.)
func (s *SessionStore) Rotate(oldRefreshToken string, newSession *service.Session) error {
	ctx, cancel := sessionCtx()
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session rotation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM sessions WHERE refresh_token_hash = $1 AND refresh_expires_at > $2`,
		sessionTokenHash(oldRefreshToken), time.Now())
	if err != nil {
		return fmt.Errorf("remove rotated session: %w", err)
	}
	removed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove rotated session: rows affected: %w", err)
	}
	if removed != 1 {
		return service.ErrInvalidToken
	}
	if err := insertSession(ctx, tx, newSession); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session rotation: %w", err)
	}
	return nil
}

// TouchLastActive records activity on the session, at most once per
// service.SessionTouchInterval. Callers that have just read the session should skip
// the call when LastActive is already fresh (the auth middleware does), which
// saves the round trip; the WHERE clause is the backstop for racing requests.
func (s *SessionStore) TouchLastActive(accessToken string) {
	now := time.Now()
	s.exec("touch session", `UPDATE sessions SET last_active_at = $2
		WHERE access_token_hash = $1 AND last_active_at < $3`,
		sessionTokenHash(accessToken), now, now.Add(-service.SessionTouchInterval))
}

// PurgeExpired deletes sessions whose refresh token has expired: no credential
// can reach them any more. Redis did this with key TTLs; here the maintenance
// job calls it.
func (s *SessionStore) PurgeExpired(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE refresh_expires_at <= $1`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("purge expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge expired sessions: rows affected: %w", err)
	}
	return n, nil
}

// exec runs a statement for one of the interface's error-less methods. A
// failure is logged — never with the token — since there is no caller to
// return it to.
func (s *SessionStore) exec(what, query string, args ...any) {
	ctx, cancel := sessionCtx()
	defer cancel()
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		slog.Error("session store: failed to "+what, "error", err)
	}
}

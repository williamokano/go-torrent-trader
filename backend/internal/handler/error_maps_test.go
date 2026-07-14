package handler

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// These mappers are the whole contract between a service error and the status
// the client sees. Getting one wrong means a refusal reads as a server fault,
// or a missing row reads as success. The default arm matters most: an unknown
// error must be a 500, never a 200 and never a 4xx that tells the caller to
// stop retrying.

func assertStatus(t *testing.T, name string, mapper func(w *httptest.ResponseRecorder, err error), err error, want int) {
	t.Helper()

	w := httptest.NewRecorder()
	mapper(w, err)
	if w.Code != want {
		t.Errorf("%s(%v) = %d, want %d", name, err, w.Code, want)
	}
}

func TestHandleCommentErrorMapping(t *testing.T) {
	m := func(w *httptest.ResponseRecorder, err error) { handleCommentError(w, err) }

	for _, tc := range []struct {
		err  error
		want int
	}{
		{service.ErrInvalidComment, 400},
		{service.ErrInvalidRating, 400},
		{service.ErrCommentNotFound, 404},
		{service.ErrTorrentNotFound, 404},
		{service.ErrForbidden, 403},
		{errors.New("database exploded"), 500},
	} {
		assertStatus(t, "handleCommentError", m, tc.err, tc.want)
	}
}

func TestHandleInviteErrorMapping(t *testing.T) {
	m := func(w *httptest.ResponseRecorder, err error) { handleInviteError(w, err) }

	for _, tc := range []struct {
		err  error
		want int
	}{
		{service.ErrNoInvitesRemaining, 403},
		{service.ErrInviteNotFound, 404},
		{service.ErrInviteExpired, 410},
		{service.ErrInviteRedeemed, 409},
		{errors.New("database exploded"), 500},
	} {
		assertStatus(t, "handleInviteError", m, tc.err, tc.want)
	}
}

func TestHandleAuthErrorMapping(t *testing.T) {
	m := func(w *httptest.ResponseRecorder, err error) { handleAuthError(w, err) }

	for _, tc := range []struct {
		err  error
		want int
	}{
		{service.ErrInvalidCredentials, 401},
		{service.ErrUsernameTaken, 409},
		{service.ErrEmailTaken, 409},
		{service.ErrInvalidToken, 401},
		{service.ErrInvalidResetToken, 400},
		{service.ErrValidationFailed, 422},
		{service.ErrInviteRequired, 403},
		{service.ErrInvalidInviteCode, 400},
		{service.ErrBannedEmail, 403},
		{service.ErrBannedIP, 403},
		{errors.New("database exploded"), 500},
	} {
		assertStatus(t, "handleAuthError", m, tc.err, tc.want)
	}
}

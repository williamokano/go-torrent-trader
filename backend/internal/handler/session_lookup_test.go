package handler_test

import (
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// sessionOf unwraps a session lookup for tests that only care whether a
// session came back: a store error is a broken fixture, not an answer.
func sessionOf(sess *service.Session, err error) *service.Session {
	if err != nil {
		panic(fmt.Sprintf("session lookup failed: %v", err))
	}
	return sess
}

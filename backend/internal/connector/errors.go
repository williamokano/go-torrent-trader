package connector

import "errors"

// ErrNotReady means a connector cannot deliver right now for a reason that is
// nobody's fault and will probably resolve itself: an IRC client still
// connecting, or a node that does not currently own the connection.
//
// The pipeline treats it as neither success nor failure — the delivery is
// rescheduled shortly without consuming one of its five attempts, so a routine
// reconnect cannot dead-letter a queue of announcements. Deliveries do not wait
// forever, though: past a bounded age they are failed like anything else.
//
// It lives here rather than in the IRC package so the drain handler stays
// generic; any connector with a warm-up or ownership model can use it.
var ErrNotReady = errors.New("connector is not ready to deliver")

// ErrPermanent means a delivery will never succeed however often it is retried:
// a chat that no longer exists, a revoked credential, a message the destination
// rejects as malformed.
//
// It exists because the alternative is worse than losing the announcement. A
// connector that fans out to several destinations fails the whole delivery when
// one of them fails, so a permanently-broken destination would otherwise make
// every healthy one receive the same announcement five times, for every torrent,
// forever. Dead-lettering immediately puts the failure in the admin log where it
// can be fixed, and leaves the healthy destinations alone.
var ErrPermanent = errors.New("connector delivery cannot succeed")

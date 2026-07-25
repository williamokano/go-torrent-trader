package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Server-sent events are useless without Flush: each frame has to reach the
// client as it is produced. Embedding http.ResponseWriter does not provide it —
// the embedded interface has no Flush — so without an explicit method every
// streaming handler behind this middleware silently buffers.
func TestRequestLoggerPreservesFlusher(t *testing.T) {
	var sawFlusher bool

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		sawFlusher = ok
		if ok {
			_, _ = w.Write([]byte("frame"))
			flusher.Flush()
		}
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))

	if !sawFlusher {
		t.Fatal("handlers behind the request logger must still see an http.Flusher")
	}
	if !rec.Flushed {
		t.Fatal("Flush did not reach the underlying writer")
	}
}

// The WebSocket upgrade path depends on this and has no test of its own.
func TestRequestLoggerPreservesHijackerThroughUnwrap(t *testing.T) {
	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Hijacker); !ok {
			t.Error("handlers behind the request logger must still see an http.Hijacker")
		}
		type unwrapper interface{ Unwrap() http.ResponseWriter }
		if _, ok := w.(unwrapper); !ok {
			t.Error("the wrapper chain must stay walkable")
		}
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws", nil))
}

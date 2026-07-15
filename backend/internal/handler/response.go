package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// JSON writes a JSON response with the given status code and data. It encodes
// into a buffer before writing the status line, so that an un-encodable value
// (e.g. a +Inf/NaN float) fails with a clean 500 rather than a half-written,
// empty 200 — the latter is indistinguishable from success to clients and
// silently breaks them (it was behind a spurious logout on zero-download
// accounts, whose ratio used to be +Inf).
func JSON(w http.ResponseWriter, status int, data interface{}) {
	body, err := json.Marshal(data)
	if err != nil {
		slog.Error("failed to encode JSON response", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"failed to encode response"}}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// ErrorResponse writes a structured JSON error response.
func ErrorResponse(w http.ResponseWriter, status int, code string, message string) {
	JSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

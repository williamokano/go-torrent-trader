package handler

import "net/http"

// HandleHealthz responds with a JSON health check status.
func HandleHealthz(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

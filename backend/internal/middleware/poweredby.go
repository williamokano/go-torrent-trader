package middleware

import "net/http"

// PoweredBy sets X-Powered-By: gott on every response.
func PoweredBy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Powered-By", "gott")
		next.ServeHTTP(w, r)
	})
}

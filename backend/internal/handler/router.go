package handler

import (
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
)

// NewRouter creates and configures the Chi router with middleware and routes.
func NewRouter() chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger)
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS)

	// Health check
	r.Get("/healthz", HandleHealthz)

	// API routes (placeholder group)
	r.Route("/api/v1", func(r chi.Router) {
		// Will be populated by feature handlers
	})

	return r
}

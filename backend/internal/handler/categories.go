package handler

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/metadata"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// HandleCategories returns the list of categories (public endpoint).
func HandleCategories(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT id, name, parent_id, image_url, sort_order FROM categories ORDER BY sort_order, name`,
		)
		if err != nil {
			slog.Error("failed to query categories", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load categories")
			return
		}
		defer func() {
			if cerr := rows.Close(); cerr != nil {
				slog.Error("failed to close categories rows", "error", cerr)
			}
		}()

		type category struct {
			ID        int64   `json:"id"`
			Name      string  `json:"name"`
			ParentID  *int64  `json:"parent_id"`
			ImageURL  *string `json:"image_url"`
			SortOrder int     `json:"sort_order"`
		}

		var categories []category
		for rows.Next() {
			var c category
			if err := rows.Scan(&c.ID, &c.Name, &c.ParentID, &c.ImageURL, &c.SortOrder); err != nil {
				slog.Error("failed to scan category", "error", err)
				ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load categories")
				return
			}
			categories = append(categories, c)
		}
		if err := rows.Err(); err != nil {
			slog.Error("failed to iterate categories", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load categories")
			return
		}

		if categories == nil {
			categories = []category{}
		}

		JSON(w, http.StatusOK, map[string]interface{}{"categories": categories})
	}
}

// HandleCategoryMetadataSchema returns a category's effective (inherited) metadata
// schema, which upload/edit forms use to render dynamic fields. Public endpoint.
func HandleCategoryMetadataSchema(catSvc *service.CategoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil || id <= 0 {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid category ID")
			return
		}

		fields, err := catSvc.ResolveSchema(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrCategoryNotFound) {
				ErrorResponse(w, http.StatusNotFound, "not_found", "category not found")
				return
			}
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to resolve metadata schema")
			return
		}

		// Never emit a null array — the frontend expects a list to iterate.
		if fields == nil {
			fields = []metadata.FieldDef{}
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"category_id": id,
			"fields":      fields,
		})
	}
}

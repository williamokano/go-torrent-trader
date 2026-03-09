package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
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

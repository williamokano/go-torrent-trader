package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const categoriesQuery = "SELECT id, name, parent_id, image_url, sort_order FROM categories"

func TestHandleCategoriesReturnsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("creating sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	parent := int64(1)
	image := "movies.png"
	mock.ExpectQuery(categoriesQuery).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "image_url", "sort_order"}).
			AddRow(int64(1), "Movies", nil, nil, 0).
			AddRow(int64(2), "HD", parent, image, 1))

	w := httptest.NewRecorder()
	HandleCategories(db)(w, httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	items, ok := decodeBody(t, w)["categories"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("categories = %v, want 2 items", items)
	}

	root := items[0].(map[string]interface{})
	if root["name"] != "Movies" || root["parent_id"] != nil || root["image_url"] != nil {
		t.Errorf("root category = %v, want Movies with null parent_id/image_url", root)
	}
	child := items[1].(map[string]interface{})
	if child["parent_id"] != float64(1) || child["image_url"] != "movies.png" || child["sort_order"] != float64(1) {
		t.Errorf("child category = %v, want parent_id=1 image_url=movies.png sort_order=1", child)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// An empty table must serialise as [] so the frontend can iterate unconditionally.
func TestHandleCategoriesReturnsEmptyArrayNotNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("creating sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(categoriesQuery).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "image_url", "sort_order"}))

	w := httptest.NewRecorder()
	HandleCategories(db)(w, httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, ok := decodeBody(t, w)["categories"].([]interface{})
	if !ok || len(items) != 0 {
		t.Errorf("categories = %v, want an empty array", items)
	}
}

func TestHandleCategoriesSurfacesQueryFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("creating sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(categoriesQuery).WillReturnError(errors.New("connection refused"))

	w := httptest.NewRecorder()
	HandleCategories(db)(w, httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// A row whose columns don't match the destination types must fail loudly rather
// than return a truncated list.
func TestHandleCategoriesSurfacesScanFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("creating sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(categoriesQuery).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "image_url", "sort_order"}).
			AddRow("not-an-id", "Movies", nil, nil, 0))

	w := httptest.NewRecorder()
	HandleCategories(db)(w, httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when a row cannot be scanned", w.Code)
	}
}

func TestHandleCategoriesSurfacesRowIterationFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("creating sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(categoriesQuery).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "parent_id", "image_url", "sort_order"}).
			AddRow(int64(1), "Movies", nil, nil, 0).
			RowError(0, errors.New("connection lost mid-scan")))

	w := httptest.NewRecorder()
	HandleCategories(db)(w, httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when iteration fails partway", w.Code)
	}
}

package server

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestParsePaginationDefaults(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	page, err := parsePagination(c)
	if err != nil {
		t.Fatalf("parsePagination returned error: %v", err)
	}
	if page.Limit != defaultLimit || page.Offset != 0 {
		t.Fatalf("unexpected pagination: %+v", page)
	}
}

func TestParsePaginationInvalidValues(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/?limit=-1&offset=-10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if _, err := parsePagination(c); err == nil {
		t.Fatalf("expected error for invalid pagination")
	}
}

func TestParsePaginationMaxLimit(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/?limit=999&offset=5", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	page, err := parsePagination(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Limit != maxLimit || page.Offset != 5 {
		t.Fatalf("unexpected pagination result: %+v", page)
	}
}

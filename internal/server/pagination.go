package server

import (
	"strconv"

	"github.com/labstack/echo/v4"
)

const (
	defaultLimit = 50
	maxLimit     = 500
)

// Pagination captures cursor settings for list endpoints.
type Pagination struct {
	Limit  int
	Offset int
}

// parsePagination extracts limit/offset from query params while enforcing sane defaults.
func parsePagination(c echo.Context) (Pagination, error) {
	limit := defaultLimit
	offset := 0
	if l := c.QueryParam("limit"); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil || v <= 0 {
			return Pagination{}, echo.NewHTTPError(400, "limit must be a positive integer")
		}
		if v > maxLimit {
			v = maxLimit
		}
		limit = v
	}
	if o := c.QueryParam("offset"); o != "" {
		v, err := strconv.Atoi(o)
		if err != nil || v < 0 {
			return Pagination{}, echo.NewHTTPError(400, "offset must be >= 0")
		}
		offset = v
	}
	return Pagination{Limit: limit, Offset: offset}, nil
}

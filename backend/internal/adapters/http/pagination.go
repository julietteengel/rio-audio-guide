package http

import (
	"fmt"
	"strconv"

	"github.com/labstack/echo/v4"
)

// List routes stay plain JSON arrays (the mobile app and web client parse
// them as such), so pagination is purely opt-in via ?limit= and ?offset=, and
// every route has a default cap -- an unbounded list is what grows into a slow
// or oversized response as the data does. A client pages until it receives
// fewer items than it asked for.
const (
	maxPlacesPageSize     = 500
	maxItinerariesPerPage = 100
)

type pageParams struct{ limit, offset int }

// parsePage reads ?limit= (1..maxLimit, default maxLimit) and ?offset= (>= 0).
func parsePage(c echo.Context, maxLimit int) (pageParams, error) {
	p := pageParams{limit: maxLimit}
	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxLimit {
			return pageParams{}, fmt.Errorf("limit must be an integer between 1 and %d", maxLimit)
		}
		p.limit = n
	}
	if raw := c.QueryParam("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return pageParams{}, fmt.Errorf("offset must be a non-negative integer")
		}
		p.offset = n
	}
	return p, nil
}

func paginate[T any](items []T, p pageParams) []T {
	if p.offset >= len(items) {
		return items[:0] // non-nil, so it encodes as [] and not null
	}
	end := p.offset + p.limit
	if end > len(items) {
		end = len(items)
	}
	return items[p.offset:end]
}

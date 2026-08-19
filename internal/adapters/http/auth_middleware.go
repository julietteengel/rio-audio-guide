package http

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/ports"
)

// contextUserIDKey/contextRoleKey: unexported keys so no other package can
// accidentally set (or misread) these -- only requireAuth ever writes them,
// only handlers in this package ever read them.
const (
	contextUserIDKey = "auth.userID"
	contextRoleKey   = "auth.role"
)

// requireAuth reads "Authorization: Bearer <token>", verifies it against
// tokens (ports.TokenIssuer), and stores the resulting user ID/role on the
// request context for downstream handlers -- e.g. updateMe/deleteMe use
// contextUserID(c) instead of trusting a client-supplied user ID in the
// URL, so a user can only ever edit their own account via this route shape.
func requireAuth(tokens ports.TokenIssuer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				return c.JSON(http.StatusUnauthorized, echo.Map{"error": "missing or malformed Authorization header"})
			}

			userID, role, err := tokens.Verify(token)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, echo.Map{"error": "invalid or expired token"})
			}

			c.Set(contextUserIDKey, userID)
			c.Set(contextRoleKey, role)
			return next(c)
		}
	}
}

func contextUserID(c echo.Context) string {
	id, _ := c.Get(contextUserIDKey).(string)
	return id
}

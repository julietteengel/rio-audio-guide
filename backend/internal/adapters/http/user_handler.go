package http

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/domain"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// registerUser always creates a RoleUser account -- self-registration can
// never grant admin, that would need a separate, already-authenticated,
// admin-only route (not built yet: nothing in this app needs it today).
func (s *Server) registerUser(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	user, err := application.RegisterUser(c.Request().Context(), s.userRepo, req.Email, req.Password, domain.RoleUser)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, userResponse{ID: user.ID(), Email: user.Email().String(), Role: user.Role().String()})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

func (s *Server) login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	token, err := application.LoginUser(c.Request().Context(), s.userRepo, s.tokens, req.Email, req.Password)
	if err != nil {
		// Toujours 401 générique ici, jamais de détail sur "email inconnu" vs
		// "mot de passe faux" -- ApplicationErrInvalidCredentials existe
		// justement pour ne jamais faire cette distinction.
		if errors.Is(err, application.ErrInvalidCredentials) {
			return c.JSON(http.StatusUnauthorized, echo.Map{"error": "invalid email or password"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, loginResponse{Token: token})
}

// logout: avec du JWT stateless, il n'y a rien à invalider côté serveur --
// aucune session stockée nulle part. La vraie déconnexion, c'est le client
// qui jette son token. Cette route existe pour donner un endpoint cohérent
// au frontend (et un futur point d'ancrage si une blocklist de tokens
// devient un jour nécessaire), pas pour faire un vrai travail aujourd'hui.
func (s *Server) logout(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

type updateMeRequest struct {
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

// updateMe modifie le compte de l'appelant, jamais un autre -- l'ID vient du
// token vérifié par requireAuth (contextUserID), pas d'un paramètre d'URL
// qu'un client pourrait falsifier pour éditer le compte de quelqu'un d'autre.
func (s *Server) updateMe(c echo.Context) error {
	var req updateMeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	user, err := application.UpdateUserProfile(c.Request().Context(), s.userRepo, contextUserID(c), req.Email, req.Password)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, userResponse{ID: user.ID(), Email: user.Email().String(), Role: user.Role().String()})
}

func (s *Server) deleteMe(c echo.Context) error {
	if err := application.DeleteUser(c.Request().Context(), s.userRepo, contextUserID(c)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, echo.Map{"error": "user not found"})
		}
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

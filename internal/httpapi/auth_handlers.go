package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kitchu-studio/order-demo-backend/internal/auth"
	"github.com/kitchu-studio/order-demo-backend/internal/store"
)

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Name     string `json:"name" binding:"max=100"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int64       `json:"expires_in"`
	User         *store.User `json:"user,omitempty"`
}

func (s *Server) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid request: email and password (8-72 chars) are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("hash password", "error", err)
		abort(c, http.StatusInternalServerError, "internal error")
		return
	}

	email := normalizeEmail(req.Email)
	user, err := s.store.CreateUser(c.Request.Context(), email, strings.TrimSpace(req.Name), hash, s.isAdminEmail(email))
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			abort(c, http.StatusConflict, "email already registered")
			return
		}
		slog.Error("create user", "error", err)
		abort(c, http.StatusInternalServerError, "internal error")
		return
	}

	s.respondWithTokens(c, http.StatusCreated, user)
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid request")
		return
	}

	user, err := s.store.GetUserByEmail(c.Request.Context(), normalizeEmail(req.Email))
	if err != nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
		// Same message for unknown email and wrong password to avoid leaking which emails exist.
		abort(c, http.StatusUnauthorized, "incorrect email or password")
		return
	}

	s.respondWithTokens(c, http.StatusOK, user)
}

func (s *Server) refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid request")
		return
	}

	userID, err := s.tokens.Parse(req.RefreshToken, auth.TypeRefresh)
	if err != nil {
		abort(c, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	user, err := s.store.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		abort(c, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	s.respondWithTokens(c, http.StatusOK, user)
}

func (s *Server) me(c *gin.Context) {
	userID := c.GetInt64(ctxUserID)
	user, err := s.store.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		abort(c, http.StatusNotFound, "user not found")
		return
	}
	c.JSON(http.StatusOK, user)
}

func (s *Server) respondWithTokens(c *gin.Context, status int, user *store.User) {
	access, err := s.tokens.NewAccess(user.ID)
	if err != nil {
		slog.Error("sign access token", "error", err)
		abort(c, http.StatusInternalServerError, "internal error")
		return
	}
	refresh, err := s.tokens.NewRefresh(user.ID)
	if err != nil {
		slog.Error("sign refresh token", "error", err)
		abort(c, http.StatusInternalServerError, "internal error")
		return
	}
	c.JSON(status, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.tokens.AccessTTL().Seconds()),
		User:         user,
	})
}

func normalizeEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}

func (s *Server) isAdminEmail(email string) bool {
	for _, a := range s.adminEmails {
		if a == email {
			return true
		}
	}
	return false
}

package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kitchu-studio/order-demo-backend/internal/auth"
)

const ctxUserID = "userID"

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}

func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			abort(c, http.StatusUnauthorized, "missing bearer token")
			return
		}
		userID, err := s.tokens.Parse(token, auth.TypeAccess)
		if err != nil {
			abort(c, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		c.Set(ctxUserID, userID)
		c.Next()
	}
}

func abort(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// requireAdmin must run after requireAuth.
func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := s.store.GetUserByID(c.Request.Context(), c.GetInt64(ctxUserID))
		if err != nil || !user.IsAdmin {
			abort(c, http.StatusForbidden, "admin only")
			return
		}
		c.Next()
	}
}

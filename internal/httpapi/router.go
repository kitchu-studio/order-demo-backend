package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kitchu-studio/order-demo-backend/internal/auth"
	"github.com/kitchu-studio/order-demo-backend/internal/store"
)

type Server struct {
	store       *store.Store
	tokens      *auth.TokenManager
	shopName    string
	promptPayID string
	adminEmails []string
	uploadDir   string
}

type Options struct {
	AppEnv      string
	ShopName    string
	PromptPayID string
	AdminEmails []string
	UploadDir   string
}

func NewRouter(st *store.Store, tokens *auth.TokenManager, opt Options) *gin.Engine {
	if opt.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	s := &Server{
		store:       st,
		tokens:      tokens,
		shopName:    opt.ShopName,
		promptPayID: opt.PromptPayID,
		adminEmails: opt.AdminEmails,
		uploadDir:   opt.UploadDir,
	}

	r := gin.New()
	r.Use(gin.Recovery(), requestLogger())
	r.MaxMultipartMemory = 8 << 20

	// Product photos are public; the directory itself is not listable.
	r.Static("/media", opt.UploadDir)

	r.GET("/healthz", s.health)

	v1 := r.Group("/api/v1")
	{
		a := v1.Group("/auth")
		a.POST("/register", s.register)
		a.POST("/login", s.login)
		a.POST("/refresh", s.refresh)

		// Public: anyone can see the shop and its menu.
		v1.GET("/shop", s.shopInfo)
		v1.GET("/products", s.listProducts)

		authed := v1.Group("")
		authed.Use(s.requireAuth())
		authed.GET("/me", s.me)
		authed.POST("/orders", s.createOrder)
		authed.GET("/orders", s.listMyOrders)
		authed.GET("/orders/:id", s.getMyOrder)
		authed.POST("/orders/:id/cancel", s.cancelMyOrder)

		admin := authed.Group("/admin")
		admin.Use(s.requireAdmin())
		admin.GET("/orders", s.adminListOrders)
		admin.POST("/orders/:id/status", s.adminSetStatus)
		admin.PATCH("/products/:id", s.adminSetProduct)
		admin.POST("/products/:id/image", s.adminSetProductImage)
	}
	return r
}

func (s *Server) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

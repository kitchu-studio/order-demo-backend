package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kitchu-studio/order-demo-backend/internal/auth"
	"github.com/kitchu-studio/order-demo-backend/internal/config"
	"github.com/kitchu-studio/order-demo-backend/internal/httpapi"
	"github.com/kitchu-studio/order-demo-backend/internal/promptpay"
	"github.com/kitchu-studio/order-demo-backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot connect to database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if _, err := promptpay.Payload(cfg.PromptPayID, 1); err != nil {
		logger.Error("invalid PROMPTPAY_ID", "error", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		logger.Error("cannot create upload dir", "dir", cfg.UploadDir, "error", err)
		os.Exit(1)
	}

	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	router := httpapi.NewRouter(st, tokens, httpapi.Options{
		AppEnv:      cfg.AppEnv,
		ShopName:    cfg.ShopName,
		PromptPayID: cfg.PromptPayID,
		AdminEmails: cfg.AdminEmails,
		UploadDir:   cfg.UploadDir,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("server started", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

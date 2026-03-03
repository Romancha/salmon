package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/romancha/bear-sync/internal/api"
	"github.com/romancha/bear-sync/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

type config struct {
	host           string
	port           string
	dbPath         string
	openclawToken  string
	bridgeToken    string
	attachmentsDir string
}

func loadConfig() (*config, error) {
	cfg := &config{
		host:           os.Getenv("HUB_HOST"),
		port:           os.Getenv("HUB_PORT"),
		dbPath:         os.Getenv("HUB_DB_PATH"),
		openclawToken:  os.Getenv("HUB_OPENCLAW_TOKEN"),
		bridgeToken:    os.Getenv("HUB_BRIDGE_TOKEN"),
		attachmentsDir: os.Getenv("HUB_ATTACHMENTS_DIR"),
	}

	if cfg.host == "" {
		cfg.host = "127.0.0.1"
	}

	if cfg.port == "" {
		cfg.port = "8080"
	}

	if cfg.dbPath == "" {
		return nil, fmt.Errorf("HUB_DB_PATH is required")
	}

	if cfg.openclawToken == "" {
		return nil, fmt.Errorf("HUB_OPENCLAW_TOKEN is required")
	}

	if cfg.bridgeToken == "" {
		return nil, fmt.Errorf("HUB_BRIDGE_TOKEN is required")
	}

	if cfg.attachmentsDir == "" {
		cfg.attachmentsDir = "attachments"
	}

	return cfg, nil
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(logHandler))

	s, err := store.NewSQLiteStore(cfg.dbPath)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	defer func() {
		if closeErr := s.Close(); closeErr != nil {
			slog.Error("failed to close store", "error", closeErr)
		}
	}()

	consumerTokens := map[string]string{"openclaw": cfg.openclawToken}
	srv := api.NewServer(s, consumerTokens, cfg.bridgeToken, cfg.attachmentsDir)

	addr := net.JoinHostPort(cfg.host, cfg.port)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		slog.Info("starting hub server", "addr", addr)

		if listenErr := httpServer.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen: %w", listenErr)
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err = <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err = httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	slog.Info("hub server stopped")

	return nil
}

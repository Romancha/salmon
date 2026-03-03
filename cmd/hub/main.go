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
	"sort"
	"strings"
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
	consumerTokens map[string]string
	bridgeToken    string
	attachmentsDir string
}

func loadConfig() (*config, error) {
	cfg := &config{
		host:           os.Getenv("HUB_HOST"),
		port:           os.Getenv("HUB_PORT"),
		dbPath:         os.Getenv("HUB_DB_PATH"),
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

	rawTokens := os.Getenv("HUB_CONSUMER_TOKENS")
	if rawTokens == "" {
		return nil, fmt.Errorf("HUB_CONSUMER_TOKENS is required")
	}

	consumerTokens, err := api.ParseConsumerTokens(rawTokens)
	if err != nil {
		return nil, fmt.Errorf("parse HUB_CONSUMER_TOKENS: %w", err)
	}

	cfg.consumerTokens = consumerTokens

	if cfg.bridgeToken == "" {
		return nil, fmt.Errorf("HUB_BRIDGE_TOKEN is required")
	}

	for name, token := range cfg.consumerTokens {
		if token == cfg.bridgeToken {
			return nil, fmt.Errorf("consumer %q token must not equal HUB_BRIDGE_TOKEN", name)
		}
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

	srv := api.NewServer(s, cfg.consumerTokens, cfg.bridgeToken, cfg.attachmentsDir)

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

	consumerNames := make([]string, 0, len(cfg.consumerTokens))
	for name := range cfg.consumerTokens {
		consumerNames = append(consumerNames, name)
	}
	sort.Strings(consumerNames)
	slog.Info("registered consumers", "consumers", strings.Join(consumerNames, ", "))

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

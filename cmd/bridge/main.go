package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/romancha/bear-sync/internal/beardb"
	"github.com/romancha/bear-sync/internal/hubclient"
	"github.com/romancha/bear-sync/internal/xcallback"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("bridge failed", "error", err)
		os.Exit(1)
	}
}

type config struct {
	hubURL    string
	hubToken  string
	bearToken string
	statePath string
	bearDBDir string
}

func loadConfig() (*config, error) {
	cfg := &config{
		hubURL:    os.Getenv("BRIDGE_HUB_URL"),
		hubToken:  os.Getenv("BRIDGE_HUB_TOKEN"),
		bearToken: os.Getenv("BEAR_TOKEN"),
		statePath: os.Getenv("BRIDGE_STATE_PATH"),
		bearDBDir: os.Getenv("BEAR_DB_DIR"),
	}

	if cfg.hubURL == "" {
		return nil, fmt.Errorf("BRIDGE_HUB_URL is required")
	}

	if cfg.hubToken == "" {
		return nil, fmt.Errorf("BRIDGE_HUB_TOKEN is required")
	}

	if cfg.bearToken == "" {
		return nil, fmt.Errorf("BEAR_TOKEN is required")
	}

	if cfg.statePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cfg.statePath = home + "/.bear-bridge-state.json"
	}

	if cfg.bearDBDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cfg.bearDBDir = home + "/Library/Group Containers/9K33E3U3T4.net.shinyfrog.bear/Application Data"
	}

	return cfg, nil
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Flock protection against parallel runs.
	lockPath := lockFilePath()
	lockFile, err := acquireLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer releaseLock(lockFile, logger)

	logger.Info("bridge starting",
		"hub_url", cfg.hubURL,
		"state_path", cfg.statePath,
		"bear_db_dir", cfg.bearDBDir)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	db, err := beardb.New(cfg.bearDBDir + "/database.sqlite")
	if err != nil {
		return fmt.Errorf("open bear db: %w", err)
	}
	defer db.Close() //nolint:errcheck // best-effort close

	hub := hubclient.NewHTTPClient(cfg.hubURL, cfg.hubToken, logger)

	// Initialize xcallback for write queue processing.
	// If xcall is not available, queue processing will be skipped.
	var xcall xcallback.XCallback
	xc, err := xcallback.New(xcallback.WithLogger(logger))
	if err != nil {
		logger.Warn("xcall not available, write queue processing disabled", "error", err)
	} else {
		xcall = xc
	}

	bridge := NewBridge(db, hub, xcall, cfg.bearToken, cfg.statePath, cfg.bearDBDir, logger)

	if err := bridge.Run(ctx); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	logger.Info("bridge completed successfully")
	return nil
}

// lockFilePath returns the path for the flock file.
func lockFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/.bear-bridge.lock"
	}
	return home + "/.bear-bridge.lock"
}

// acquireLock attempts to acquire an exclusive non-blocking file lock.
func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lock file path from trusted config
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // fd fits int on darwin
		f.Close() //nolint:errcheck,gosec // closing on lock failure
		return nil, fmt.Errorf("another bridge instance is running (lock file: %s)", path)
	}

	return f, nil
}

// releaseLock releases the file lock and removes the lock file.
func releaseLock(f *os.File, logger *slog.Logger) {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil { //nolint:gosec // fd fits int on darwin
		logger.Warn("failed to unlock", "error", err)
	}

	name := f.Name()
	if err := f.Close(); err != nil {
		logger.Warn("failed to close lock file", "error", err)
	}

	if err := os.Remove(name); err != nil { //nolint:gosec // lock file path from trusted config
		logger.Warn("failed to remove lock file", "error", err)
	}
}

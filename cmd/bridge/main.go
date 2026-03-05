package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/romancha/salmon/internal/beardb"
	"github.com/romancha/salmon/internal/hubclient"
	"github.com/romancha/salmon/internal/xcallback"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	daemonMode := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--version":
			fmt.Println("salmon-run " + version)
			return
		case "--daemon":
			daemonMode = true
		}
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger, daemonMode); err != nil {
		logger.Error("bridge failed", "error", err)
		os.Exit(1)
	}
}

// defaultSyncInterval is the default interval between sync cycles in daemon mode.
const defaultSyncInterval = 300 * time.Second

type config struct {
	hubURL       string
	hubToken     string
	bearToken    string
	statePath    string
	bearDBDir    string
	syncInterval time.Duration
	ipcSocket    string
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

	if v := os.Getenv("BRIDGE_IPC_SOCKET"); v != "" {
		cfg.ipcSocket = v
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cfg.ipcSocket = home + "/.bear-bridge.sock"
	}

	cfg.syncInterval = defaultSyncInterval
	if v := os.Getenv("BRIDGE_SYNC_INTERVAL"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 1 {
			return nil, fmt.Errorf("BRIDGE_SYNC_INTERVAL must be a positive integer (seconds), got %q", v)
		}
		cfg.syncInterval = time.Duration(secs) * time.Second
	}

	return cfg, nil
}

func run(logger *slog.Logger, daemonMode bool) error {
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
		"bear_db_dir", cfg.bearDBDir,
		"daemon", daemonMode)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	db, err := beardb.New(cfg.bearDBDir + "/database.sqlite")
	if err != nil {
		return fmt.Errorf("open bear db: %w", err)
	}
	defer db.Close() //nolint:errcheck // best-effort close

	hub := hubclient.NewHTTPClient(cfg.hubURL, cfg.hubToken, logger)

	// Initialize xcallback for write queue processing.
	// If bear-xcall is not available, queue processing will be skipped.
	var xcall xcallback.XCallback
	xc, err := xcallback.New(xcallback.WithLogger(logger))
	if err != nil {
		logger.Warn("bear-xcall not available, write queue processing disabled", "error", err)
	} else {
		xcall = xc
	}

	bridge := NewBridge(db, hub, xcall, cfg.bearToken, cfg.statePath, cfg.bearDBDir, logger)

	if daemonMode {
		bridge.events = NewEventEmitter(os.Stdout)
		logger.Info("entering daemon mode", "sync_interval", cfg.syncInterval, "ipc_socket", cfg.ipcSocket)
		return runDaemonWithIPC(ctx, bridge, cfg.syncInterval, cfg.ipcSocket, logger)
	}

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

// releaseLock releases the file lock. The lock file is intentionally kept on disk
// to avoid a TOCTOU race between concurrent bridge instances. The OS automatically
// releases the flock when the process exits.
func releaseLock(f *os.File, logger *slog.Logger) {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil { //nolint:gosec // fd fits int on darwin
		logger.Warn("failed to unlock", "error", err)
	}

	if err := f.Close(); err != nil {
		logger.Warn("failed to close lock file", "error", err)
	}
}

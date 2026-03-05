package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/romancha/bear-sync/internal/ipc"
)

// runDaemon runs the bridge sync loop continuously until the context is cancelled.
// Errors in individual sync cycles are logged but do not stop the daemon.
func runDaemon(ctx context.Context, bridge *Bridge, interval time.Duration, logger *slog.Logger) error {
	return runDaemonWithIPC(ctx, bridge, interval, "", logger)
}

// runDaemonWithIPC runs the daemon loop with an optional IPC server.
// If socketPath is non-empty, an IPC server is started on that Unix socket.
func runDaemonWithIPC(
	ctx context.Context, bridge *Bridge, interval time.Duration, socketPath string, logger *slog.Logger,
) error {
	stats := ipc.NewStatsTracker(0)
	bridge.stats = stats

	// Wrap the logger to feed log entries into the stats tracker for IPC logs command.
	logger = slog.New(&statsLogHandler{Handler: logger.Handler(), stats: stats})

	if socketPath != "" {
		srv := ipc.NewServer(socketPath, stats, logger)
		if err := srv.Start(ctx); err != nil {
			return fmt.Errorf("start ipc server: %w", err)
		}
		defer srv.Stop() //nolint:errcheck,gosec // best-effort stop on daemon exit
	}

	// Run the first sync immediately.
	logger.Info("daemon: running initial sync")
	runSyncCycle(ctx, bridge, stats, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("daemon: shutting down")
			return nil
		case <-ticker.C:
			logger.Info("daemon: starting sync cycle")
			runSyncCycle(ctx, bridge, stats, logger)
		case <-stats.SyncTriggered():
			logger.Info("daemon: sync triggered via IPC")
			runSyncCycle(ctx, bridge, stats, logger)
			// Reset ticker so next tick is a full interval from now.
			ticker.Reset(interval)
		case <-stats.ShutdownRequested():
			logger.Info("daemon: shutdown requested via IPC")
			return nil
		}
	}
}

// statsLogHandler wraps a slog.Handler and feeds log records into StatsTracker.
type statsLogHandler struct {
	slog.Handler
	stats *ipc.StatsTracker
}

//nolint:gocritic // slog.Handler interface requires slog.Record by value
func (h *statsLogHandler) Handle(ctx context.Context, r slog.Record) error {
	h.stats.AddLog(ipc.LogEntry{
		Time:  r.Time.UTC().Format(time.RFC3339),
		Level: r.Level.String(),
		Msg:   r.Message,
	})
	if err := h.Handler.Handle(ctx, r); err != nil {
		return fmt.Errorf("stats log handler: %w", err)
	}
	return nil
}

func (h *statsLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &statsLogHandler{Handler: h.Handler.WithAttrs(attrs), stats: h.stats}
}

func (h *statsLogHandler) WithGroup(name string) slog.Handler {
	return &statsLogHandler{Handler: h.Handler.WithGroup(name), stats: h.stats}
}

// runSyncCycle executes a single sync and updates stats accordingly.
func runSyncCycle(ctx context.Context, bridge *Bridge, stats *ipc.StatsTracker, logger *slog.Logger) {
	stats.SetSyncing()
	start := time.Now()

	if err := bridge.Run(ctx); err != nil {
		durationMs := time.Since(start).Milliseconds()
		stats.SetError(err.Error())
		stats.RecordSync(0, 0, 0, durationMs)
		logger.Error("daemon: sync cycle failed", "error", err)
	} else {
		durationMs := time.Since(start).Milliseconds()
		stats.SetIdle()
		stats.RecordSync(bridge.cycleNotes, bridge.cycleTags, bridge.cycleQueue, durationMs)
		logger.Info("daemon: sync cycle completed", "duration_ms", durationMs)
	}
	stats.ClearQueueItems()
}

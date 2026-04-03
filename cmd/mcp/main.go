package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/romancha/salmon/internal/mcp"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("salmon-mcp " + version)
		return
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

type config struct {
	hubURL string
	token  string
}

func loadConfig() (*config, error) {
	cfg := &config{
		hubURL: os.Getenv("SALMON_HUB_URL"),
		token:  os.Getenv("SALMON_CONSUMER_TOKEN"),
	}

	if cfg.hubURL == "" {
		return nil, fmt.Errorf("SALMON_HUB_URL is required")
	}

	if cfg.token == "" {
		return nil, fmt.Errorf("SALMON_CONSUMER_TOKEN is required")
	}

	return cfg, nil
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client := mcp.NewClient(cfg.hubURL, cfg.token)

	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "salmon-mcp",
		Version: version,
	}, &gomcp.ServerOptions{
		Instructions: serverInstructions(),
	})

	mcp.RegisterTools(server, client)

	slog.Info("starting salmon MCP server", "hub_url", cfg.hubURL) //nolint:gosec // G706: hub_url is from trusted env var, not user input

	transport := &gomcp.StdioTransport{}

	if err := server.Run(context.Background(), transport); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}

	return nil
}

func serverInstructions() string {
	return "Salmon provides access to the user's Bear notes. " +
		"Use these tools when the user asks about their notes, " +
		"wants to search, read, create, or edit notes in Bear. " +
		"Key behaviors: " +
		"1) Note bodies are Markdown. Titles are plain text. " +
		"2) list_notes omits body; use search_notes or get_note " +
		"to read content. " +
		"3) Writes are async: queued and applied to Bear by bridge. " +
		"sync_status: synced → pending_to_bear → synced. " +
		"4) Encrypted notes are read-only (403 on writes). " +
		"5) Notes with sync_status conflict/pending_to_bear " +
		"reject writes (409). " +
		"6) When creating notes, do NOT put #tags in the body " +
		"if you also pass them in the tags parameter — " +
		"Bear adds them automatically, duplicating causes double tags."
}

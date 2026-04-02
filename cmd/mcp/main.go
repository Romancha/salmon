package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/romancha/salmon/internal/mcp"
)

func main() {
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
		Version: "1.0.0",
	}, nil)

	mcp.RegisterTools(server, client)

	slog.Info("starting salmon MCP server", "hub_url", cfg.hubURL)

	transport := &gomcp.StdioTransport{}

	if err := server.Run(context.Background(), transport); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}

	return nil
}

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_MissingHubURL(t *testing.T) {
	t.Setenv("SALMON_HUB_URL", "")
	t.Setenv("SALMON_CONSUMER_TOKEN", "test-token")

	cfg, err := loadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "SALMON_HUB_URL is required")
}

func TestLoadConfig_MissingToken(t *testing.T) {
	t.Setenv("SALMON_HUB_URL", "https://example.com")
	t.Setenv("SALMON_CONSUMER_TOKEN", "")

	cfg, err := loadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "SALMON_CONSUMER_TOKEN is required")
}

func TestLoadConfig_Valid(t *testing.T) {
	t.Setenv("SALMON_HUB_URL", "https://example.com")
	t.Setenv("SALMON_CONSUMER_TOKEN", "test-token")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", cfg.hubURL)
	assert.Equal(t, "test-token", cfg.token)
}

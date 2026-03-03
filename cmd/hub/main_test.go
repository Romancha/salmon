package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_MissingRequired(t *testing.T) {
	for _, key := range []string{"HUB_DB_PATH", "HUB_CONSUMER_TOKENS", "HUB_BRIDGE_TOKEN"} {
		t.Run(key, func(t *testing.T) {
			required := map[string]string{ //nolint:gosec // test credentials
				"HUB_DB_PATH":         "/tmp/test.db",
				"HUB_CONSUMER_TOKENS": "app1:oc-test",
				"HUB_BRIDGE_TOKEN":    "br-test",
			}
			for k, v := range required {
				if k != key {
					t.Setenv(k, v)
				}
			}
			_, err := loadConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
		})
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("HUB_DB_PATH", "/tmp/test.db")
	t.Setenv("HUB_CONSUMER_TOKENS", "app1:oc-test")
	t.Setenv("HUB_BRIDGE_TOKEN", "br-test")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", cfg.host)
	assert.Equal(t, "8080", cfg.port)
	assert.Equal(t, "attachments", cfg.attachmentsDir)
	assert.Equal(t, map[string]string{"app1": "oc-test"}, cfg.consumerTokens)
	assert.Equal(t, "br-test", cfg.bridgeToken)
}

func TestLoadConfig_MultipleConsumers(t *testing.T) {
	t.Setenv("HUB_DB_PATH", "/tmp/test.db")
	t.Setenv("HUB_CONSUMER_TOKENS", "app1:oc-test,myapp:my-test")
	t.Setenv("HUB_BRIDGE_TOKEN", "br-test")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"app1": "oc-test",
		"myapp":    "my-test",
	}, cfg.consumerTokens)
}

func TestLoadConfig_InvalidConsumerTokensFormat(t *testing.T) {
	t.Setenv("HUB_DB_PATH", "/tmp/test.db")
	t.Setenv("HUB_CONSUMER_TOKENS", "bad-format-no-colon")
	t.Setenv("HUB_BRIDGE_TOKEN", "br-test")

	_, err := loadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse HUB_CONSUMER_TOKENS")
}

func TestLoadConfig_ConsumerTokenEqualsBridgeToken(t *testing.T) {
	t.Setenv("HUB_DB_PATH", "/tmp/test.db")
	t.Setenv("HUB_CONSUMER_TOKENS", "app1:same-secret")
	t.Setenv("HUB_BRIDGE_TOKEN", "same-secret")

	_, err := loadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not equal HUB_BRIDGE_TOKEN")
}

func TestLoadConfig_CustomValues(t *testing.T) {
	t.Setenv("HUB_DB_PATH", "/tmp/hub.db")
	t.Setenv("HUB_CONSUMER_TOKENS", "app1:oc-test")
	t.Setenv("HUB_BRIDGE_TOKEN", "br-test")
	t.Setenv("HUB_PORT", "9090")
	t.Setenv("HUB_ATTACHMENTS_DIR", "/tmp/att")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, "9090", cfg.port)
	assert.Equal(t, "/tmp/att", cfg.attachmentsDir)
	assert.Equal(t, "/tmp/hub.db", cfg.dbPath)
}

package main

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionFlag(t *testing.T) {
	// Build the binary with a known version.
	binPath := t.TempDir() + "/salmon-run-test"
	ldflags := "-X main.version=v1.2.3"
	//nolint:gosec // test builds trusted code
	buildCmd := exec.CommandContext(context.Background(), "go", "build", "-ldflags", ldflags, "-o", binPath, ".")
	buildCmd.Dir = "."
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", out)

	// Run with --version flag.
	//nolint:gosec // test runs trusted binary
	cmd := exec.CommandContext(context.Background(), binPath, "--version")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "command failed: %s", output)
	assert.Equal(t, "salmon-run v1.2.3\n", string(output))
}

func TestVersionDefault(t *testing.T) {
	assert.Equal(t, "dev", version)
}

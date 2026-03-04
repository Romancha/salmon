package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMakefileArchiveDetection verifies that the Makefile correctly detects
// release archive context (no go.mod) and adjusts source paths accordingly.
func TestMakefileArchiveDetection(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping on non-Darwin: Makefile install targets are macOS only")
	}

	repoRoot := findRepoRoot(t)
	makefilePath := filepath.Join(repoRoot, "Makefile")

	t.Run("repo context uses bin/ paths", func(t *testing.T) {
		// In repo context (go.mod exists), install-bridge depends on build
		// and uses bin/ prefixed paths.
		//nolint:gosec // trusted command with test-controlled args
		cmd := exec.CommandContext(context.Background(), "make", "-n", "-f", makefilePath, "-C", repoRoot, "install-bridge")
		cmd.Env = append(os.Environ(), "HOME=/tmp/test-home")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "make -n failed: %s", out)
		output := string(out)

		// "cp bin/bear-bridge" = source path with bin/ prefix (repo build output)
		assert.Contains(t, output, "cp bin/bear-bridge", "repo context should copy from bin/bear-bridge")
		assert.Contains(t, output, "cp -R bin/bear-xcall.app", "repo context should copy from bin/bear-xcall.app")
		assert.Contains(t, output, "deploy/bear-bridge-wrapper.sh", "repo context should reference deploy/ paths")
		assert.Contains(t, output, "go build", "repo context should run go build")
	})

	t.Run("archive context uses root paths", func(t *testing.T) {
		// Simulate archive context by copying Makefile to a temp dir without go.mod.
		archiveDir := t.TempDir()
		//nolint:gosec // test reads trusted Makefile from repo
		makefileContent, err := os.ReadFile(makefilePath)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(archiveDir, "Makefile"), makefileContent, 0o600)
		require.NoError(t, err)

		//nolint:gosec // trusted command with test-controlled args
		cmd := exec.CommandContext(context.Background(), "make", "-n", "-C", archiveDir, "install-bridge")
		cmd.Env = append(os.Environ(), "HOME=/tmp/test-home")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "make -n failed: %s", out)
		output := string(out)

		// "cp bear-bridge" without bin/ prefix = archive root path
		assert.Contains(t, output, "cp bear-bridge ", "archive context should copy from root bear-bridge")
		assert.Contains(t, output, "cp -R bear-xcall.app ", "archive context should copy from root bear-xcall.app")
		// Should NOT reference bin/ or deploy/ as source paths.
		assert.NotContains(t, output, "cp bin/", "archive context should not copy from bin/")
		assert.NotContains(t, output, "deploy/", "archive context should not reference deploy/ paths")
		// Should NOT try to build.
		assert.NotContains(t, output, "go build", "archive context should not run go build")
	})
}

// TestMakefileVerifyBridgeTarget verifies that verify-bridge target exists and
// references the expected paths.
func TestMakefileVerifyBridgeTarget(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping on non-Darwin: Makefile verify-bridge target is macOS only")
	}

	repoRoot := findRepoRoot(t)

	//nolint:gosec // trusted command with test-controlled args
	cmd := exec.CommandContext(context.Background(), "make", "-n", "-C", repoRoot, "verify-bridge")
	cmd.Env = append(os.Environ(), "HOME=/tmp/test-home")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "make -n failed: %s", out)
	output := string(out)

	assert.Contains(t, output, "codesign --verify", "verify-bridge should run codesign --verify")
	assert.Contains(t, output, "bear-bridge", "verify-bridge should check bear-bridge")
	assert.Contains(t, output, "bear-xcall.app", "verify-bridge should check bear-xcall.app")
}

// TestEnvBridgeExampleExists verifies the .env.bridge.example template exists.
func TestEnvBridgeExampleExists(t *testing.T) {
	repoRoot := findRepoRoot(t)
	examplePath := filepath.Join(repoRoot, "deploy", ".env.bridge.example")

	//nolint:gosec // test reads trusted file from known repo path
	content, err := os.ReadFile(examplePath)
	require.NoError(t, err, ".env.bridge.example should exist")

	text := string(content)
	assert.Contains(t, text, "BRIDGE_HUB_URL", "should contain BRIDGE_HUB_URL")
	assert.Contains(t, text, "BRIDGE_HUB_TOKEN", "should contain BRIDGE_HUB_TOKEN")
	assert.Contains(t, text, "BEAR_TOKEN", "should contain BEAR_TOKEN")
	assert.Contains(t, text, "BRIDGE_STATE_PATH", "should contain BRIDGE_STATE_PATH")
	assert.Contains(t, text, "BEAR_DB_DIR", "should contain BEAR_DB_DIR")
}

// findRepoRoot walks up from the current directory to find the repository root.
func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "could not find repo root (no go.mod found)")
		dir = parent
	}
}

package beardb_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateTestBearSQLite creates a Bear SQLite test fixture at the given path.
// Exported for use by other test packages (e.g., E2E tests).
func CreateTestBearSQLite(t *testing.T, dir string) string {
	t.Helper()

	dbPath := filepath.Join(dir, "bear.sqlite")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	createBearSchema(t, db)
	insertTestData(t, db)

	require.NoError(t, db.Close())

	return dbPath
}

// TestGenerateTestFixture creates a test Bear SQLite file in testdata/ directory.
func TestGenerateTestFixture(t *testing.T) {
	if os.Getenv("GENERATE_FIXTURES") == "" {
		t.Skip("set GENERATE_FIXTURES=1 to regenerate testdata/bear.sqlite")
	}

	dir := filepath.Join("..", "..", "testdata")

	require.NoError(t, os.MkdirAll(dir, 0o750))

	dbPath := filepath.Join(dir, "bear.sqlite")

	// Remove old fixture if exists.
	_ = os.Remove(dbPath)

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	createBearSchema(t, db)
	insertTestData(t, db)

	require.NoError(t, db.Close())

	t.Logf("Generated test fixture at %s", dbPath)
}

package testing

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/validator/db/kv"
)

func TestClearDB(t *testing.T) {
	// Setting up manually is required, since SetupDB() will also register a teardown procedure.
	testDB, err := kv.NewKVStore(context.Background(), t.TempDir(), &kv.Config{
		PubKeys: nil,
	})
	require.NoError(t, err, "Failed to instantiate DB")
	require.NoError(t, testDB.ClearDB())

	if _, err := os.Stat(filepath.Join(testDB.DatabasePath(), "validator.db")); !os.IsNotExist(err) {
		t.Fatalf("DB was not cleared: %v", err)
	}
}

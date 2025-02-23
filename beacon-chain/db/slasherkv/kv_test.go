package slasherkv

import (
	"context"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

// setupDB instantiates and returns a Store instance.
func setupDB(t testing.TB) *Store {
	db, err := NewKVStore(context.Background(), t.TempDir(), &Config{})
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		require.NoError(t, db.Close(), "Failed to close database")
	})
	return db
}

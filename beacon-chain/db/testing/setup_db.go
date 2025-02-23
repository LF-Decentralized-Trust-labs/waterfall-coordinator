// Package testing allows for spinning up a real bolt-db
// instance for unit tests throughout the Prysm repo.
package testing

import (
	"context"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/iface"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/kv"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/slasherkv"
)

// SetupDB instantiates and returns database backed by key value store.
func SetupDB(t testing.TB) db.Database {
	s, err := kv.NewKVStore(context.Background(), t.TempDir(), &kv.Config{GenesisSszPath: "testing/testdata/genesis.ssz"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("failed to close database: %v", err)
		}
	})
	return s
}

// SetupSlasherDB --
func SetupSlasherDB(t testing.TB) iface.SlasherDatabase {
	s, err := slasherkv.NewKVStore(context.Background(), t.TempDir(), &slasherkv.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("failed to close database: %v", err)
		}
	})
	return s
}

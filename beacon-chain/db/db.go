// Package db defines the ability to create a new database
// for an coordinator node.
package db

import (
	"context"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/kv"
)

// NewDB initializes a new DB.
func NewDB(ctx context.Context, dirPath string, config *kv.Config) (Database, error) {
	return kv.NewKVStore(ctx, dirPath, config)
}

// NewDBFilename uses the KVStoreDatafilePath so that if this layer of
// indirection between db.NewDB->kv.NewKVStore ever changes, it will be easy to remember
// to also change this filename indirection at the same time.
func NewDBFilename(dirPath string) string {
	return kv.KVStoreDatafilePath(dirPath)
}

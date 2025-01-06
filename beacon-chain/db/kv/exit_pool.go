//Copyright 2024   Blue Wave Inc.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package kv

import (
	"context"

	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// ReadExitPool retrieve the ExitPool.
func (s *Store) ReadExitPool(ctx context.Context) ([]*ethpb.VoluntaryExit, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.ReadExitPool")
	defer span.End()

	var err error
	var raw []byte
	key := exitOpPoolKey
	err = s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(valOpPoolBucket)
		raw = bkt.Get(key)
		return err
	})
	var data = make([]*ethpb.VoluntaryExit, 0)
	if err == nil && raw != nil {
		poolData := &ethpb.VoluntaryExitList{}
		if err := poolData.UnmarshalSSZ(raw); err != nil {
			return nil, err
		}
		data = poolData.Data
	}
	return data, err
}

// WriteExitPool save the ExitPool to the db.
func (s *Store) WriteExitPool(ctx context.Context, exits []*ethpb.VoluntaryExit) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.WriteExitPool")
	defer span.End()

	key := exitOpPoolKey
	list := &ethpb.VoluntaryExitList{
		Data: exits,
	}
	buff, err := list.MarshalSSZ()
	if err != nil {
		return err
	}
	return s.db.Batch(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(valOpPoolBucket)
		if err := bkt.Put(key, buff); err != nil {
			return err
		}
		return nil
	})
}

// DeleteExitPool from the db
func (s *Store) DeleteExitPool(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.DeleteExitPool")
	defer span.End()
	key := exitOpPoolKey
	return s.db.Batch(func(tx *bolt.Tx) error {
		if err := tx.Bucket(valOpPoolBucket).Delete(key); err != nil {
			return err
		}
		return nil
	})
}

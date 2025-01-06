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

// ReadWithdrawalPool retrieve the WithdrawalPool.
func (s *Store) ReadWithdrawalPool(ctx context.Context) ([]*ethpb.Withdrawal, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.ReadWithdrawalPool")
	defer span.End()

	var err error
	var raw []byte

	key := withdrawalOpPoolKey

	err = s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(valOpPoolBucket)
		raw = bkt.Get(key)
		return err
	})
	var data = make([]*ethpb.Withdrawal, 0)
	if err == nil && raw != nil {
		poolData := &ethpb.WithdrawalList{}
		if err := poolData.UnmarshalSSZ(raw); err != nil {
			return nil, err
		}
		data = poolData.Data
	}
	return data, err
}

// WriteWithdrawalPool save the WithdrawalPool to the db.
func (s *Store) WriteWithdrawalPool(ctx context.Context, withdrawals []*ethpb.Withdrawal) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.WriteWithdrawalPool")
	defer span.End()

	key := withdrawalOpPoolKey
	list := &ethpb.WithdrawalList{
		Data: withdrawals,
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

// DeleteWithdrawalPool from the db
func (s *Store) DeleteWithdrawalPool(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.DeleteWithdrawalPool")
	defer span.End()
	key := withdrawalOpPoolKey
	return s.db.Batch(func(tx *bolt.Tx) error {
		if err := tx.Bucket(valOpPoolBucket).Delete(key); err != nil {
			return err
		}
		return nil
	})
}

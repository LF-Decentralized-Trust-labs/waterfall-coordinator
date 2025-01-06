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
	"fmt"
	"testing"

	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

func TestStore_WithdrawalPoolCRUD(t *testing.T) {
	ctx := context.Background()

	var paramWithdrawalPoolTests = []struct {
		name                   string
		newWithdrawalPoolParam func() []*ethpb.Withdrawal
	}{
		{
			name: "withdrawal pool crud",
			newWithdrawalPoolParam: func() []*ethpb.Withdrawal {
				return NewWithdrawalPoolParam()
			},
		},
	}

	for _, tt := range paramWithdrawalPoolTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)

			withdrawalPool := tt.newWithdrawalPoolParam()
			retrievedPool, err := db.ReadWithdrawalPool(ctx)
			require.NoError(t, err)
			var nilGsp []*ethpb.Withdrawal
			assert.Equal(t, fmt.Sprintf("%v", nilGsp), fmt.Sprintf("%v", retrievedPool), "Expected nil ReadWithdrawalPool")
			err = db.WriteWithdrawalPool(ctx, withdrawalPool)
			require.NoError(t, err)

			retrievedPool, err = db.ReadWithdrawalPool(ctx)
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("%#x", withdrawalPool), fmt.Sprintf("%#x", retrievedPool), "Wanted: %#x, received: %#x", withdrawalPool, retrievedPool)
		})
	}
}

func NewWithdrawalPoolParam() []*ethpb.Withdrawal {
	return []*ethpb.Withdrawal{
		{
			PublicKey:      gwatCommon.BlsPubKey{0x01, 0x01, 0x01, 0x01, 0x01}.Bytes(),
			ValidatorIndex: 100,
			Amount:         100000000,
			InitTxHash:     gwatCommon.Hash{0x01, 0x01, 0x01, 0x01, 0x01}.Bytes(),
			Epoch:          1000,
		},
		{
			PublicKey:      gwatCommon.BlsPubKey{0x02, 0x02, 0x02, 0x02, 0x02}.Bytes(),
			ValidatorIndex: 200,
			Amount:         200000000,
			InitTxHash:     gwatCommon.Hash{0x02, 0x02, 0x02, 0x02, 0x02}.Bytes(),
			Epoch:          2000,
		},
		{
			PublicKey:      gwatCommon.BlsPubKey{0x03, 0x03, 0x03, 0x03, 0x03}.Bytes(),
			ValidatorIndex: 300,
			Amount:         300000000,
			InitTxHash:     gwatCommon.Hash{0x03, 0x03, 0x03, 0x03, 0x03}.Bytes(),
			Epoch:          3000,
		},
	}
}

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

func TestStore_ExitPoolCRUD(t *testing.T) {
	ctx := context.Background()

	var paramExitPoolTests = []struct {
		name             string
		newExitPoolParam func() []*ethpb.VoluntaryExit
	}{
		{
			name: "exit pool crud",
			newExitPoolParam: func() []*ethpb.VoluntaryExit {
				return NewExitPoolParam()
			},
		},
	}

	for _, tt := range paramExitPoolTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)

			exitPool := tt.newExitPoolParam()
			retrievedPool, err := db.ReadExitPool(ctx)
			require.NoError(t, err)
			var nilGsp []*ethpb.VoluntaryExit
			assert.Equal(t, fmt.Sprintf("%v", nilGsp), fmt.Sprintf("%v", retrievedPool), "Expected nil ReadExitPool")
			err = db.WriteExitPool(ctx, exitPool)
			require.NoError(t, err)

			retrievedPool, err = db.ReadExitPool(ctx)
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("%#x", exitPool), fmt.Sprintf("%#x", retrievedPool), "Wanted: %#x, received: %#x", exitPool, retrievedPool)
		})
	}
}

func NewExitPoolParam() []*ethpb.VoluntaryExit {
	return []*ethpb.VoluntaryExit{
		{
			Epoch:          1000,
			ValidatorIndex: 100,
			InitTxHash:     gwatCommon.Hash{0x01, 0x01, 0x01, 0x01, 0x01}.Bytes(),
		},
		{
			ValidatorIndex: 200,
			InitTxHash:     gwatCommon.Hash{0x02, 0x02, 0x02, 0x02, 0x02}.Bytes(),
			Epoch:          2000,
		},
		{
			ValidatorIndex: 300,
			InitTxHash:     gwatCommon.Hash{0x03, 0x03, 0x03, 0x03, 0x03}.Bytes(),
			Epoch:          3000,
		},
	}
}

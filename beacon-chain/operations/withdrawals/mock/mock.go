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

package mock

import (
	"bytes"
	"context"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	eth "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Withdrawals []*eth.Withdrawal
}

func (m *PoolMock) InsertWithdrawalByGwat(ctx context.Context, withdrawal *eth.Withdrawal) {
	m.Withdrawals = append(m.Withdrawals, withdrawal)
}

// PendingWithdrawals --
func (m *PoolMock) PendingWithdrawals(_ state.ReadOnlyBeaconState, _ types.Slot, _ bool) []*eth.Withdrawal {
	return m.Withdrawals
}

// InsertWithdrawal --
func (m *PoolMock) InsertWithdrawal(_ context.Context, withdrawal *eth.Withdrawal) {
	m.Withdrawals = append(m.Withdrawals, withdrawal)
}

// MarkIncluded --
func (m *PoolMock) MarkIncluded(withdrawal *eth.Withdrawal) {
	res := make([]*eth.Withdrawal, 0, len(m.Withdrawals))
	for _, w := range m.Withdrawals {
		if bytes.Equal(w.InitTxHash, withdrawal.InitTxHash) {
			continue
		}
		res = append(res, w)
	}
	m.Withdrawals = res
}

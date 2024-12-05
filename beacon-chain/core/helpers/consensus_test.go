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

package helpers_test

import (
	"fmt"
	"math"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	eth "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

func TestValidateUnpublishedChains(t *testing.T) {
	var err error
	chain_1 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte{'a', '1'}),
	}
	chain_2 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte{'a', '1'}), gwatCommon.BytesToHash([]byte{'b', '2'}),
	}
	chain_3 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte{'a', '1'}), gwatCommon.BytesToHash([]byte{'a', '2'}),
	}

	// No errors
	correctParam := []gwatCommon.HashArray{
		chain_3, chain_2, chain_1,
	}
	err = helpers.ConsensusValidateUnpublishedChains(correctParam)
	assert.NoError(t, err)

	// err "contains empty chain"
	unpubChains := []gwatCommon.HashArray{
		chain_3, {}, chain_1,
	}
	err = helpers.ConsensusValidateUnpublishedChains(unpubChains)
	assert.ErrorContains(t, helpers.ErrBadUnpublishedChains.Error(), err)
	assert.ErrorContains(t, "contains empty chain", err)

	// err "the first values of each chain must be equal"
	// bad chain
	chain_4 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte{'c', '1'}), gwatCommon.BytesToHash([]byte{'a', '2'}),
	}
	unpubChains = []gwatCommon.HashArray{
		chain_4, chain_3, chain_2, chain_1,
	}
	err = helpers.ConsensusValidateUnpublishedChains(unpubChains)
	assert.ErrorContains(t, helpers.ErrBadUnpublishedChains.Error(), err)
	assert.ErrorContains(t, "the first values of chain are not equal", err)

	// err "chain is not uniq"
	chain_5 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte{'a', '1'}), gwatCommon.BytesToHash([]byte{'d', '2'}), gwatCommon.BytesToHash([]byte{'d', '2'}),
	}
	unpubChains = []gwatCommon.HashArray{
		chain_5, chain_3, chain_2, chain_1,
	}
	err = helpers.ConsensusValidateUnpublishedChains(unpubChains)
	assert.ErrorContains(t, helpers.ErrBadUnpublishedChains.Error(), err)
	assert.ErrorContains(t, "chain is not uniq", err)
}

func TestConsensusCalculatePefix_OK(t *testing.T) {
	chain_1 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")),
	}
	chain_2 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")), gwatCommon.BytesToHash([]byte("b2")),
	}
	chain_3 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")), gwatCommon.BytesToHash([]byte("a2")),
	}

	unpubChains := []gwatCommon.HashArray{
		chain_3,
		chain_2,
		chain_1,
	}
	want := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")),
	}
	prefix, err := helpers.ConsensusCalcPrefix(unpubChains)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%v", want), fmt.Sprintf("%v", prefix))
}

func TestConsensusCalculatePefix_AdditionalOK(t *testing.T) {
	chain_1 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")), gwatCommon.BytesToHash([]byte("a2")), gwatCommon.BytesToHash([]byte("a3")), gwatCommon.BytesToHash([]byte("a4")),
	}
	chain_2 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")), gwatCommon.BytesToHash([]byte("a2")), gwatCommon.BytesToHash([]byte("a3")), gwatCommon.BytesToHash([]byte("a4")),
	}
	chain_3 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")), gwatCommon.BytesToHash([]byte("a2")), gwatCommon.BytesToHash([]byte("a3")), gwatCommon.BytesToHash([]byte("a4")),
	}
	chain_4 := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")), gwatCommon.BytesToHash([]byte("a2")), gwatCommon.BytesToHash([]byte("b3")), gwatCommon.BytesToHash([]byte("a4")),
	}

	unpubChains := []gwatCommon.HashArray{
		chain_4, chain_3, chain_2, chain_1,
	}
	want := gwatCommon.HashArray{
		gwatCommon.BytesToHash([]byte("a1")), gwatCommon.BytesToHash([]byte("a2")),
	}
	prefix, err := helpers.ConsensusCalcPrefix(unpubChains)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%v", want), fmt.Sprintf("%v", prefix))
}

func TestProcessWithdrawalOps_OK(t *testing.T) {

	bstate, err := util.NewBeaconState()
	assert.NoError(t, err)

	staleAfterSlots := types.Slot(params.BeaconConfig().CleanWithdrawalsAftEpochs) * params.BeaconConfig().SlotsPerEpoch
	params.BeaconConfig().DelegateForkSlot = staleAfterSlots
	err = bstate.SetSlot(10 + staleAfterSlots)
	assert.NoError(t, err)

	preFinRoot := bytesutil.PadTo([]byte{0x11}, 32)
	err = bstate.SetFinalizedCheckpoint(&eth.Checkpoint{
		Epoch: 0,
		Root:  bytesutil.PadTo([]byte{0x22}, 32),
	})
	assert.NoError(t, err)

	err = bstate.AppendValidator(&eth.Validator{
		PublicKey:                  nil,
		CreatorAddress:             nil,
		WithdrawalCredentials:      nil,
		EffectiveBalance:           3_200_000_000_000,
		Slashed:                    false,
		ActivationEligibilityEpoch: 0,
		ActivationEpoch:            0,
		ExitEpoch:                  math.MaxUint64,
		WithdrawableEpoch:          math.MaxUint64,
		ActivationHash:             nil,
		ExitHash:                   nil,
		WithdrawalOps: []*eth.WithdrawalOp{
			{Slot: 5},
			{Slot: 10},
			{Slot: 11},
			{Slot: 55},
		},
	})
	assert.NoError(t, err)

	expectValidators := []*eth.WithdrawalOp{
		{Slot: 11},
		{Slot: 55},
	}

	postState, err := helpers.ProcessWithdrawalOps(bstate.Copy(), preFinRoot)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%v", expectValidators), fmt.Sprintf("%v", postState.Validators()[0].WithdrawalOps))

	// clean all withrawal op
	err = bstate.SetSlot(100 + staleAfterSlots)
	assert.NoError(t, err)

	expectValidators = []*eth.WithdrawalOp{}

	postState, err = helpers.ProcessWithdrawalOps(bstate.Copy(), preFinRoot)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%v", expectValidators), fmt.Sprintf("%v", postState.Validators()[0].WithdrawalOps))
}

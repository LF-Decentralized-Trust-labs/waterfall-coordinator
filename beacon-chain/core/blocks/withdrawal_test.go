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

package blocks_test

import (
	"context"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"math"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/blocks"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestVerifyWithdrawalData_ErrWithdrawalIsNil(t *testing.T) {
	var withdrawal *ethpb.Withdrawal
	validator := &ethpb.Validator{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
		ActivationHash: make([]byte, 32),
		ExitHash:       make([]byte, 32),
		WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{validator},
		Slot:       10,
	})
	require.NoError(t, err)
	roVal, err := state.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)

	want := blocks.ErrWithdrawalIsNil
	err = blocks.VerifyWithdrawalData(withdrawal, roVal, 0, 0)
	assert.ErrorContains(t, want.Error(), err)
}

func TestVerifyWithdrawalData_ErrWithdrawalBadValidatorIndex(t *testing.T) {
	withdrawal := &ethpb.Withdrawal{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ValidatorIndex: math.MaxUint64,
		Amount:         10,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          0,
	}
	validator := &ethpb.Validator{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
		ActivationHash: make([]byte, 32),
		ExitHash:       make([]byte, 32),
		WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{validator},
		Slot:       10,
	})
	require.NoError(t, err)
	roVal, err := state.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)

	want := blocks.ErrWithdrawalBadValidatorIndex
	err = blocks.VerifyWithdrawalData(withdrawal, roVal, 0, 0)
	assert.ErrorContains(t, want.Error(), err)
}

func TestVerifyWithdrawalData_ErrWithdrawalBadPublicKey(t *testing.T) {
	withdrawal := &ethpb.Withdrawal{
		PublicKey:      bytesutil.PadTo([]byte{0x11, 0x11}, 48),
		ValidatorIndex: 0,
		Amount:         10,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          0,
	}
	validator := &ethpb.Validator{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
		ActivationHash: make([]byte, 32),
		ExitHash:       make([]byte, 32),
		WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{validator},
		Slot:       10,
	})
	require.NoError(t, err)
	roVal, err := state.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)

	want := blocks.ErrWithdrawalBadPublicKey
	err = blocks.VerifyWithdrawalData(withdrawal, roVal, 0, 0)
	assert.ErrorContains(t, want.Error(), err)
}

func TestVerifyWithdrawalData_ErrWithdrawalBadEpoch(t *testing.T) {
	withdrawal := &ethpb.Withdrawal{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ValidatorIndex: 0,
		Amount:         10,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          111,
	}
	validator := &ethpb.Validator{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
		ActivationHash: make([]byte, 32),
		ExitHash:       make([]byte, 32),
		WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{validator},
		Slot:       10,
	})
	require.NoError(t, err)
	roVal, err := state.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)

	want := blocks.ErrWithdrawalBadEpoch
	err = blocks.VerifyWithdrawalData(withdrawal, roVal, 0, 0)
	assert.ErrorContains(t, want.Error(), err)
}

func TestVerifyWithdrawalData_ErrWithdrawalLowBalance(t *testing.T) {
	withdrawal := &ethpb.Withdrawal{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ValidatorIndex: 0,
		Amount:         10,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          111,
	}
	validator := &ethpb.Validator{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
		ActivationHash: make([]byte, 32),
		ExitHash:       make([]byte, 32),
		WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{validator},
		Slot:       10,
	})
	require.NoError(t, err)
	roVal, err := state.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)

	want := blocks.ErrWithdrawalBadEpoch
	err = blocks.VerifyWithdrawalData(withdrawal, roVal, 111, 9)
	assert.ErrorContains(t, want.Error(), err)
}

func TestVerifyWithdrawalData_ErrWithdrawalAlreadyApplied(t *testing.T) {
	withdrawal := &ethpb.Withdrawal{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ValidatorIndex: 0,
		Amount:         10,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          2,
	}
	validator := &ethpb.Validator{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
		ActivationHash: make([]byte, 32),
		ExitHash:       make([]byte, 32),
		WithdrawalOps: []*ethpb.WithdrawalOp{{
			Amount: withdrawal.Amount,
			Hash:   withdrawal.InitTxHash,
		}},
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{validator},
		Slot:       10,
	})
	require.NoError(t, err)
	roVal, err := state.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)

	want := blocks.ErrWithdrawalAlreadyApplied
	err = blocks.VerifyWithdrawalData(withdrawal, roVal, 111, 10)
	assert.ErrorContains(t, want.Error(), err)
}

func TestVerifyWithdrawalData_Ok(t *testing.T) {
	withdrawal := &ethpb.Withdrawal{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ValidatorIndex: 0,
		Amount:         10,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          111,
	}
	validator := &ethpb.Validator{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
		ActivationHash: make([]byte, 32),
		ExitHash:       make([]byte, 32),
		WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{validator},
		Slot:       10,
	})
	require.NoError(t, err)
	roVal, err := state.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)

	want := blocks.ErrWithdrawalBadEpoch
	err = blocks.VerifyWithdrawalData(withdrawal, roVal, 111, 10)
	assert.ErrorContains(t, want.Error(), err)
}

func TestProcessWithdrawal_Ok(t *testing.T) {
	params.UseTestConfig()
	withdrawals := []*ethpb.Withdrawal{{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ValidatorIndex: 0,
		Amount:         10,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          0,
	}}
	registry := []*ethpb.Validator{
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
			ActivationHash: make([]byte, 32),
			ExitHash:       make([]byte, 32),
			WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
		},
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: registry,
		Slot:       10,
		Balances:   []uint64{3_290_000_000_000},
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			Withdrawals: withdrawals,
		},
	}

	_, err = blocks.ProcessWithdrawal(context.Background(), state, b.Block.Body.Withdrawals)
	require.NoError(t, err)

	//check balance
	expBal := 3_290_000_000_000 - withdrawals[0].Amount
	gotBal, err := state.BalanceAtIndex(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, expBal, gotBal)

	//check validator
	gotVal, err := state.ValidatorAtIndexReadOnly(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, 2, len(gotVal.WithdrawalOps()))
	require.Equal(t, withdrawals[0].Amount, gotVal.WithdrawalOps()[1].Amount)
	require.DeepEqual(t, withdrawals[0].InitTxHash, gotVal.WithdrawalOps()[1].Hash)
	require.Equal(t, state.Slot(), gotVal.WithdrawalOps()[1].GetSlot())
}

func TestProcessWithdrawal_SkippingAlreadyApplied(t *testing.T) {
	params.UseTestConfig()
	withdrawals := []*ethpb.Withdrawal{
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ValidatorIndex: 0,
			Amount:         10,
			InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
			Epoch:          0,
		},
		//duplicate withdrawal op
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ValidatorIndex: 0,
			Amount:         10,
			InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
			Epoch:          0,
		},
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ValidatorIndex: 0,
			Amount:         10,
			InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
			Epoch:          0,
		},
	}
	registry := []*ethpb.Validator{
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
			ActivationHash: make([]byte, 32),
			ExitHash:       make([]byte, 32),
			WithdrawalOps: []*ethpb.WithdrawalOp{{
				Amount: 10,
				Hash:   bytesutil.PadTo([]byte{0x77}, 32),
				Slot:   10,
			}},
		},
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: registry,
		Slot:       10,
		Balances:   []uint64{3_290_000_000_000},
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			Withdrawals: withdrawals,
		},
	}

	_, err = blocks.ProcessWithdrawal(context.Background(), state, b.Block.Body.Withdrawals)
	require.NoError(t, err)

	//check balance
	expBal := uint64(3_290_000_000_000)
	gotBal, err := state.BalanceAtIndex(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, expBal, gotBal)

	//check validator
	gotVal, err := state.ValidatorAtIndexReadOnly(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, 1, len(gotVal.WithdrawalOps()))
	require.Equal(t, withdrawals[0].Amount, gotVal.WithdrawalOps()[0].Amount)
	require.DeepEqual(t, withdrawals[0].InitTxHash, gotVal.WithdrawalOps()[0].Hash)
	require.Equal(t, state.Slot(), gotVal.WithdrawalOps()[0].GetSlot())
}

func TestProcessWithdrawal_WithdrawalOpsLimit(t *testing.T) {
	params.UseTestConfig()
	withdrawals := []*ethpb.Withdrawal{
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ValidatorIndex: 0,
			Amount:         10,
			InitTxHash:     bytesutil.PadTo([]byte{0x11}, 32),
			Epoch:          0,
		},
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ValidatorIndex: 0,
			Amount:         10,
			InitTxHash:     bytesutil.PadTo([]byte{0x22}, 32),
			Epoch:          0,
		},
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ValidatorIndex: 0,
			Amount:         20,
			InitTxHash:     bytesutil.PadTo([]byte{0x33}, 32),
			Epoch:          0,
		},
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ValidatorIndex: 0,
			Amount:         30,
			InitTxHash:     bytesutil.PadTo([]byte{0x44}, 32),
			Epoch:          0,
		},
	}
	registry := []*ethpb.Validator{
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
			ActivationHash: make([]byte, 32),
			ExitHash:       make([]byte, 32),
			WithdrawalOps: []*ethpb.WithdrawalOp{
				{
					Amount: 10,
					Hash:   bytesutil.PadTo([]byte{0x11}, 32),
					Slot:   10,
				},
				{
					Amount: 10,
					Hash:   bytesutil.PadTo([]byte{0x22}, 32),
					Slot:   10,
				},
			},
		},
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: registry,
		Slot:       10,
		Balances:   []uint64{3_290_000_000_000},
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			Withdrawals: withdrawals,
		},
	}

	// reduce limit
	params.BeaconConfig().WithdrawalOpsLimit = 2

	_, err = blocks.ProcessWithdrawal(context.Background(), state, b.Block.Body.Withdrawals)
	require.NoError(t, err)

	//check balance
	expBal := uint64(3_290_000_000_000)
	gotBal, err := state.BalanceAtIndex(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, expBal, gotBal)

	//check validator
	gotVal, err := state.ValidatorAtIndexReadOnly(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, 2, len(gotVal.WithdrawalOps()))
	//check items
	require.Equal(t, withdrawals[0].Amount, gotVal.WithdrawalOps()[0].Amount)
	require.DeepEqual(t, withdrawals[0].InitTxHash, gotVal.WithdrawalOps()[0].Hash)
	require.Equal(t, withdrawals[1].Amount, gotVal.WithdrawalOps()[1].Amount)
	require.DeepEqual(t, withdrawals[1].InitTxHash, gotVal.WithdrawalOps()[1].Hash)

	require.Equal(t, state.Slot(), gotVal.WithdrawalOps()[0].GetSlot())
	require.Equal(t, state.Slot(), gotVal.WithdrawalOps()[1].GetSlot())
}

func TestProcessWithdrawal_WithdrawalEntireAvailableBalance(t *testing.T) {
	params.UseTestConfig()
	withdrawals := []*ethpb.Withdrawal{{
		PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
		ValidatorIndex: 0,
		Amount:         0,
		InitTxHash:     bytesutil.PadTo([]byte{0x77}, 32),
		Epoch:          0,
	}}
	registry := []*ethpb.Validator{
		{
			PublicKey:      bytesutil.PadTo([]byte{0x11}, 48),
			ExitEpoch:      params.BeaconConfig().FarFutureEpoch,
			ActivationHash: make([]byte, 32),
			ExitHash:       make([]byte, 32),
			WithdrawalOps: []*ethpb.WithdrawalOp{{
				Amount: 90_000_000_000,
				Hash:   bytesutil.PadTo([]byte{0x77}, 32),
				Slot:   10,
			}},
		},
	}
	state, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: registry,
		Slot:       10,
		Balances:   []uint64{3_290_000_000_000},
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			Withdrawals: withdrawals,
		},
	}

	_, err = blocks.ProcessWithdrawal(context.Background(), state, b.Block.Body.Withdrawals)
	require.NoError(t, err)

	//check balance
	expBal := uint64(3_290_000_000_000)
	gotBal, err := state.BalanceAtIndex(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, expBal, gotBal)

	//check validator
	gotVal, err := state.ValidatorAtIndexReadOnly(withdrawals[0].ValidatorIndex)
	require.NoError(t, err)
	require.Equal(t, 1, len(gotVal.WithdrawalOps()))
	require.Equal(t, 3_290_000_000_000-params.BeaconConfig().MaxEffectiveBalance, gotVal.WithdrawalOps()[0].Amount)
	require.DeepEqual(t, withdrawals[0].InitTxHash, gotVal.WithdrawalOps()[0].Hash)
	require.Equal(t, state.Slot(), gotVal.WithdrawalOps()[0].GetSlot())
}

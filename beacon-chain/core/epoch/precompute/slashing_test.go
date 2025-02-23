package precompute_test

import (
	"fmt"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/epoch/precompute"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"google.golang.org/protobuf/proto"
)

func TestProcessSlashingsPrecompute_NotSlashedWithSlashedTrue(t *testing.T) {
	s, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Slot:       0,
		Validators: []*ethpb.Validator{{Slashed: true}},
		Balances:   []uint64{params.BeaconConfig().MaxEffectiveBalance},
		Slashings:  []uint64{0, 1e9},
	})
	require.NoError(t, err)
	pBal := &precompute.Balance{ActiveCurrentEpoch: params.BeaconConfig().MaxEffectiveBalance}
	require.NoError(t, precompute.ProcessSlashingsPrecompute(s, pBal))

	wanted := params.BeaconConfig().MaxEffectiveBalance
	assert.Equal(t, wanted, s.Balances()[0], "Unexpected slashed balance")
}

func TestProcessSlashingsPrecompute_NotSlashedWithSlashedFalse(t *testing.T) {
	s, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Slot:       0,
		Validators: []*ethpb.Validator{{}},
		Balances:   []uint64{params.BeaconConfig().MaxEffectiveBalance},
		Slashings:  []uint64{0, 1e9},
	})
	require.NoError(t, err)
	pBal := &precompute.Balance{ActiveCurrentEpoch: params.BeaconConfig().MaxEffectiveBalance}
	require.NoError(t, precompute.ProcessSlashingsPrecompute(s, pBal))

	wanted := params.BeaconConfig().MaxEffectiveBalance
	assert.Equal(t, wanted, s.Balances()[0], "Unexpected slashed balance")
}

func TestProcessSlashingsPrecompute_SlashedLess(t *testing.T) {
	params.UseTestConfig()
	tests := []struct {
		state *ethpb.BeaconState
		want  uint64
	}{
		{
			state: &ethpb.BeaconState{
				Validators: []*ethpb.Validator{
					{
						ActivationHash:    make([]byte, 32),
						ExitHash:          make([]byte, 32),
						WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
						Slashed:           true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{
						ActivationHash: make([]byte, 32),
						ExitHash:       make([]byte, 32),
						WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
						ExitEpoch:      params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance}},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 1e9},
			},
			// penalty    = validator balance / increment * (2*total_penalties) / total_balance * increment
			// 1000000000 = (32 * 1e9)        / (1 * 1e9) * (1*1e9)             / (32*1e9)      * (1 * 1e9)
			want: uint64(3200000000000), // 32 * 1e9 - 1000000000
		},
		{
			state: &ethpb.BeaconState{
				Validators: []*ethpb.Validator{
					{
						ActivationHash:    make([]byte, 32),
						ExitHash:          make([]byte, 32),
						WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
						Slashed:           true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{
						ActivationHash: make([]byte, 32),
						ExitHash:       make([]byte, 32),
						WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
						ExitEpoch:      params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
					{
						ActivationHash: make([]byte, 32),
						ExitHash:       make([]byte, 32),
						WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
						ExitEpoch:      params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
				},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 1e9},
			},
			// penalty    = validator balance / increment * (2*total_penalties) / total_balance * increment
			// 500000000 = (32 * 1e9)        / (1 * 1e9) * (1*1e9)             / (32*1e9)      * (1 * 1e9)
			want: uint64(3200000000000), // 32 * 1e9 - 500000000
		},
		{
			state: &ethpb.BeaconState{
				Validators: []*ethpb.Validator{
					{
						ActivationHash:    make([]byte, 32),
						ExitHash:          make([]byte, 32),
						WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
						Slashed:           true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{
						ActivationHash: make([]byte, 32),
						ExitHash:       make([]byte, 32),
						WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
						ExitEpoch:      params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
					{
						ActivationHash: make([]byte, 32),
						ExitHash:       make([]byte, 32),
						WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
						ExitEpoch:      params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
				},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 2 * 1e9},
			},
			// penalty    = validator balance / increment * (3*total_penalties) / total_balance * increment
			// 1000000000 = (32 * 1e9)        / (1 * 1e9) * (1*2e9)             / (64*1e9)      * (1 * 1e9)
			want: uint64(3200000000000), // 32 * 1e9 - 1000000000
		},
		{
			state: &ethpb.BeaconState{
				Validators: []*ethpb.Validator{
					{
						ActivationHash:    make([]byte, 32),
						ExitHash:          make([]byte, 32),
						WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
						Slashed:           true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement},
					{
						ActivationHash: make([]byte, 32),
						ExitHash:       make([]byte, 32),
						WithdrawalOps:  make([]*ethpb.WithdrawalOp, 0),
						ExitEpoch:      params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement}},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement, params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement},
				Slashings: []uint64{0, 1e9},
			},
			// penalty    = validator balance           / increment * (3*total_penalties) / total_balance        * increment
			// 2000000000 = (32  * 1e9 - 1*1e9)         / (1 * 1e9) * (2*1e9)             / (31*1e9)             * (1 * 1e9)
			want: uint64(3100000000000), // 32 * 1e9 - 2000000000
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			ab := uint64(0)
			for i, b := range tt.state.Balances {
				// Skip validator 0 since it's slashed
				if i == 0 {
					continue
				}
				ab += b
			}
			pBal := &precompute.Balance{ActiveCurrentEpoch: ab}

			original := proto.Clone(tt.state)
			state, err := v1.InitializeFromProto(tt.state)
			require.NoError(t, err)
			require.NoError(t, precompute.ProcessSlashingsPrecompute(state, pBal))
			assert.Equal(t, tt.want, state.Balances()[0], "ProcessSlashings({%v}) = newState; newState.Balances[0] = %d; wanted %d", original, state.Balances()[0])
		})
	}
}

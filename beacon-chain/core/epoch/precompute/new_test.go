package precompute_test

import (
	"context"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/epoch/precompute"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestNew(t *testing.T) {
	ffe := params.BeaconConfig().FarFutureEpoch
	s, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Slot: params.BeaconConfig().SlotsPerEpoch,
		// Validator 0 is slashed
		// Validator 1 is withdrawable
		// Validator 2 is active prev epoch and current epoch
		// Validator 3 is active prev epoch
		Validators: []*ethpb.Validator{
			{Slashed: true, WithdrawableEpoch: ffe, EffectiveBalance: 100},
			{EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: 1, EffectiveBalance: 100},
		},
	})
	require.NoError(t, err)
	e := params.BeaconConfig().FarFutureSlot
	v, b, err := precompute.New(context.Background(), s)
	require.NoError(t, err)
	assert.DeepEqual(t, &precompute.Validator{
		IsSlashed:                    true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[0], "Incorrect validator 0 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[1], "Incorrect validator 1 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[2], "Incorrect validator 2 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActivePrevEpoch:            true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[3], "Incorrect validator 3 status")

	wantedBalances := &precompute.Balance{
		ActiveCurrentEpoch: 100,
		ActivePrevEpoch:    200,
	}
	assert.DeepEqual(t, wantedBalances, b, "Incorrect wanted balance")
}

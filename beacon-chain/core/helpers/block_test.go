package helpers_test

import (
	"fmt"
	"math"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestBlockRootAtSlot_CorrectBlockRoot(t *testing.T) {
	var blockRoots [][]byte

	for i := uint64(0); i < uint64(params.BeaconConfig().SlotsPerHistoricalRoot); i++ {
		blockRoots = append(blockRoots, []byte{byte(i)})
	}
	s := &ethpb.BeaconState{
		BlockRoots: blockRoots,
	}

	tests := []struct {
		slot         types.Slot
		stateSlot    types.Slot
		expectedRoot [32]byte
	}{
		{
			slot:         0,
			stateSlot:    1,
			expectedRoot: [32]byte{0},
		},
		{
			slot:         2,
			stateSlot:    5,
			expectedRoot: [32]byte{2},
		},
		{
			slot:         64,
			stateSlot:    128,
			expectedRoot: [32]byte{64},
		}, {
			slot:         2999,
			stateSlot:    3000,
			expectedRoot: [32]byte{183},
		}, {
			slot:         2873,
			stateSlot:    3000,
			expectedRoot: [32]byte{57},
		},
		{
			slot:         0,
			stateSlot:    params.BeaconConfig().SlotsPerHistoricalRoot,
			expectedRoot: [32]byte{},
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			s.Slot = tt.stateSlot
			state, err := v1.InitializeFromProto(s)
			require.NoError(t, err)
			wantedSlot := tt.slot
			result, err := helpers.BlockRootAtSlot(state, wantedSlot)
			require.NoError(t, err, "Failed to get block root at slot %d", wantedSlot)
			assert.DeepEqual(t, tt.expectedRoot[:], result, "Result block root was an unexpected value")
		})
	}
}

func TestBlockRootAtSlot_OutOfBounds(t *testing.T) {
	var blockRoots [][]byte

	for i := uint64(0); i < uint64(params.BeaconConfig().SlotsPerHistoricalRoot); i++ {
		blockRoots = append(blockRoots, []byte{byte(i)})
	}
	state := &ethpb.BeaconState{
		BlockRoots: blockRoots,
	}

	tests := []struct {
		slot        types.Slot
		stateSlot   types.Slot
		expectedErr string
	}{
		{
			slot:        1000,
			stateSlot:   500,
			expectedErr: "slot 1000 out of bounds",
		},
		{
			slot:        3000,
			stateSlot:   3000,
			expectedErr: "slot 3000 out of bounds",
		},
		{
			// Edge case where stateSlot is over slots per historical root and
			// slot is not within (stateSlot - HistoricalRootsLimit, statSlot]
			slot:        1,
			stateSlot:   params.BeaconConfig().SlotsPerHistoricalRoot + 2,
			expectedErr: "slot 1 out of bounds",
		},
		{
			slot:        math.MaxUint64 - 5,
			stateSlot:   0, // Doesn't matter
			expectedErr: "slot overflows uint64",
		},
	}
	for _, tt := range tests {
		state.Slot = tt.stateSlot
		s, err := v1.InitializeFromProto(state)
		require.NoError(t, err)
		_, err = helpers.BlockRootAtSlot(s, tt.slot)
		assert.ErrorContains(t, tt.expectedErr, err)
	}
}

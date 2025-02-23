package helpers_test

import (
	"context"
	"fmt"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestWeakSubjectivity_ComputeWeakSubjectivityPeriod(t *testing.T) {
	params.UseTestConfig()
	tests := []struct {
		valCount   uint64
		avgBalance uint64
		want       types.Epoch
	}{
		// Asserting that we get the same numbers as defined in the reference table:
		// https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/weak-subjectivity.md#calculating-the-weak-subjectivity-period
		{valCount: 32768, avgBalance: 28, want: 4},
		{valCount: 65536, avgBalance: 28, want: 4},
		{valCount: 131072, avgBalance: 28, want: 4},
		{valCount: 262144, avgBalance: 28, want: 4},
		{valCount: 524288, avgBalance: 28, want: 5},
		{valCount: 1048576, avgBalance: 28, want: 6},
		{valCount: 32768, avgBalance: 32, want: 4},
		{valCount: 65536, avgBalance: 32, want: 4},
		{valCount: 131072, avgBalance: 32, want: 4},
		{valCount: 262144, avgBalance: 32, want: 4},
		{valCount: 524288, avgBalance: 32, want: 5},
		{valCount: 1048576, avgBalance: 32, want: 7},
		// Additional test vectors, to check case when T*(200+3*D) >= t*(200+12*D)
		{valCount: 32768, avgBalance: 22, want: 4},
		{valCount: 65536, avgBalance: 22, want: 4},
		{valCount: 131072, avgBalance: 22, want: 4},
		{valCount: 262144, avgBalance: 22, want: 4},
		{valCount: 524288, avgBalance: 22, want: 5},
		{valCount: 1048576, avgBalance: 22, want: 6},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("valCount: %d, avgBalance: %d", tt.valCount, tt.avgBalance), func(t *testing.T) {
			// Reset committee cache - as we need to recalculate active validator set for each test.
			helpers.ClearCache()
			got, err := helpers.ComputeWeakSubjectivityPeriod(context.Background(), genState(t, tt.valCount, tt.avgBalance), params.BeaconConfig())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got, "valCount: %v, avgBalance: %v", tt.valCount, tt.avgBalance)
		})
	}
}

type mockWsCheckpoint func() (stateRoot [32]byte, blockRoot [32]byte, e types.Epoch)

func TestWeakSubjectivity_IsWithinWeakSubjectivityPeriod(t *testing.T) {
	tests := []struct {
		name            string
		epoch           types.Epoch
		genWsState      func() state.ReadOnlyBeaconState
		genWsCheckpoint mockWsCheckpoint
		want            bool
		wantedErr       string
	}{
		{
			name: "nil weak subjectivity state",
			genWsState: func() state.ReadOnlyBeaconState {
				return nil
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, types.Epoch) {
				return [32]byte{}, [32]byte{}, 42
			},
			wantedErr: "invalid weak subjectivity state or checkpoint",
		},
		{
			name: "state and checkpoint roots do not match",
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 32)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&ethpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot1"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, types.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot2"), 32))
				return sr, [32]byte{}, 42
			},
			wantedErr: fmt.Sprintf("state (%#x) and checkpoint (%#x) roots do not match",
				bytesutil.PadTo([]byte("stateroot1"), 32), bytesutil.PadTo([]byte("stateroot2"), 32)),
		},
		{
			name: "state and checkpoint epochs do not match",
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 32)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&ethpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, types.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 43
			},
			wantedErr: "state (42) and checkpoint (43) epochs do not match",
		},
		{
			name: "no active validators",
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 0, 32)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&ethpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, types.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 42
			},
			wantedErr: "cannot compute weak subjectivity period: no active validators found",
		},
		{
			name:  "outside weak subjectivity period",
			epoch: 300,
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 32)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&ethpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, types.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 42
			},
			want: false,
		},
		{
			name:  "within weak subjectivity period",
			epoch: 299,
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 32)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&ethpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, types.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 42
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr, _, e := tt.genWsCheckpoint()
			got, err := helpers.IsWithinWeakSubjectivityPeriod(context.Background(), tt.epoch, tt.genWsState(), sr, e, params.BeaconConfig())
			if tt.wantedErr != "" {
				assert.Equal(t, false, got)
				assert.ErrorContains(t, tt.wantedErr, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWeakSubjectivity_ParseWeakSubjectivityInputString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkpt   *ethpb.Checkpoint
		wantedErr string
	}{
		{
			name:      "No column in string",
			input:     "0x111111;123",
			wantedErr: "did not contain column",
		},
		{
			name:      "Too many columns in string",
			input:     "0x010203:123:456",
			wantedErr: "weak subjectivity checkpoint input should be in `block_root:epoch_number` format",
		},
		{
			name:      "Incorrect block root length",
			input:     "0x010203:987",
			wantedErr: "block root is not length of 32",
		},
		{
			name:  "Correct input",
			input: "0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &ethpb.Checkpoint{
				Root:  []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: types.Epoch(123456789),
			},
		},
		{
			name:  "Correct input without 0x",
			input: "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &ethpb.Checkpoint{
				Root:  []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: types.Epoch(123456789),
			},
		},
		{
			name:  "Correct input",
			input: "0xF0FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &ethpb.Checkpoint{
				Root:  []byte{0xf0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: types.Epoch(123456789),
			},
		},
		{
			name:  "Correct input without 0x",
			input: "F0FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &ethpb.Checkpoint{
				Root:  []byte{0xf0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: types.Epoch(123456789),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsCheckpt, err := helpers.ParseWeakSubjectivityInputString(tt.input)
			if tt.wantedErr != "" {
				require.ErrorContains(t, tt.wantedErr, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, wsCheckpt)
			require.DeepEqual(t, tt.checkpt.Root, wsCheckpt.Root, "Roots do not match")
			require.Equal(t, tt.checkpt.Epoch, wsCheckpt.Epoch, "Epochs do not match")
		})
	}
}

func genState(t *testing.T, valCount, avgBalance uint64) state.BeaconState {
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)

	validators := make([]*ethpb.Validator, valCount)
	balances := make([]uint64, len(validators))
	for i := uint64(0); i < valCount; i++ {
		validators[i] = &ethpb.Validator{
			PublicKey:             make([]byte, params.BeaconConfig().BLSPubkeyLength),
			CreatorAddress:        make([]byte, 20),
			WithdrawalCredentials: make([]byte, 20),
			EffectiveBalance:      avgBalance * 1e9,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			ActivationHash:        make([]byte, 32),
			ExitHash:              make([]byte, 32),
			WithdrawalOps:         make([]*ethpb.WithdrawalOp, 0),
		}
		balances[i] = validators[i].EffectiveBalance
	}

	require.NoError(t, beaconState.SetValidators(validators))
	require.NoError(t, beaconState.SetBalances(balances))

	return beaconState
}

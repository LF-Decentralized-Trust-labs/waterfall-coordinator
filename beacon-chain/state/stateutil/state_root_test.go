package stateutil_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/interop"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestState_FieldCount(t *testing.T) {
	count := params.BeaconConfig().BeaconStateFieldCount
	typ := reflect.TypeOf(ethpb.BeaconState{})
	numFields := 0
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Name == "state" ||
			typ.Field(i).Name == "sizeCache" ||
			typ.Field(i).Name == "unknownFields" {
			continue
		}
		numFields++
	}
	assert.Equal(t, count, numFields)
}

func BenchmarkHashTreeRoot_Generic_512(b *testing.B) {
	b.StopTimer()
	genesisState := setupGenesisState(b, 512)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := genesisState.HashTreeRoot()
		require.NoError(b, err)
	}
}

func BenchmarkHashTreeRoot_Generic_16384(b *testing.B) {
	b.StopTimer()
	genesisState := setupGenesisState(b, 16384)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := genesisState.HashTreeRoot()
		require.NoError(b, err)
	}
}

func BenchmarkHashTreeRoot_Generic_300000(b *testing.B) {
	b.StopTimer()
	genesisState := setupGenesisState(b, 300000)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := genesisState.HashTreeRoot()
		require.NoError(b, err)
	}
}

func setupGenesisState(tb testing.TB, count uint64) *ethpb.BeaconState {
	genesisState, _, err := interop.GenerateGenesisState(context.Background(), 0, 1)
	require.NoError(tb, err, "Could not generate genesis beacon state")
	for i := uint64(1); i < count; i++ {
		someRoot := [20]byte{}
		someKey := [fieldparams.BLSPubkeyLength]byte{}
		copy(someRoot[:], strconv.Itoa(int(i)))
		copy(someKey[:], strconv.Itoa(int(i)))
		genesisState.Validators = append(genesisState.Validators, &ethpb.Validator{
			PublicKey:                  someKey[:],
			CreatorAddress:             someRoot[:],
			WithdrawalCredentials:      someRoot[:],
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			Slashed:                    false,
			ActivationEligibilityEpoch: 1,
			ActivationEpoch:            1,
			ExitEpoch:                  1,
			WithdrawableEpoch:          1,
			ActivationHash:             (params.BeaconConfig().ZeroHash)[:],
			ExitHash:                   (params.BeaconConfig().ZeroHash)[:],
			WithdrawalOps:              []*ethpb.WithdrawalOp{},
		})
		genesisState.Balances = append(genesisState.Balances, params.BeaconConfig().MaxEffectiveBalance)
	}
	return genesisState
}

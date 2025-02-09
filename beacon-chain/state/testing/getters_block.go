package testing

import (
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

type getStateWithLatestBlockHeader func(*ethpb.BeaconBlockHeader) (state.BeaconState, error)

func VerifyBeaconStateLatestBlockHeader(
	t *testing.T,
	factory getState,
	factoryLBH getStateWithLatestBlockHeader,
) {
	s, err := factory()
	require.NoError(t, err)
	got := s.LatestBlockHeader()
	require.DeepEqual(t, (*ethpb.BeaconBlockHeader)(nil), got)

	want := &ethpb.BeaconBlockHeader{Slot: 100}
	s, err = factoryLBH(want)
	require.NoError(t, err)
	got = s.LatestBlockHeader()
	require.DeepEqual(t, want, got)

	// Test copy does not mutate.
	got.Slot = 101
	require.DeepNotEqual(t, want, got)
}

type getStateWithLBlockRoots func([][]byte) (state.BeaconState, error)

func VerifyBeaconStateBlockRoots(
	t *testing.T,
	factory getState,
	factoryBR getStateWithLBlockRoots,
) {
	s, err := factory()
	require.NoError(t, err)
	got := s.BlockRoots()
	require.DeepEqual(t, ([][]byte)(nil), got)

	want := [][]byte{{'a'}}
	s, err = factoryBR(want)
	require.NoError(t, err)
	got = s.BlockRoots()
	require.DeepEqual(t, want, got)

	// Test copy does not mutate.
	got[0][0] = 'b'
	require.DeepNotEqual(t, want, got)
}

func VerifyBeaconStateBlockRootsNative(
	t *testing.T,
	factory getState,
	factoryBR getStateWithLBlockRoots,
) {
	s, err := factory()
	require.NoError(t, err)
	got := s.BlockRoots()
	want := make([][]byte, fieldparams.BlockRootsLength)
	for i := range want {
		want[i] = make([]byte, 32)
	}
	require.DeepEqual(t, want, got)

	want = make([][]byte, fieldparams.BlockRootsLength)
	for i := range want {
		if i == 0 {
			want[i] = bytesutil.PadTo([]byte{'a'}, 32)
		} else {
			want[i] = make([]byte, 32)
		}

	}
	s, err = factoryBR(want)
	require.NoError(t, err)
	got = s.BlockRoots()
	require.DeepEqual(t, want, got)

	// Test copy does not mutate.
	got[0][0] = 'b'
	require.DeepNotEqual(t, want, got)
}

func VerifyBeaconStateBlockRootAtIndex(
	t *testing.T,
	factory getState,
	factoryBR getStateWithLBlockRoots,
) {
	s, err := factory()
	require.NoError(t, err)
	got, err := s.BlockRootAtIndex(0)
	require.NoError(t, err)
	require.DeepEqual(t, ([]byte)(nil), got)

	r := [][]byte{{'a'}}
	s, err = factoryBR(r)
	require.NoError(t, err)
	got, err = s.BlockRootAtIndex(0)
	require.NoError(t, err)
	want := bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
	require.DeepSSZEqual(t, want, got)
}

func VerifyBeaconStateBlockRootAtIndexNative(
	t *testing.T,
	factory getState,
	factoryBR getStateWithLBlockRoots,
) {
	s, err := factory()
	require.NoError(t, err)
	got, err := s.BlockRootAtIndex(0)
	require.NoError(t, err)
	require.DeepEqual(t, bytesutil.PadTo([]byte{}, 32), got)

	r := [fieldparams.BlockRootsLength][32]byte{{'a'}}
	bRoots := make([][]byte, len(r))
	for i, root := range r {
		tmp := root
		bRoots[i] = tmp[:]
	}
	s, err = factoryBR(bRoots)
	require.NoError(t, err)
	got, err = s.BlockRootAtIndex(0)
	require.NoError(t, err)
	want := bytesutil.PadTo([]byte{'a'}, 32)
	require.DeepSSZEqual(t, want, got)
}

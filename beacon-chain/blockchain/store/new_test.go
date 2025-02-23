package store

import (
	"testing"

	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestNew(t *testing.T) {
	j := &ethpb.Checkpoint{
		Epoch: 0,
		Root:  []byte("hi"),
	}
	f := &ethpb.Checkpoint{
		Epoch: 0,
		Root:  []byte("hello"),
	}
	s := New(j, f)
	require.DeepSSZEqual(t, s.JustifiedCheckpt(), j)
	require.DeepSSZEqual(t, s.BestJustifiedCheckpt(), j)
	require.DeepSSZEqual(t, s.PrevJustifiedCheckpt(), j)
	require.DeepSSZEqual(t, s.FinalizedCheckpt(), f)
	require.DeepSSZEqual(t, s.PrevFinalizedCheckpt(), f)
}

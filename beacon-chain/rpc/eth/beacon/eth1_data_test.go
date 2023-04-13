package beacon

import (
	"context"
	"testing"

	chainMock "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain/testing"
	dbTest "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/rpc/testutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/eth/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestGetEth1Data(t *testing.T) {
	ctx := context.Background()
	db := dbTest.SetupDB(t)

	var st state.BeaconState
	st, _ = util.DeterministicGenesisState(t, 8192)

	t.Run("Head Get Eth1Data", func(t *testing.T) {
		s := Server{
			StateFetcher: &testutil.MockFetcher{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{},
			BeaconDB:    db,
		}

		resp, err := s.GetEth1Data(ctx, &ethpb.StateEth1DataRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotEqual(t, resp.Data, nil)
	})

	t.Run("execution optimistic", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		//root, err := blk.Block.HashTreeRoot()
		root := parentRoot
		//require.NoError(t, err)
		wsb, err := wrapper.WrappedSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(ctx, wsb))
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		s := Server{
			StateFetcher: &testutil.MockFetcher{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{Optimistic: true},
			BeaconDB:    db,
		}
		resp, err := s.GetEth1Data(ctx, &ethpb.StateEth1DataRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
}

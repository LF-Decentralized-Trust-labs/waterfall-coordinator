package debug

import (
	"context"
	"testing"
	"time"

	"github.com/prysmaticlabs/go-bitfield"
	mock "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	dbTest "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/stategen"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestServer_GetBlock(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	b := util.NewBeaconBlock()
	b.Block.Slot = 100
	wsb, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	bs := &Server{
		BeaconDB: db,
	}
	res, err := bs.GetBlock(ctx, &ethpb.BlockRequestByRoot{
		BlockRoot: blockRoot[:],
	})
	require.NoError(t, err)
	wanted, err := b.MarshalSSZ()
	require.NoError(t, err)
	assert.DeepEqual(t, wanted, res.Encoded)

	// Checking for nil block.
	blockRoot = [32]byte{}
	res, err = bs.GetBlock(ctx, &ethpb.BlockRequestByRoot{
		BlockRoot: blockRoot[:],
	})
	require.NoError(t, err)
	assert.DeepEqual(t, []byte{}, res.Encoded)
}

func TestServer_GetAttestationInclusionSlot(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	offset := int64(2 * params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	bs := &Server{
		BeaconDB:           db,
		StateGen:           stategen.New(db),
		GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
	}

	s, _ := util.DeterministicGenesisState(t, 2048)
	tr := [32]byte{'a'}
	require.NoError(t, bs.StateGen.SaveState(ctx, tr, s))
	c, err := helpers.BeaconCommitteeFromState(context.Background(), s, 1, 0)
	require.NoError(t, err)

	a := &ethpb.Attestation{
		Data: &ethpb.AttestationData{
			Target:          &ethpb.Checkpoint{Root: tr[:]},
			Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			BeaconBlockRoot: make([]byte, 32),
			Slot:            1,
		},
		AggregationBits: bitfield.Bitlist{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01},
		Signature:       make([]byte, fieldparams.BLSSignatureLength),
	}
	b := util.NewBeaconBlock()
	b.Block.Slot = 2
	b.Block.Body.Attestations = []*ethpb.Attestation{a}
	wsb, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, bs.BeaconDB.SaveBlock(ctx, wsb))
	res, err := bs.GetInclusionSlot(ctx, &ethpb.InclusionSlotRequest{Slot: 1, Id: uint64(c[0])})
	require.NoError(t, err)
	require.Equal(t, b.Block.Slot, res.Slot)
	res, err = bs.GetInclusionSlot(ctx, &ethpb.InclusionSlotRequest{Slot: 1, Id: 9999999})
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().FarFutureSlot, res.Slot)
}

package blockchain

import (
	"bytes"
	"context"
	"testing"
	"time"

	types "github.com/prysmaticlabs/eth2-types"
	logTest "github.com/sirupsen/logrus/hooks/test"
	mock "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain/testing"
	testDB "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/features"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpbv1 "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/eth/v1"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
)

func TestSaveHead_Same(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	r := [32]byte{'A'}
	service.head = &head{slot: 0, root: r}
	b, err := wrapper.WrappedSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisState(t, 1)
	require.NoError(t, service.saveHead(context.Background(), r, b, st))
	assert.Equal(t, types.Slot(0), service.headSlot(), "Head did not stay the same")
	assert.Equal(t, r, service.headRoot(), "Head did not stay the same")
}

func TestSaveHead_Different(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	util.NewBeaconBlock()
	oldBlock, err := wrapper.WrappedSignedBeaconBlock(
		util.NewBeaconBlock(),
	)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(context.Background(), oldBlock))
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	service.head = &head{
		slot:  0,
		root:  oldRoot,
		block: oldBlock,
	}

	newHeadSignedBlock := util.NewBeaconBlock()
	newHeadSignedBlock.Block.Slot = 1
	newHeadBlock := newHeadSignedBlock.Block

	wsb, err := wrapper.WrappedSignedBeaconBlock(newHeadSignedBlock)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(context.Background(), wsb))
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(context.Background(), &ethpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(context.Background(), headState, newRoot))
	require.NoError(t, service.saveHead(context.Background(), newRoot, wsb, headState))

	assert.Equal(t, types.Slot(1), service.HeadSlot(), "Head did not change")

	cachedRoot, err := service.HeadRoot(context.Background())
	require.NoError(t, err)
	assert.DeepEqual(t, cachedRoot, newRoot[:], "Head did not change")
	assert.DeepEqual(t, newHeadSignedBlock, service.headBlock().Proto(), "Head did not change")
	assert.DeepSSZEqual(t, headState.CloneInnerState(), service.headState(ctx).CloneInnerState(), "Head did not change")
}

func TestSaveHead_Different_Reorg(t *testing.T) {
	ctx := context.Background()
	hook := logTest.NewGlobal()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	oldBlock, err := wrapper.WrappedSignedBeaconBlock(
		util.NewBeaconBlock(),
	)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(context.Background(), oldBlock))
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	service.head = &head{
		slot:  0,
		root:  oldRoot,
		block: oldBlock,
	}

	reorgChainParent := [32]byte{'B'}
	newHeadSignedBlock := util.NewBeaconBlock()
	newHeadSignedBlock.Block.Slot = 1
	newHeadSignedBlock.Block.ParentRoot = reorgChainParent[:]
	newHeadBlock := newHeadSignedBlock.Block

	wsb, err := wrapper.WrappedSignedBeaconBlock(newHeadSignedBlock)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(context.Background(), wsb))
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(context.Background(), &ethpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(context.Background(), headState, newRoot))
	require.NoError(t, service.saveHead(context.Background(), newRoot, wsb, headState))

	assert.Equal(t, types.Slot(1), service.HeadSlot(), "Head did not change")

	cachedRoot, err := service.HeadRoot(context.Background())
	require.NoError(t, err)
	if !bytes.Equal(cachedRoot, newRoot[:]) {
		t.Error("Head did not change")
	}
	assert.DeepEqual(t, newHeadSignedBlock, service.headBlock().Proto(), "Head did not change")
	assert.DeepSSZEqual(t, headState.CloneInnerState(), service.headState(ctx).CloneInnerState(), "Head did not change")
	require.LogsContain(t, hook, "Chain reorg occurred")
}

func TestCacheJustifiedStateBalances_CanCache(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	ctx := context.Background()

	state, _ := util.DeterministicGenesisState(t, 100)
	r := [32]byte{'a'}
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(context.Background(), &ethpb.StateSummary{Root: r[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(context.Background(), state, r))
	balances, err := service.justifiedBalances.get(ctx, r)
	require.NoError(t, err)
	require.DeepEqual(t, balances, state.Balances(), "Incorrect justified balances")
}

func TestUpdateHead_MissingJustifiedRoot(t *testing.T) {
	t.Skip()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	b := util.NewBeaconBlock()
	wsb, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(context.Background(), wsb))
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	service.store.SetJustifiedCheckpt(&ethpb.Checkpoint{Root: r[:]})
	service.store.SetFinalizedCheckpt(&ethpb.Checkpoint{
		Epoch: 0,
		Root:  r[:],
	})
	service.store.SetBestJustifiedCheckpt(&ethpb.Checkpoint{
		Epoch: 0,
		Root:  r[:],
	})
	headRoot, err := service.updateHead(context.Background(), []uint64{})
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisState(t, 1)
	require.NoError(t, service.saveHead(context.Background(), headRoot, wsb, st))
}

func Test_notifyNewHeadEvent(t *testing.T) {
	t.Run("genesis_state_root", func(t *testing.T) {
		bState, _ := util.DeterministicGenesisState(t, 10)
		notifier := &mock.MockStateNotifier{RecordEvents: true}
		srv := &Service{
			cfg: &config{
				StateNotifier: notifier,
			},
			originBlockRoot: [32]byte{1},
		}
		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		err := srv.notifyNewHeadEvent(context.Background(), 1, bState, newHeadStateRoot[:], newHeadRoot[:])
		require.NoError(t, err)
		events := notifier.ReceivedEvents()
		require.Equal(t, 1, len(events))

		eventHead, ok := events[0].Data.(*ethpbv1.EventHead)
		require.Equal(t, true, ok)
		wanted := &ethpbv1.EventHead{
			Slot:                      1,
			Block:                     newHeadRoot[:],
			State:                     newHeadStateRoot[:],
			EpochTransition:           false,
			PreviousDutyDependentRoot: srv.originBlockRoot[:],
			CurrentDutyDependentRoot:  srv.originBlockRoot[:],
		}
		require.DeepSSZEqual(t, wanted, eventHead)
	})
	t.Run("non_genesis_values", func(t *testing.T) {
		bState, _ := util.DeterministicGenesisState(t, 10)
		notifier := &mock.MockStateNotifier{RecordEvents: true}
		genesisRoot := [32]byte{1}
		srv := &Service{
			cfg: &config{
				StateNotifier: notifier,
			},
			originBlockRoot: genesisRoot,
		}
		epoch1Start, err := slots.EpochStart(1)
		require.NoError(t, err)
		epoch2Start, err := slots.EpochStart(1)
		require.NoError(t, err)
		require.NoError(t, bState.SetSlot(epoch1Start))

		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		err = srv.notifyNewHeadEvent(context.Background(), epoch2Start, bState, newHeadStateRoot[:], newHeadRoot[:])
		require.NoError(t, err)
		events := notifier.ReceivedEvents()
		require.Equal(t, 1, len(events))

		eventHead, ok := events[0].Data.(*ethpbv1.EventHead)
		require.Equal(t, true, ok)
		wanted := &ethpbv1.EventHead{
			Slot:                      epoch2Start,
			Block:                     newHeadRoot[:],
			State:                     newHeadStateRoot[:],
			EpochTransition:           true,
			PreviousDutyDependentRoot: genesisRoot[:],
			CurrentDutyDependentRoot:  make([]byte, 32),
		}
		require.DeepSSZEqual(t, wanted, eventHead)
	})
}

func TestSaveOrphanedAtts(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{
		CorrectlyInsertOrphanedAtts: true,
	})
	defer resetCfg()

	genesis, keys := util.DeterministicGenesisState(t, 64)
	b, err := util.GenerateFullBlock(genesis, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now()

	wsb, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb))
	require.NoError(t, service.saveOrphanedAtts(ctx, r))

	require.Equal(t, len(b.Block.Body.Attestations), service.cfg.AttPool.AggregatedAttestationCount())
	savedAtts := service.cfg.AttPool.AggregatedAttestations()
	atts := b.Block.Body.Attestations
	require.DeepSSZEqual(t, atts, savedAtts)
}

func TestSaveOrphanedAtts_CanFilter(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{
		CorrectlyInsertOrphanedAtts: true,
	})
	defer resetCfg()

	genesis, keys := util.DeterministicGenesisState(t, 64)
	b, err := util.GenerateFullBlock(genesis, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SlotsPerEpoch+1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	wsb, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb))
	require.NoError(t, service.saveOrphanedAtts(ctx, r))

	require.Equal(t, 0, service.cfg.AttPool.AggregatedAttestationCount())
	savedAtts := service.cfg.AttPool.AggregatedAttestations()
	atts := b.Block.Body.Attestations
	require.DeepNotSSZEqual(t, atts, savedAtts)
}

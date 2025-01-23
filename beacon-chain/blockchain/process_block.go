package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed"
	statefeed "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	coreTime "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/transition"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db"
	forkchoicetypes "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/forkchoice/types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/features"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/tracing"
	ethpbv1 "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/eth/v1"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/attestation"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"go.opencensus.io/trace"
)

// A custom slot deadline for processing state slots in our cache.
const slotDeadline = 5 * time.Second

// A custom deadline for deposit trie insertion.
const depositDeadline = 20 * time.Second

// This defines size of the upper bound for initial sync block cache.
var initialSyncBlockCacheSize = uint64(2 * params.BeaconConfig().SlotsPerEpoch)

// onBlock is called when a gossip block is received. It runs regular state transition on the block.
// The block's signing root should be computed before calling this method to avoid redundant
// computation in this method and methods it calls into.
//
// Spec pseudocode definition:
//
//	def on_block(store: Store, signed_block: SignedBeaconBlock) -> None:
//	 block = signed_block.message
//	 # Parent block must be known
//	 assert block.parent_root in store.block_states
//	 # Make a copy of the state to avoid mutability issues
//	 pre_state = copy(store.block_states[block.parent_root])
//	 # Blocks cannot be in the future. If they are, their consideration must be delayed until the are in the past.
//	 assert get_current_slot(store) >= block.slot
//
//	 # Check that block is later than the finalized epoch slot (optimization to reduce calls to get_ancestor)
//	 finalized_slot = compute_start_slot_at_epoch(store.finalized_checkpoint.epoch)
//	 assert block.slot > finalized_slot
//	 # Check block is a descendant of the finalized block at the checkpoint finalized slot
//	 assert get_ancestor(store, block.parent_root, finalized_slot) == store.finalized_checkpoint.root
//
//	 # Check the block is valid and compute the post-state
//	 state = pre_state.copy()
//	 state_transition(state, signed_block, True)
//	 # Add new block to the store
//	 store.blocks[hash_tree_root(block)] = block
//	 # Add new state for this block to the store
//	 store.block_states[hash_tree_root(block)] = state
//
//	 # Update justified checkpoint
//	 if state.current_justified_checkpoint.epoch > store.justified_checkpoint.epoch:
//	     if state.current_justified_checkpoint.epoch > store.best_justified_checkpoint.epoch:
//	         store.best_justified_checkpoint = state.current_justified_checkpoint
//	     if should_update_justified_checkpoint(store, state.current_justified_checkpoint):
//	         store.justified_checkpoint = state.current_justified_checkpoint
//
//	 # Update finalized checkpoint
//	 if state.finalized_checkpoint.epoch > store.finalized_checkpoint.epoch:
//	     store.finalized_checkpoint = state.finalized_checkpoint
//
//	     # Potentially update justified if different from store
//	     if store.justified_checkpoint != state.current_justified_checkpoint:
//	         # Update justified if new justified is later than store justified
//	         if state.current_justified_checkpoint.epoch > store.justified_checkpoint.epoch:
//	             store.justified_checkpoint = state.current_justified_checkpoint
//	             return
//
//	         # Update justified if store justified is not in chain with finalized checkpoint
//	         finalized_slot = compute_start_slot_at_epoch(store.finalized_checkpoint.epoch)
//	         ancestor_at_finalized_slot = get_ancestor(store, store.justified_checkpoint.root, finalized_slot)
//	         if ancestor_at_finalized_slot != store.finalized_checkpoint.root:
//	             store.justified_checkpoint = state.current_justified_checkpoint
func (s *Service) onBlock(ctx context.Context, signed block.SignedBeaconBlock, blockRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.onBlock")
	//required to support rewards and penalties state operations
	ctx = context.WithValue(ctx, params.BeaconConfig().CtxBlockFetcherKey, db.BlockInfoFetcherFunc(s.cfg.BeaconDB))
	defer span.End()

	rmBlRootProc := true
	s.setBlRootProcessing(blockRoot)
	defer func() {
		if rmBlRootProc {
			s.rmBlRootProcessing(blockRoot)
		}
	}()

	s.onBlockMu.Lock()
	defer s.onBlockMu.Unlock()

	if err := helpers.BeaconBlockIsNil(signed); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"curSlot": slots.CurrentSlot(uint64(s.genesisTime.Unix())),
		}).Error("onBlock error")
		return err
	}
	b := signed.Block()

	defer func(start time.Time, curSlot types.Slot) {
		log.WithField(
			"elapsed", time.Since(start),
		).WithField(
			"curSlot", curSlot,
		).WithField(
			"blSlot", signed.Block().Slot(),
		).WithFields(logrus.Fields{
			"parentRoot": fmt.Sprintf("%#x", signed.Block().ParentRoot()),
			"root":       fmt.Sprintf("%#x", blockRoot),
		}).Info("onBlock: end")
	}(time.Now(), slots.CurrentSlot(uint64(s.genesisTime.Unix())))

	log.WithFields(logrus.Fields{
		"blSlot":            signed.Block().Slot(),
		"root":              fmt.Sprintf("%#x", blockRoot),
		"parentRoot":        fmt.Sprintf("%#x", signed.Block().ParentRoot()),
		"delegateFork":      params.BeaconConfig().IsDelegatingStakeSlot(signed.Block().Slot()),
		"gwatSynchronizing": s.IsGwatSynchronizing(),
		"\u2692":            version.BuildId,
	}).Info("onBlock: start")

	if len(signed.Block().Body().Withdrawals()) > 0 {
		for i, itm := range signed.Block().Body().Withdrawals() {
			log.WithFields(logrus.Fields{
				"i":              i,
				"slot":           signed.Block().Slot(),
				"Amount":         fmt.Sprintf("%d", itm.Amount),
				"Epoch":          fmt.Sprintf("%d", itm.Epoch),
				"InitTxHash":     fmt.Sprintf("%#x", itm.InitTxHash),
				"PublicKey":      fmt.Sprintf("%#x", itm.PublicKey),
				"ValidatorIndex": fmt.Sprintf("%d", itm.ValidatorIndex),
			}).Info("onBlock: withdrawal")

			if !s.IsGwatSynchronizing() && !s.isSynchronizing() && params.BeaconConfig().IsDelegatingStakeSlot(signed.Block().Slot()) {
				if err := s.cfg.WithdrawalPool.Verify(itm); err != nil {
					log.WithError(err).WithFields(logrus.Fields{
						"i":              i,
						"slot":           signed.Block().Slot(),
						"Amount":         fmt.Sprintf("%d", itm.Amount),
						"Epoch":          fmt.Sprintf("%d", itm.Epoch),
						"InitTxHash":     fmt.Sprintf("%#x", itm.InitTxHash),
						"PublicKey":      fmt.Sprintf("%#x", itm.PublicKey),
						"ValidatorIndex": fmt.Sprintf("%d", itm.ValidatorIndex),
					}).Error("onBlock: withdrawal")
					return err
				}
			}
		}
	}

	if len(signed.Block().Body().VoluntaryExits()) > 0 {
		for i, itm := range signed.Block().Body().VoluntaryExits() {
			log.WithFields(logrus.Fields{
				"i":              i,
				"slot":           signed.Block().Slot(),
				"Epoch":          fmt.Sprintf("%d", itm.Epoch),
				"InitTxHash":     fmt.Sprintf("%#x", itm.InitTxHash),
				"ValidatorIndex": fmt.Sprintf("%d", itm.ValidatorIndex),
			}).Info("onBlock: exit")

			if !s.IsGwatSynchronizing() && !s.isSynchronizing() && params.BeaconConfig().IsDelegatingStakeSlot(signed.Block().Slot()) {
				if err := s.cfg.ExitPool.Verify(itm); err != nil {
					log.WithError(err).WithFields(logrus.Fields{
						"i":              i,
						"slot":           signed.Block().Slot(),
						"Epoch":          fmt.Sprintf("%d", itm.Epoch),
						"InitTxHash":     fmt.Sprintf("%#x", itm.InitTxHash),
						"ValidatorIndex": fmt.Sprintf("%d", itm.ValidatorIndex),
					}).Error("onBlock: exit")
					return err
				}
			}
		}
	}

	preState, err := s.getBlockPreState(ctx, b)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return err
	}

	postState, err := transition.ExecuteStateTransition(ctx, preState, signed)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return err
	}

	//validate limitation of all spines count
	if spinesCount := helpers.CountUniqSpines(postState); spinesCount > params.BeaconConfig().AllSpinesLimit {
		err = errAllSpinesLimitExceeded
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot":     signed.Block().Slot(),
			"spinesCount":    spinesCount,
			"AllSpinesLimit": params.BeaconConfig().AllSpinesLimit,
		}).Error("onBlock error")
		return err
	}

	log.WithError(err).WithFields(logrus.Fields{
		"block.slot": signed.Block().Slot(),
		//"postBlockVoting": helpers.PrintBlockVotingArr(postState.BlockVoting()),
		"postBlockVoting": len(postState.BlockVoting()),
	}).Info("onBlock: State transition executed")

	if err := s.insertBlockAndAttestationsToForkChoiceStore(ctx, signed.Block(), blockRoot, postState); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return errors.Wrapf(err, "could not insert block %d to fork choice store", signed.Block().Slot())
	}

	// We add a proposer score boost to fork choice for the block root if applicable, right after
	// running a successful state transition for the block.
	secondsIntoSlot := uint64(time.Since(s.genesisTime).Seconds()) % params.BeaconConfig().SecondsPerSlot
	if err := s.cfg.ForkChoiceStore.BoostProposerRoot(ctx, &forkchoicetypes.ProposerBoostRootArgs{
		BlockRoot:       blockRoot,
		BlockSlot:       signed.Block().Slot(),
		CurrentSlot:     slots.SinceGenesis(s.genesisTime),
		SecondsIntoSlot: secondsIntoSlot,
	}); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return err
	}

	log.WithError(err).WithError(err).WithFields(logrus.Fields{
		"block.slot": signed.Block().Slot(),
	}).Info("onBlock: fc boost proposer root")

	if err := s.savePostStateInfo(ctx, blockRoot, signed, postState, false /* reg sync */); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return err
	}
	s.rmBlRootProcessing(blockRoot)
	rmBlRootProc = false

	log.WithError(err).WithFields(logrus.Fields{
		"block.slot": signed.Block().Slot(),
		//"postBlockVoting": helpers.PrintBlockVotingArr(postState.BlockVoting()),
		"postState.Finalization": gwatCommon.HashArrayFromBytes(postState.SpineData().Finalization),
	}).Info("onBlock: save post state")

	// If slasher is configured, forward the attestations in the block via
	// an event feed for processing.
	if features.Get().EnableSlasher {
		// Feed the indexed attestation to slasher if enabled. This action
		// is done in the background to avoid adding more load to this critical code path.
		go func() {
			// Using a different context to prevent timeouts as this operation can be expensive
			// and we want to avoid affecting the critical code path.
			ctx := context.TODO()
			for _, att := range signed.Block().Body().Attestations() {
				committee, err := helpers.BeaconCommitteeFromState(ctx, preState, att.Data.Slot, att.Data.CommitteeIndex)
				if err != nil {
					log.WithError(err).Error("Could not get attestation committee")
					tracing.AnnotateError(span, err)
					return
				}
				indexedAtt, err := attestation.ConvertToIndexed(ctx, att, committee)
				if err != nil {
					log.WithError(err).Error("Could not convert to indexed attestation")
					tracing.AnnotateError(span, err)
					return
				}
				s.cfg.SlasherAttestationsFeed.Send(indexedAtt)
			}
		}()
	}

	// Update justified check point.
	justified := s.store.JustifiedCheckpt()
	if justified == nil {
		log.WithError(errNilJustifiedInStore).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return errNilJustifiedInStore
	}
	currJustifiedEpoch := justified.Epoch
	if postState.CurrentJustifiedCheckpoint().Epoch > currJustifiedEpoch {
		if err := s.updateJustified(ctx, postState); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"block.slot": signed.Block().Slot(),
			}).Error("onBlock error")
			return err
		}
	}

	finalized := s.store.FinalizedCheckpt()
	if finalized == nil {
		log.WithError(errNilFinalizedInStore).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return errNilFinalizedInStore
	}
	newFinalized := postState.FinalizedCheckpointEpoch() > finalized.Epoch
	if newFinalized {
		s.store.SetPrevFinalizedCheckpt(finalized)
		s.store.SetFinalizedCheckpt(postState.FinalizedCheckpoint())
		s.store.SetPrevJustifiedCheckpt(justified)
		s.store.SetJustifiedCheckpt(postState.CurrentJustifiedCheckpoint())
	}

	balances, err := s.justifiedBalances.get(ctx, bytesutil.ToBytes32(justified.Root))
	if err != nil {
		msg := fmt.Sprintf("could not read balances for state w/ justified checkpoint %#x", justified.Root)
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
			"message":    msg,
		}).Error("onBlock error")
		return errors.Wrap(err, msg)
	}
	headRoot, err := s.updateHead(ctx, balances)

	log.WithError(err).WithFields(logrus.Fields{
		"block.slot": signed.Block().Slot(),
		"headRoot":   fmt.Sprintf("%#x", headRoot),
		//"postBlockVoting": helpers.PrintBlockVotingArr(postState.BlockVoting()),
		"headState.Finalization": gwatCommon.HashArrayFromBytes(s.head.state.SpineData().Finalization),
	}).Info("onBlock: update head")

	if err != nil {
		log.WithError(err).Warn("Could not update head")
	}
	headBlock, err := s.cfg.BeaconDB.Block(ctx, headRoot)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return err
	}
	headState, err := s.cfg.StateGen.StateByRoot(ctx, headRoot)

	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return err
	}

	log.WithFields(logrus.Fields{
		"block.slot":             signed.Block().Slot(),
		"headRoot":               fmt.Sprintf("%#x", headRoot),
		"headState.Finalization": gwatCommon.HashArrayFromBytes(headState.SpineData().Finalization),
	}).Info("onBlock: get state by root")

	if err := s.saveHead(ctx, headRoot, headBlock, headState); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error could not save head")
		return errors.Wrap(err, "could not save head")
	}

	log.WithError(err).WithFields(logrus.Fields{
		"block.slot": signed.Block().Slot(),
		//"postBlockVoting": helpers.PrintBlockVotingArr(postState.BlockVoting()),
		"headState.Finalization": gwatCommon.HashArrayFromBytes(s.head.state.SpineData().Finalization),
	}).Info("onBlock: save head")

	log.WithFields(logrus.Fields{
		"IsSynced":                 s.IsSynced(),
		"CurrentSlot == BlockSlot": s.CurrentSlot() == signed.Block().Slot(),
		"CurrentSlot":              s.CurrentSlot(),
		"BlockSlot":                signed.Block().Slot(),
	}).Info("On block sync status")

	if err := s.pruneCanonicalAttsFromPool(ctx, blockRoot, signed); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Error("onBlock error")
		return err
	}

	// Send notification of the processed block to the state feed.
	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.BlockProcessed,
		Data: &statefeed.BlockProcessedData{
			Slot:        signed.Block().Slot(),
			BlockRoot:   blockRoot,
			SignedBlock: signed,
			Verified:    true,
			InitialSync: false,
		},
	})

	// Updating next slot state cache can happen in the background. It shouldn't block rest of the process.
	go func() {
		// Use a custom deadline here, since this method runs asynchronously.
		// We ignore the parent method's context and instead create a new one
		// with a custom deadline, therefore using the background context instead.
		slotCtx, cancel := context.WithTimeout(context.Background(), slotDeadline)
		defer cancel()
		if err := transition.UpdateNextSlotCache(slotCtx, blockRoot[:], postState); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"block.slot": signed.Block().Slot(),
			}).Error("onBlock error: could not update next slot state cache")
		}
	}()

	// Save justified check point to db.
	if postState.CurrentJustifiedCheckpoint().Epoch > currJustifiedEpoch {
		if err := s.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, postState.CurrentJustifiedCheckpoint()); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"block.slot": signed.Block().Slot(),
			}).Error("onBlock error")
			return err
		}
	}

	// Update finalized check point.
	if newFinalized {
		if err := s.updateFinalized(ctx, postState.FinalizedCheckpoint()); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"block.slot": signed.Block().Slot(),
			}).Error("onBlock error")
			return err
		}
		fRoot := bytesutil.ToBytes32(postState.FinalizedCheckpoint().Root)

		log.WithError(err).WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Info("onBlock: update finalized")

		if err := s.cfg.ForkChoiceStore.Prune(ctx, fRoot); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"block.slot": signed.Block().Slot(),
			}).Error("onBlock error could not prune proto array fork choice nodes")
			return errors.Wrap(err, "could not prune proto array fork choice nodes")
		}

		log.WithError(err).WithError(err).WithFields(logrus.Fields{
			"block.slot": signed.Block().Slot(),
		}).Info("onBlock: fc prune")

		isOptimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(fRoot)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"block.slot": signed.Block().Slot(),
			}).Error("onBlock error could not check if node is optimistically synced")
			return errors.Wrap(err, "could not check if node is optimistically synced")
		}

		log.WithError(err).WithError(err).WithFields(logrus.Fields{
			"block.slot":   signed.Block().Slot(),
			"isOptimistic": isOptimistic,
		}).Info("onBlock: fc is optimistic")

		go func() {
			// Send an event regarding the new finalized checkpoint over a common event feed.
			s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
				Type: statefeed.FinalizedCheckpoint,
				Data: &ethpbv1.EventFinalizedCheckpoint{
					Epoch:               postState.FinalizedCheckpoint().Epoch,
					Block:               postState.FinalizedCheckpoint().Root,
					State:               signed.Block().StateRoot(),
					ExecutionOptimistic: isOptimistic,
					FinalizationSlot:    signed.Block().Slot(),
				},
			})

			// Use a custom deadline here, since this method runs asynchronously.
			// We ignore the parent method's context and instead create a new one
			// with a custom deadline, therefore using the background context instead.
			depCtx, cancel := context.WithTimeout(context.Background(), depositDeadline)
			defer cancel()
			if err := s.insertFinalizedDeposits(depCtx, fRoot); err != nil {
				log.WithError(err).Error("Could not insert finalized deposits.")
			}
		}()
	}

	defer reportAttestationInclusion(b)

	return s.handleEpochBoundary(ctx, postState)
}

func (s *Service) onBlockBatch(ctx context.Context, blks []block.SignedBeaconBlock, blockRoots [][32]byte) ([]*ethpb.Checkpoint, []*ethpb.Checkpoint, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.onBlockBatch")
	//required to support rewards and penalties state operations
	ctx = context.WithValue(ctx, params.BeaconConfig().CtxBlockFetcherKey, BatchHandlerBlockInfoFetcherFunc(s.cfg.BeaconDB, blks, blockRoots))
	defer span.End()

	if len(blks) == 0 || len(blockRoots) == 0 {
		log.WithFields(logrus.Fields{
			"len(blks) == 0":       len(blks) == 0,
			"len(blockRoots) == 0": len(blockRoots) == 0,
		}).Error("Block batch handling error")
		return nil, nil, errors.New("no blocks provided")
	}

	if len(blks) != len(blockRoots) {
		log.WithError(errWrongBlockCount).WithFields(logrus.Fields{
			"len(blks) != len(blockRoots)": len(blks) != len(blockRoots),
		}).Error("Block batch handling error")
		return nil, nil, errWrongBlockCount
	}

	if err := helpers.BeaconBlockIsNil(blks[0]); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"blks[0]": blks[0],
		}).Error("Block batch handling error")
		return nil, nil, err
	}
	b := blks[0].Block()

	// Retrieve incoming block's pre state.
	if err := s.verifyBlkPreState(ctx, b); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"verifyBlkPreState()": "fail",
		}).Error("Block batch handling error")
		return nil, nil, err
	}
	preState, err := s.cfg.StateGen.StateByRootInitialSync(ctx, bytesutil.ToBytes32(b.ParentRoot()))
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"StateByRootInitialSync": "fail",
		}).Error("Block batch handling error")
		return nil, nil, err
	}
	if preState == nil || preState.IsNil() {
		log.WithError(fmt.Errorf("nil pre state for slot %d", b.Slot())).WithFields(logrus.Fields{
			"preState == nil || preState.IsNil()": preState == nil || preState.IsNil(),
		}).Error("Block batch handling error")
		return nil, nil, fmt.Errorf("nil pre state for slot %d", b.Slot())
	}

	stSpineData := make([]*ethpb.SpineData, len(blks))
	jCheckpoints := make([]*ethpb.Checkpoint, len(blks))
	fCheckpoints := make([]*ethpb.Checkpoint, len(blks))
	sigSet := &bls.SignatureBatch{
		Signatures:   [][]byte{},
		PublicKeys:   []bls.PublicKey{},
		Messages:     [][32]byte{},
		Descriptions: []string{},
	}
	var set *bls.SignatureBatch
	boundaries := make(map[[32]byte]state.BeaconState)
	for i, b := range blks {

		set, preState, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx, preState, b)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"ExecuteStateTransitionNoVerifyAnySig": "fail",
			}).Error("Block batch handling error")
			return nil, nil, err
		}
		stSpineData[i] = preState.SpineData()
		// Save potential boundary states.
		if slots.IsEpochStart(preState.Slot()) {
			boundaries[blockRoots[i]] = preState.Copy()
		}
		jCheckpoints[i] = preState.CurrentJustifiedCheckpoint()
		fCheckpoints[i] = preState.FinalizedCheckpoint()

		sigSet.Join(set)
	}
	verify, err := sigSet.Verify()
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"Verify()": "fail",
			"verify":   verify,
		}).Error("Block batch handling error")
		return nil, nil, err
	}
	if !verify {
		log.WithError(errors.New("batch block signature verification failed")).WithFields(logrus.Fields{
			"Verify()": "fail",
			"verify":   verify,
		}).Error("Block batch handling error")
		return nil, nil, errors.New("batch block signature verification failed")
	}

	// blocks have been verified, add them to forkchoice and call the engine
	for i, b := range blks {
		s.saveInitSyncBlock(blockRoots[i], b)

		if err = s.insertBlockToForkChoiceStore(ctx, b.Block(), blockRoots[i], fCheckpoints[i], jCheckpoints[i], stSpineData[i]); err != nil {
			return nil, nil, err
		}
	}

	for r, st := range boundaries {
		if err := s.cfg.StateGen.SaveState(ctx, r, st); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"s.cfg.StateGen.SaveState:boundaries": "fail",
			}).Error("Block batch handling error")
			return nil, nil, err
		}
	}
	// Also saves the last post state which to be used as pre state for the next batch.
	lastB := blks[len(blks)-1]
	lastBR := blockRoots[len(blockRoots)-1]
	if err := s.cfg.StateGen.SaveState(ctx, lastBR, preState); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"StateByRootInitialSync:lastBR": "fail",
		}).Error("Block batch handling error")
		return nil, nil, err
	}
	if err := s.saveHeadNoDB(ctx, lastB, lastBR, preState); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"saveHeadNoDB": "fail",
		}).Error("Block batch handling error")
		return nil, nil, err
	}

	lastFinCp := fCheckpoints[len(fCheckpoints)-1]

	err = s.ForkChoicer().Prune(ctx, [32]byte(lastFinCp.Root))
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"checkpointRoot": lastFinCp.Root,
		}).Error("ForkChoicer cache prune error")
	}

	return fCheckpoints, jCheckpoints, nil
}

// BatchHandlerBlockInfoFetcherFunc provides access to blocks info
// including stored in db over context for state transition while initial-sync.
func BatchHandlerBlockInfoFetcherFunc(dbRo db.ReadOnlyDatabase, blks []block.SignedBeaconBlock, blockRoots [][32]byte) params.CtxBlockFetcher {
	return func(ctx context.Context, blockRoot [32]byte) (types.ValidatorIndex, types.Slot, uint64, error) {
		var blk block.SignedBeaconBlock
		var err error

		for i, root := range blockRoots {
			if blockRoot == root {
				blk = blks[i]
				break
			}
		}
		// if not found in batch
		if blk == nil {
			blk, err = dbRo.Block(ctx, blockRoot)
			if err != nil {
				return 0, 0, 0, err
			}
		}
		if blk == nil {
			return 0, 0, 0, db.ErrNotFound
		}

		votesIncluded := uint64(0)
		for _, att := range blk.Block().Body().Attestations() {
			votesIncluded += att.AggregationBits.Count()
		}

		return blk.Block().ProposerIndex(), blk.Block().Slot(), votesIncluded, nil
	}
}

// handles a block after the block's batch has been verified, where we can save blocks
// their state summaries and split them off to relative hot/cold storage.
func (s *Service) handleBlockAfterBatchVerify(ctx context.Context, signed block.SignedBeaconBlock,
	blockRoot [32]byte, fCheckpoint, jCheckpoint *ethpb.Checkpoint) error {

	if err := s.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{
		Slot: signed.Block().Slot(),
		Root: blockRoot[:],
	}); err != nil {
		return err
	}

	// Rate limit how many blocks (2 epochs worth of blocks) a node keeps in the memory.
	if uint64(len(s.getInitSyncBlocks())) > initialSyncBlockCacheSize {
		if err := s.cfg.BeaconDB.SaveBlocks(ctx, s.getInitSyncBlocks()); err != nil {
			return err
		}
		s.clearInitSyncBlocks()
	}

	justified := s.store.JustifiedCheckpt()
	if justified == nil {
		return errNilJustifiedInStore
	}
	if jCheckpoint.Epoch > justified.Epoch {
		if err := s.updateJustifiedInitSync(ctx, jCheckpoint); err != nil {
			return err
		}
	}

	finalized := s.store.FinalizedCheckpt()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	// Update finalized check point. Prune the block cache and helper caches on every new finalized epoch.
	if fCheckpoint.Epoch > finalized.Epoch {
		if err := s.updateFinalized(ctx, fCheckpoint); err != nil {
			return err
		}
		s.store.SetPrevFinalizedCheckpt(finalized)
		s.store.SetFinalizedCheckpt(fCheckpoint)
	}
	// Deprecated
	////create gwat synchronization params
	//if currEpoch := slots.ToEpoch(signed.Block().Slot()); currEpoch > s.store.LastEpoch() {
	//	if err := s.saveGwatSyncState(ctx, blockRoot); err != nil {
	//		return err
	//	}
	//	s.store.SetLastEpoch(currEpoch)
	//}
	return nil
}

// Epoch boundary bookkeeping such as logging epoch summaries.
func (s *Service) handleEpochBoundary(ctx context.Context, postState state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.handleEpochBoundary")
	defer span.End()

	if postState.Slot()+1 == s.nextEpochBoundarySlot {
		// Update caches for the next epoch at epoch boundary slot - 1.
		if err := helpers.UpdateCommitteeCache(postState, coreTime.NextEpoch(postState)); err != nil {
			return err
		}
		copied := postState.Copy()
		copied, err := transition.ProcessSlots(ctx, copied, copied.Slot()+1)
		if err != nil {
			return err
		}
		if err := helpers.UpdateProposerIndicesInCache(ctx, copied); err != nil {
			return err
		}
	} else if postState.Slot() >= s.nextEpochBoundarySlot {
		if err := reportEpochMetrics(ctx, postState, s.head.state); err != nil {
			return err
		}
		var err error
		s.nextEpochBoundarySlot, err = slots.EpochStart(coreTime.NextEpoch(postState))
		if err != nil {
			return err
		}

		// Update caches at epoch boundary slot.
		// The following updates have short cut to return nil cheaply if fulfilled during boundary slot - 1.
		if err := helpers.UpdateCommitteeCache(postState, coreTime.CurrentEpoch(postState)); err != nil {
			return err
		}
		if err := helpers.UpdateProposerIndicesInCache(ctx, postState); err != nil {
			return err
		}
	}
	return nil
}

// This feeds in the block and block's attestations to fork choice store. It's allows fork choice store
// to gain information on the most current chain.
func (s *Service) insertBlockAndAttestationsToForkChoiceStore(
	ctx context.Context,
	blk block.BeaconBlock,
	root [32]byte,
	st state.BeaconState,
) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.insertBlockAndAttestationsToForkChoiceStore")
	defer span.End()

	fCheckpoint := st.FinalizedCheckpoint()
	jCheckpoint := st.CurrentJustifiedCheckpoint()
	if err := s.insertBlockToForkChoiceStore(ctx, blk, root, fCheckpoint, jCheckpoint, st.SpineData()); err != nil {
		return err
	}
	// Feed in block's attestations to fork choice store.
	for _, a := range blk.Body().Attestations() {
		committee, err := helpers.BeaconCommitteeFromState(ctx, st, a.Data.Slot, a.Data.CommitteeIndex)
		if err != nil {
			return err
		}
		indices, err := attestation.AttestingIndices(a.AggregationBits, committee)
		if err != nil {
			return err
		}
		s.cfg.ForkChoiceStore.ProcessAttestation(ctx, indices, bytesutil.ToBytes32(a.Data.BeaconBlockRoot), a.Data.Target.Epoch)
	}
	return nil
}

func (s *Service) insertBlockToForkChoiceStore(
	ctx context.Context,
	blk block.BeaconBlock,
	root [32]byte,
	fCheckpoint, jCheckpoint *ethpb.Checkpoint,
	spineData *ethpb.SpineData,
) error {
	if err := s.fillInForkChoiceMissingBlocks(ctx, blk, fCheckpoint, jCheckpoint); err != nil {
		return err
	}

	// Feed in block to fork choice store.
	return s.cfg.ForkChoiceStore.InsertOptimisticBlock(ctx,
		blk.Slot(), root, bytesutil.ToBytes32(blk.ParentRoot()),
		jCheckpoint.Epoch,
		fCheckpoint.Epoch,
		jCheckpoint.Root,
		fCheckpoint.Root,
		spineData,
	)
}

// This saves post state info to DB or cache. This also saves post state info to fork choice store.
// Post state info consists of processed block and state. Do not call this method unless the block and state are verified.
func (s *Service) savePostStateInfo(ctx context.Context, r [32]byte, b block.SignedBeaconBlock, st state.BeaconState, initSync bool) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.savePostStateInfo")
	defer span.End()

	log.WithFields(logrus.Fields{
		"slot":     b.Block().Slot(),
		"initSync": initSync,
	}).Info("onBlock: savePostStateInfo 000")
	tstart := time.Now()

	if initSync {
		s.saveInitSyncBlock(r, b)
	} else if err := s.cfg.BeaconDB.SaveBlock(ctx, b); err != nil {
		return errors.Wrapf(err, "could not save block from slot %d", b.Block().Slot())
	}

	log.WithFields(logrus.Fields{
		"slot":     b.Block().Slot(),
		"initSync": initSync,
		"elapsed":  time.Since(tstart),
	}).Info("onBlock: savePostStateInfo block")
	tstart = time.Now()

	if err := s.cfg.StateGen.SaveState(ctx, r, st); err != nil {
		return errors.Wrap(err, "could not save state")
	}

	log.WithFields(logrus.Fields{
		"slot":     b.Block().Slot(),
		"initSync": initSync,
		"elapsed":  time.Since(tstart),
	}).Info("onBlock: savePostStateInfo state")

	return nil
}

// This removes the attestations from the mem pool. It will only remove the attestations if input root `r` is canonical,
// meaning the block `b` is part of the canonical chain.
func (s *Service) pruneCanonicalAttsFromPool(ctx context.Context, r [32]byte, b block.SignedBeaconBlock) error {
	if !features.Get().CorrectlyPruneCanonicalAtts {
		return nil
	}

	canonical, err := s.IsCanonical(ctx, r)
	if err != nil {
		return err
	}
	if !canonical {
		return nil
	}

	atts := b.Block().Body().Attestations()
	for _, att := range atts {
		if helpers.IsAggregated(att) {
			if err := s.cfg.AttPool.DeleteAggregatedAttestation(att); err != nil {
				return err
			}
		} else {
			if err := s.cfg.AttPool.DeleteUnaggregatedAttestation(att); err != nil {
				return err
			}
		}
	}
	return nil
}

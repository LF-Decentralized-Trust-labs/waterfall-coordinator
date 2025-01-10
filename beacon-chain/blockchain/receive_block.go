package blockchain

import (
	"context"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed"
	statefeed "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/tracing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	"go.opencensus.io/trace"
)

// This defines how many epochs since finality the run time will begin to save hot state on to the DB.
var epochsSinceFinalitySaveHotStateDB = types.Epoch(100)

var ErrBlockIsProcessing = errors.New("block is processing")

// SyncBlockchainService defines the interface for interaction with block chain service
// for sync service.
type SyncBlockchainService interface {
	BlockReceiver
	HeadFetcher
	FinalizationFetcher
	ForkFetcher
	AttestationReceiver
	TimeFetcher
	GenesisFetcher
	CanonicalFetcher
}

// InitialSyncBlockchainService defines the interface for interaction with block chain service
// for initial-sync service.
type InitialSyncBlockchainService interface {
	BlockReceiver
	ChainInfoFetcher
	SyncSrv
}

// BlockReceiver interface defines the methods of chain service receive and processing new blocks.
type BlockReceiver interface {
	ReceiveBlock(ctx context.Context, block block.SignedBeaconBlock, blockRoot [32]byte) error
	ReceiveBlockBatch(ctx context.Context, blocks []block.SignedBeaconBlock, blkRoots [][32]byte) error
	HasInitSyncBlock(root [32]byte) bool
	IsBlockRootProcessing(root [32]byte) bool
}

// ReceiveBlock is a function that defines the operations (minus pubsub)
// that are performed on blocks that is received from regular sync service. The operations consists of:
//  1. Validate block, apply state transition and update check points
//  2. Apply fork choice to the processed block
//  3. Save latest head info
func (s *Service) ReceiveBlock(ctx context.Context, block block.SignedBeaconBlock, blockRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.ReceiveBlock")
	defer span.End()
	// ensure that block is not already processing
	if s.isBlockProcessing(block.Block().Slot(), block.Block().ProposerIndex()) {
		return ErrBlockIsProcessing
	}
	s.setBlockProcessing(block.Block().Slot(), block.Block().ProposerIndex())
	defer s.rmBlockProcessing(block.Block().Slot(), block.Block().ProposerIndex())

	receivedTime := time.Now()
	blockCopy := block.Copy()

	// Apply state transition on the new block.
	if err := s.onBlock(ctx, blockCopy, blockRoot); err != nil {
		err := errors.Wrap(err, "could not process block")
		tracing.AnnotateError(span, err)
		return err
	}

	// Handle post block operations such as attestations and exits.
	if err := s.handlePostBlockOperations(blockCopy.Block()); err != nil {
		return err
	}

	// Have we been finalizing? Should we start saving hot states to db?
	if err := s.checkSaveHotStateDB(ctx); err != nil {
		return err
	}

	// Reports on block and fork choice metrics.
	finalized := s.store.FinalizedCheckpt()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	reportSlotMetrics(blockCopy.Block().Slot(), s.HeadSlot(), s.CurrentSlot(), finalized)

	// Log block sync status.
	if err := logBlockSyncStatus(blockCopy.Block(), blockRoot, finalized, receivedTime, uint64(s.genesisTime.Unix())); err != nil {
		return err
	}
	// Log state transition data.
	if err := logStateTransitionData(blockCopy.Block()); err != nil {
		return err
	}

	return nil
}

// ReceiveBlockBatch processes the whole block batch at once, assuming the block batch is linear ,transitioning
// the state, performing batch verification of all collected signatures and then performing the appropriate
// actions for a block post-transition.
func (s *Service) ReceiveBlockBatch(ctx context.Context, blocks []block.SignedBeaconBlock, blkRoots [][32]byte) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.ReceiveBlockBatch")
	defer span.End()

	// Apply state transition on the incoming newly received blockCopy without verifying its BLS contents.
	fCheckpoints, jCheckpoints, err := s.onBlockBatch(ctx, blocks, blkRoots)
	if err != nil {
		err := errors.Wrap(err, "could not process block in batch")
		tracing.AnnotateError(span, err)
		return err
	}

	for i, b := range blocks {
		blockCopy := b.Copy()
		if err = s.handleBlockAfterBatchVerify(ctx, blockCopy, blkRoots[i], fCheckpoints[i], jCheckpoints[i]); err != nil {
			tracing.AnnotateError(span, err)
			return err
		}
		// Send notification of the processed block to the state feed.
		s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{
				Slot:        blockCopy.Block().Slot(),
				BlockRoot:   blkRoots[i],
				SignedBlock: blockCopy,
				Verified:    true,
				InitialSync: true,
			},
		})

		// Reports on blockCopy and fork choice metrics.
		finalized := s.store.FinalizedCheckpt()
		if finalized == nil {
			return errNilFinalizedInStore
		}
		reportSlotMetrics(blockCopy.Block().Slot(), s.HeadSlot(), s.CurrentSlot(), finalized)
	}

	if err := s.cfg.BeaconDB.SaveBlocks(ctx, s.getInitSyncBlocks()); err != nil {
		return err
	}
	finalized := s.store.FinalizedCheckpt()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	if err := s.wsVerifier.VerifyWeakSubjectivity(s.ctx, finalized.Epoch); err != nil {
		// log.Fatalf will prevent defer from being called
		span.End()
		// Exit run time if the node failed to verify weak subjectivity checkpoint.
		log.Fatalf("Could not verify weak subjectivity checkpoint: %v", err)
	}

	// run parallel gwat sync
	go s.initParallelGwatSync()
	return nil
}

// HasInitSyncBlock returns true if the block of the input root exists in initial sync blocks cache.
func (s *Service) HasInitSyncBlock(root [32]byte) bool {
	return s.hasInitSyncBlock(root)
}

func (s *Service) handlePostBlockOperations(b block.BeaconBlock) error {
	// Delete the processed block attestations from attestation pool.
	if err := s.deletePoolAtts(b.Body().Attestations()); err != nil {
		return err
	}

	// Add block attestations to the fork choice pool to compute head.
	if err := s.cfg.AttPool.SaveBlockAttestations(b.Body().Attestations()); err != nil {
		log.Errorf("Could not save block attestations for fork choice: %v", err)
		return nil
	}
	// Mark block exits as seen so we don't include same ones in future blocks.
	for _, e := range b.Body().VoluntaryExits() {
		s.cfg.ExitPool.MarkIncluded(e)
	}

	// Mark block withdrawals as seen so we don't include same ones in future blocks.
	for _, w := range b.Body().Withdrawals() {
		s.cfg.WithdrawalPool.MarkIncluded(w)
	}

	//  Mark attester slashings as seen so we don't include same ones in future blocks.
	for _, as := range b.Body().AttesterSlashings() {
		s.cfg.SlashingPool.MarkIncludedAttesterSlashing(as)
	}
	return nil
}

// This checks whether it's time to start saving hot state to DB.
// It's time when there's `epochsSinceFinalitySaveHotStateDB` epochs of non-finality.
func (s *Service) checkSaveHotStateDB(ctx context.Context) error {
	currentEpoch := slots.ToEpoch(s.CurrentSlot())
	// Prevent `sinceFinality` going underflow.
	var sinceFinality types.Epoch
	finalized := s.store.FinalizedCheckpt()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	if currentEpoch > finalized.Epoch {
		sinceFinality = currentEpoch - finalized.Epoch
	}

	if sinceFinality >= epochsSinceFinalitySaveHotStateDB {
		s.cfg.StateGen.EnableSaveHotStateToDB(ctx)
		return nil
	}

	return s.cfg.StateGen.DisableSaveHotStateToDB(ctx)
}

// Returns true if the block is processing by method ReceiveBlock.
func (s *Service) isBlockProcessing(slot types.Slot, proposerIdx types.ValidatorIndex) bool {
	s.procBlockLock.RLock()
	defer s.procBlockLock.RUnlock()

	key := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIdx))...)
	_, isProc := s.procBlockCache.Get(string(key))
	return isProc
}

// Set block proposer index and slot as seen for incoming blocks.
func (s *Service) setBlockProcessing(slot types.Slot, proposerIdx types.ValidatorIndex) {
	s.procBlockLock.Lock()
	defer s.procBlockLock.Unlock()
	key := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIdx))...)
	s.procBlockCache.Add(string(key), true)
}

func (s *Service) rmBlockProcessing(slot types.Slot, proposerIdx types.ValidatorIndex) {
	s.procBlockLock.Lock()
	defer s.procBlockLock.Unlock()
	key := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIdx))...)
	s.procBlockCache.Remove(string(key))
}

// IsBlockRootProcessing returns true if block processing otherwise - false.
func (s *Service) IsBlockRootProcessing(root [32]byte) bool {
	_, isProc := s.procBlockCache.Get(root)
	return isProc
}

func (s *Service) setBlRootProcessing(root [32]byte) {
	s.procBlockCache.Add(root, true)
}

func (s *Service) rmBlRootProcessing(root [32]byte) {
	s.procBlockCache.Remove(root)
}

// Package transition implements the whole state transition
// function which consists of per slot, per-epoch transitions.
// It also bootstraps the genesis beacon state for slot 0.
package transition

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/altair"
	b "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/blocks"
	e "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/epoch"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/epoch/precompute"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/execution"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/math"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/tracing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
	"go.opencensus.io/trace"
)

// ExecuteStateTransition defines the procedure for a state transition function.
//
// Note: This method differs from the spec pseudocode as it uses a batch signature verification.
// See: ExecuteStateTransitionNoVerifyAnySig
//
// Spec pseudocode definition:
//
//	def state_transition(state: BeaconState, signed_block: SignedBeaconBlock, validate_result: bool=True) -> None:
//	  block = signed_block.message
//	  # Process slots (including those with no blocks) since block
//	  process_slots(state, block.slot)
//	  # Verify signature
//	  if validate_result:
//	      assert verify_block_signature(state, signed_block)
//	  # Process block
//	  process_block(state, block)
//	  # Verify state root
//	  if validate_result:
//	      assert block.state_root == hash_tree_root(state)
func ExecuteStateTransition(
	ctx context.Context,
	state state.BeaconState,
	signed block.SignedBeaconBlock,
) (state.BeaconState, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := helpers.BeaconBlockIsNil(signed); err != nil {
		return nil, err
	}

	ctx, span := trace.StartSpan(ctx, "core.state.ExecuteStateTransition")
	defer span.End()
	var err error

	set, postState, err := ExecuteStateTransitionNoVerifyAnySig(ctx, state, signed)
	if err != nil {
		return nil, errors.Wrap(err, "could not execute state transition")
	}
	valid, err := set.Verify()
	if err != nil {
		return nil, errors.Wrap(err, "could not batch verify signature")
	}
	if !valid {

		//TODO RM tmp log ^^^^^^^^^^
		bSet, err := b.BlockSignatureBatch(state, signed.Block().ProposerIndex(), signed.Signature(), signed.Block().HashTreeRoot)
		if err != nil {
			log.WithError(err).Error("*** ExecuteStateTransition: get set err BLOCK ***")
		}
		aSet, err := b.AttestationSignatureBatch(ctx, state, signed.Block().Body().Attestations())
		if err != nil {
			log.WithError(err).Error("*** ExecuteStateTransition: get set err ATTESTATION ***")
		}
		if bSet != nil {
			valid, err := bSet.Verify()
			log.WithError(err).WithFields(logrus.Fields{
				"valid":                valid,
				"len(bSet.Signatures)": len(bSet.Signatures),
			}).Warn("*** ExecuteStateTransition: signature invalid BLOCK ***")
		} else {
			log.Warn("*** ExecuteStateTransition: signature==nil BLOCK ***")
		}
		if aSet != nil {
			valid, err := aSet.Verify()
			log.WithError(err).WithFields(logrus.Fields{
				"valid":                valid,
				"len(aSet.Signatures)": len(aSet.Signatures),
			}).Warn("*** ExecuteStateTransition: signature invalid ATTESTATION ***")
		} else {
			log.Warn("*** ExecuteStateTransition: signature==nil ATTESTATION ***")
		}
		//TODO RM tmp log ^^^^^^^^^^

		return nil, errors.New("signature in block failed to verify")
	}

	return postState, nil
}

// ProcessSlot happens every slot and focuses on the slot counter and block roots record updates.
// It happens regardless if there's an incoming block or not.
// Spec pseudocode definition:
//
//	def process_slot(state: BeaconState) -> None:
//	  # Cache state root
//	  previous_state_root = hash_tree_root(state)
//	  state.state_roots[state.slot % SLOTS_PER_HISTORICAL_ROOT] = previous_state_root
//	  # Cache latest block header state root
//	  if state.latest_block_header.state_root == Bytes32():
//	      state.latest_block_header.state_root = previous_state_root
//	  # Cache block root
//	  previous_block_root = hash_tree_root(state.latest_block_header)
//	  state.block_roots[state.slot % SLOTS_PER_HISTORICAL_ROOT] = previous_block_root
func ProcessSlot(ctx context.Context, state state.BeaconState) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.ProcessSlot")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("slot", int64(state.Slot()))) // lint:ignore uintcast -- This is OK for tracing.

	prevStateRoot, err := state.HashTreeRoot(ctx)
	if err != nil {
		return nil, err
	}
	if err := state.UpdateStateRootAtIndex(
		uint64(state.Slot()%params.BeaconConfig().SlotsPerHistoricalRoot),
		prevStateRoot,
	); err != nil {
		return nil, err
	}

	zeroHash := params.BeaconConfig().ZeroHash
	// Cache latest block header state root.
	header := state.LatestBlockHeader()
	if header.StateRoot == nil || bytes.Equal(header.StateRoot, zeroHash[:]) {
		header.StateRoot = prevStateRoot[:]
		if err := state.SetLatestBlockHeader(header); err != nil {
			return nil, err
		}
	}
	prevBlockRoot, err := state.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not determine prev block root")
	}
	// Cache the block root.
	if err := state.UpdateBlockRootAtIndex(
		uint64(state.Slot()%params.BeaconConfig().SlotsPerHistoricalRoot),
		prevBlockRoot,
	); err != nil {
		return nil, err
	}
	return state, nil
}

// ProcessSlotsUsingNextSlotCache processes slots by using next slot cache for higher efficiency.
func ProcessSlotsUsingNextSlotCache(
	ctx context.Context,
	parentState state.BeaconState,
	parentRoot []byte,
	slot types.Slot,
) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.ProcessSlotsUsingNextSlotCache")
	defer span.End()

	// Check whether the parent state has been advanced by 1 slot in next slot cache.
	nextSlotState, err := NextSlotState(ctx, parentRoot)
	if err != nil {
		return nil, err
	}
	cachedStateExists := nextSlotState != nil && !nextSlotState.IsNil()
	// If the next slot state is not nil (i.e. cache hit).
	// We replace next slot state with parent state.
	if cachedStateExists {
		parentState = nextSlotState
	}

	// In the event our cached state has advanced our
	// state to the desired slot, we exit early.
	if cachedStateExists && parentState.Slot() == slot {
		return parentState, nil
	}
	// Since next slot cache only advances state by 1 slot,
	// we check if there's more slots that need to process.
	parentState, err = ProcessSlots(ctx, parentState, slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not process slots")
	}
	return parentState, nil
}

// ProcessSlotsIfPossible executes ProcessSlots on the input state when target slot is above the state's slot.
// Otherwise, it returns the input state unchanged.
func ProcessSlotsIfPossible(ctx context.Context, state state.BeaconState, targetSlot types.Slot) (state.BeaconState, error) {
	if targetSlot > state.Slot() {
		return ProcessSlots(ctx, state, targetSlot)
	}
	return state, nil
}

// ProcessSlots process through skip slots and apply epoch transition when it's needed
//
// Spec pseudocode definition:
//
//	def process_slots(state: BeaconState, slot: Slot) -> None:
//	  assert state.slot < slot
//	  while state.slot < slot:
//	      process_slot(state)
//	      # Process epoch on the start slot of the next epoch
//	      if (state.slot + 1) % SLOTS_PER_EPOCH == 0:
//	          process_epoch(state)
//	      state.slot = Slot(state.slot + 1)
func ProcessSlots(ctx context.Context, st state.BeaconState, slot types.Slot) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.ProcessSlots")
	defer span.End()
	if st == nil || st.IsNil() {
		return nil, errors.New("nil state")
	}
	span.AddAttributes(trace.Int64Attribute("slots", int64(slot)-int64(st.Slot()))) // lint:ignore uintcast -- This is OK for tracing.

	// The block must have a higher slot than parent state.
	if st.Slot() >= slot {
		err := fmt.Errorf("expected state.slot %d < slot %d", st.Slot(), slot)
		tracing.AnnotateError(span, err)
		return nil, err
	}

	highestSlot := st.Slot()
	key, err := cacheKey(ctx, st)
	if err != nil {
		return nil, err
	}

	// Restart from cached value, if one exists.
	cachedState, err := SkipSlotCache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if cachedState != nil && !cachedState.IsNil() && cachedState.Slot() < slot {
		highestSlot = cachedState.Slot()
		st = cachedState
	}
	if err := SkipSlotCache.MarkInProgress(key); errors.Is(err, cache.ErrAlreadyInProgress) {
		cachedState, err = SkipSlotCache.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if cachedState != nil && !cachedState.IsNil() && cachedState.Slot() < slot {
			highestSlot = cachedState.Slot()
			st = cachedState
		}
	} else if err != nil {
		return nil, err
	}
	defer func() {
		SkipSlotCache.MarkNotInProgress(key)
	}()

	for st.Slot() < slot {
		if ctx.Err() != nil {
			log.WithError(ctx.Err()).WithFields(logrus.Fields{
				"stSlot": st.Slot(),
				"slot":   slot,
			}).Error("Slot processing: context err")
			tracing.AnnotateError(span, ctx.Err())
			// Cache last best value.
			if highestSlot < st.Slot() {
				SkipSlotCache.Put(ctx, key, st)
			}
			return nil, ctx.Err()
		}
		st, err = ProcessSlot(ctx, st)
		if err != nil {
			tracing.AnnotateError(span, err)
			return nil, errors.Wrap(err, "could not process slot")
		}
		if time.CanProcessEpoch(st) {
			// new epoch
			//check nextSlotCache
			nscState, err := GetNextEpochStateByState(ctx, st)
			if err != nil {
				log.WithError(err).WithFields(logrus.Fields{
					"slot": st.Slot(),
				}).Warn("Transition: process epoch: next slot cache not found")
			}

			if nscState == nil {
				switch st.Version() {
				case version.Phase0:
					st, err = ProcessEpochPrecompute(ctx, st)
					if err != nil {
						tracing.AnnotateError(span, err)
						return nil, errors.Wrap(err, "could not process epoch with optimizations")
					}

				case version.Altair, version.Bellatrix:
					st, err = altair.ProcessEpoch(ctx, st)
					if err != nil {
						tracing.AnnotateError(span, err)
						return nil, errors.Wrap(err, "could not process epoch")
					}

				default:
					return nil, errors.New("beacon state should have a version")
				}
				if err := SetNextEpochCache(ctx, st); err != nil {
					log.WithError(err).WithFields(logrus.Fields{
						"slot": st.Slot(),
					}).Warn("Transition: process epoch: set next slot cache failed")
				}

			} else {
				log.WithFields(logrus.Fields{
					"slot": st.Slot(),
				}).Info("Transition: process epoch: next slot cache success")
				st = nscState
			}
		}
		if err := st.SetSlot(st.Slot() + 1); err != nil {
			tracing.AnnotateError(span, err)
			return nil, errors.Wrap(err, "failed to increment state slot")
		}

		if time.CanUpgradeToAltair(st.Slot()) {
			st, err = altair.UpgradeToAltair(ctx, st)
			if err != nil {
				tracing.AnnotateError(span, err)
				return nil, err
			}
		}

		if time.CanUpgradeToBellatrix(st.Slot()) {
			st, err = execution.UpgradeToBellatrix(ctx, st)
			if err != nil {
				tracing.AnnotateError(span, err)
				return nil, err
			}
		}
	}

	if highestSlot < st.Slot() {
		SkipSlotCache.Put(ctx, key, st)
	}

	return st, nil
}

// VerifyOperationLengths verifies that block operation lengths are valid.
func VerifyOperationLengths(_ context.Context, state state.BeaconState, b block.SignedBeaconBlock) (state.BeaconState, error) {
	if err := helpers.BeaconBlockIsNil(b); err != nil {
		return nil, err
	}
	body := b.Block().Body()

	if uint64(len(body.ProposerSlashings())) > params.BeaconConfig().MaxProposerSlashings {
		return nil, fmt.Errorf(
			"number of proposer slashings (%d) in block body exceeds allowed threshold of %d",
			len(body.ProposerSlashings()),
			params.BeaconConfig().MaxProposerSlashings,
		)
	}

	if uint64(len(body.AttesterSlashings())) > params.BeaconConfig().MaxAttesterSlashings {
		return nil, fmt.Errorf(
			"number of attester slashings (%d) in block body exceeds allowed threshold of %d",
			len(body.AttesterSlashings()),
			params.BeaconConfig().MaxAttesterSlashings,
		)
	}

	if uint64(len(body.Attestations())) > params.BeaconConfig().MaxAttestations {
		return nil, fmt.Errorf(
			"number of attestations (%d) in block body exceeds allowed threshold of %d",
			len(body.Attestations()),
			params.BeaconConfig().MaxAttestations,
		)
	}

	if uint64(len(body.VoluntaryExits())) > params.BeaconConfig().MaxVoluntaryExits {
		return nil, fmt.Errorf(
			"number of voluntary exits (%d) in block body exceeds allowed threshold of %d",
			len(body.VoluntaryExits()),
			params.BeaconConfig().MaxVoluntaryExits,
		)
	}

	if uint64(len(body.Withdrawals())) > params.BeaconConfig().MaxWithdrawals {
		return nil, fmt.Errorf(
			"number of withdrawals requests (%d) in block body exceeds allowed threshold of %d",
			len(body.VoluntaryExits()),
			params.BeaconConfig().MaxWithdrawals,
		)
	}

	eth1Data := state.Eth1Data()
	if eth1Data == nil {
		return nil, errors.New("nil eth1data in state")
	}
	if state.Eth1DepositIndex() > eth1Data.DepositCount {
		return nil, fmt.Errorf("expected state.deposit_index %d <= eth1data.deposit_count %d", state.Eth1DepositIndex(), eth1Data.DepositCount)
	}
	maxDeposits := math.Min(params.BeaconConfig().MaxDeposits, eth1Data.DepositCount-state.Eth1DepositIndex())
	// Verify outstanding deposits are processed up to max number of deposits
	if uint64(len(body.Deposits())) != maxDeposits {
		return nil, fmt.Errorf("incorrect outstanding deposits in block body, wanted: %d, got: %d",
			maxDeposits, len(body.Deposits()))
	}

	return state, nil
}

// ProcessEpochPrecompute describes the per epoch operations that are performed on the beacon state.
// It's optimized by pre computing validator attested info and epoch total/attested balances upfront.
func ProcessEpochPrecompute(ctx context.Context, state state.BeaconState) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.ProcessEpochPrecompute")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("epoch", int64(time.CurrentEpoch(state)))) // lint:ignore uintcast -- This is OK for tracing.

	if state == nil || state.IsNil() {
		return nil, errors.New("nil state")
	}

	preFinRoot := state.FinalizedCheckpoint().GetRoot()
	preJustRoot := state.CurrentJustifiedCheckpoint().GetRoot()

	vp, bp, err := precompute.New(ctx, state)
	if err != nil {
		return nil, err
	}
	vp, bp, err = precompute.ProcessAttestations(ctx, state, vp, bp)
	if err != nil {
		return nil, err
	}

	state, err = precompute.ProcessJustificationAndFinalizationPreCompute(state, bp)
	if err != nil {
		return nil, errors.Wrap(err, "could not process justification")
	}

	log.WithField("state.slot", state.Slot()).Info("process rewards and penalties phase0")

	state, err = precompute.ProcessRewardsAndPenaltiesPrecompute(state, bp, vp, precompute.AttestationsDelta, precompute.ProposersDelta)
	if err != nil {
		return nil, errors.Wrap(err, "could not process rewards and penalties phase0")
	}

	state, err = e.ProcessRegistryUpdates(ctx, state)
	if err != nil {
		return nil, errors.Wrap(err, "could not process registry updates")
	}

	err = precompute.ProcessSlashingsPrecompute(state, bp)
	if err != nil {
		return nil, err
	}

	state, err = e.ProcessFinalUpdates(state)
	if err != nil {
		return nil, errors.Wrap(err, "could not process final updates")
	}

	state, err = helpers.ConsensusUpdateStateSpineFinalization(state, preJustRoot, preFinRoot)
	if err != nil {
		return nil, err
	}

	state, err = helpers.ProcessWithdrawalOps(state, preFinRoot)
	if err != nil {
		return nil, err
	}

	return state, nil
}

package transition

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/altair"
	b "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/blocks"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/transition/interop"
	v "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/validators"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/tracing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
	"go.opencensus.io/trace"
)

// ExecuteStateTransitionNoVerifyAnySig defines the procedure for a state transition function.
// This does not validate any BLS signatures of attestations, block proposer signature, randao signature,
// it is used for performing a state transition as quickly as possible. This function also returns a signature
// set of all signatures not verified, so that they can be stored and verified later.
//
// WARNING: This method does not validate any signatures (i.e. calling `state_transition()` with `validate_result=False`).
// This method also modifies the passed in state.
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
func ExecuteStateTransitionNoVerifyAnySig(
	ctx context.Context,
	bState state.BeaconState,
	signed block.SignedBeaconBlock,
) (*bls.SignatureBatch, state.BeaconState, error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if signed == nil || signed.IsNil() || signed.Block().IsNil() {
		return nil, nil, errors.New("nil block")
	}

	ctx, span := trace.StartSpan(ctx, "core.state.ExecuteStateTransitionNoVerifyAttSigs")
	defer span.End()
	var err error

	interop.WriteBlockToDisk(signed, false /* Has the block failed */)
	interop.WriteStateToDisk(bState)

	bState, err = ProcessSlotsUsingNextSlotCache(ctx, bState, signed.Block().ParentRoot(), signed.Block().Slot())
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not process slots")
	}

	// Execute per block transition.
	set, bState, err := ProcessBlockNoVerifyAnySig(ctx, bState, signed)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not process block")
	}

	// State root validation.
	postStateRoot, err := bState.HashTreeRoot(ctx)
	if err != nil {
		return nil, nil, err
	}

	if !bytes.Equal(postStateRoot[:], signed.Block().StateRoot()) {
		return nil, nil, fmt.Errorf("could not validate state root, wanted: %#x, received: %#x (slot=%d)",
			postStateRoot[:], signed.Block().StateRoot(), signed.Block().Slot())
	}

	return set, bState, nil
}

// CalculateStateRoot defines the procedure for a state transition function.
// This does not validate any BLS signatures in a block, it is used for calculating the
// state root of the state for the block proposer to use.
// This does not modify state.
//
// WARNING: This method does not validate any BLS signatures (i.e. calling `state_transition()` with `validate_result=False`).
// This is used for proposer to compute state root before proposing a new block, and this does not modify state.
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
func CalculateStateRoot(
	ctx context.Context,
	bState state.BeaconState,
	signed block.SignedBeaconBlock,
) ([32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.CalculateStateRoot")
	defer span.End()
	if ctx.Err() != nil {
		tracing.AnnotateError(span, ctx.Err())
		return [32]byte{}, ctx.Err()
	}
	if bState == nil || bState.IsNil() {
		return [32]byte{}, errors.New("nil state")
	}
	if signed == nil || signed.IsNil() || signed.Block().IsNil() {
		return [32]byte{}, errors.New("nil block")
	}

	// Copy state to avoid mutating the state reference.
	bState = bState.Copy()

	// Execute per slots transition.
	var err error
	bState, err = ProcessSlotsUsingNextSlotCache(ctx, bState, signed.Block().ParentRoot(), signed.Block().Slot())
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not process slots")
	}

	// Execute per block transition.
	bState, err = ProcessBlockForStateRoot(ctx, bState, signed)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not process block")
	}

	return bState.HashTreeRoot(ctx)
}

// ProcessBlockNoVerifyAnySig creates a new, modified beacon state by applying block operation
// transformations as defined in the Ethereum Serenity specification. It does not validate
// any block signature except for deposit and slashing signatures. It also returns the relevant
// signature set from all the respective methods.
//
// Spec pseudocode definition:
//
//	def process_block(state: BeaconState, block: BeaconBlock) -> None:
//	  process_block_header(state, block)
//	  process_randao(state, block.body)
//	  process_eth1_data(state, block.body)
//	  process_operations(state, block.body)
func ProcessBlockNoVerifyAnySig(
	ctx context.Context,
	bState state.BeaconState,
	signed block.SignedBeaconBlock,
) (*bls.SignatureBatch, state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.ProcessBlockNoVerifyAnySig")
	defer span.End()
	if err := helpers.BeaconBlockIsNil(signed); err != nil {
		return nil, nil, err
	}

	if bState.Version() != signed.Block().Version() {
		return nil, nil, fmt.Errorf("state and block are different version. %d != %d", bState.Version(), signed.Block().Version())
	}

	blk := signed.Block()
	bState, err := ProcessBlockForStateRoot(ctx, bState, signed)
	if err != nil {
		return nil, nil, err
	}

	bSet, err := b.BlockSignatureBatch(bState, blk.ProposerIndex(), signed.Signature(), blk.HashTreeRoot)
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, nil, errors.Wrap(err, "could not retrieve block signature set")
	}

	rSet, err := b.RandaoSignatureBatch(ctx, bState, signed.Block().Body().RandaoReveal())
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, nil, errors.Wrap(err, "could not retrieve randao signature set")
	}

	aSet, err := b.AttestationSignatureBatch(ctx, bState, signed.Block().Body().Attestations())
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not retrieve attestation signature set")
	}

	// Merge beacon block, randao and attestations signatures into a set.
	set := bls.NewSet()
	set.Join(bSet).Join(rSet).Join(aSet)

	return set, bState, nil
}

// ProcessOperationsNoVerifyAttsSigs processes the operations in the beacon block and updates beacon state
// with the operations in block. It does not verify attestation signatures.
//
// WARNING: This method does not verify attestation signatures.
// This is used to perform the block operations as fast as possible.
//
// Spec pseudocode definition:
//
//	def process_operations(state: BeaconState, body: BeaconBlockBody) -> None:
//	  # Verify that outstanding deposits are processed up to the maximum number of deposits
//	  assert len(body.deposits) == min(MAX_DEPOSITS, state.eth1_data.deposit_count - state.eth1_deposit_index)
//
//	  def for_ops(operations: Sequence[Any], fn: Callable[[BeaconState, Any], None]) -> None:
//	      for operation in operations:
//	          fn(state, operation)
//
//	  for_ops(body.proposer_slashings, process_proposer_slashing)
//	  for_ops(body.attester_slashings, process_attester_slashing)
//	  for_ops(body.attestations, process_attestation)
//	  for_ops(body.deposits, process_deposit)
//	  for_ops(body.voluntary_exits, process_voluntary_exit)
func ProcessOperationsNoVerifyAttsSigs(
	ctx context.Context,
	state state.BeaconState,
	signedBeaconBlock block.SignedBeaconBlock) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.ProcessOperationsNoVerifyAttsSigs")
	defer span.End()
	if err := helpers.BeaconBlockIsNil(signedBeaconBlock); err != nil {
		return nil, err
	}

	if _, err := VerifyOperationLengths(ctx, state, signedBeaconBlock); err != nil {
		return nil, errors.Wrap(err, "could not verify operation lengths")
	}

	var err error
	switch signedBeaconBlock.Version() {
	case version.Phase0:
		state, err = phase0Operations(ctx, state, signedBeaconBlock)
		if err != nil {
			return nil, err
		}
	case version.Altair, version.Bellatrix:
		state, err = altairOperations(ctx, state, signedBeaconBlock)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("block does not have correct version")
	}

	return state, nil
}

// ProcessBlockForStateRoot processes the state for state root computation. It skips proposer signature
// and randao signature verifications.
//
// Spec pseudocode definition:
// def process_block(state: BeaconState, block: BeaconBlock) -> None:
//
//	process_block_header(state, block)
//	if is_execution_enabled(state, block.body):
//	    process_execution_payload(state, block.body.execution_payload, EXECUTION_ENGINE)  # [New in Bellatrix]
//	process_randao(state, block.body)
//	process_eth1_data(state, block.body)
//	process_operations(state, block.body)
//	process_sync_aggregate(state, block.body.sync_aggregate)
func ProcessBlockForStateRoot(
	ctx context.Context,
	bState state.BeaconState,
	signed block.SignedBeaconBlock,
) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.ProcessBlockForStateRoot")
	defer span.End()
	if err := helpers.BeaconBlockIsNil(signed); err != nil {
		log.WithError(
			errors.Wrap(err, "BeaconBlockIsNil"),
		).Error("ProcessBlockForStateRoot:Err")
		return nil, err
	}

	blk := signed.Block()
	body := blk.Body()
	bodyRoot, err := body.HashTreeRoot()
	if err != nil {
		log.WithError(
			errors.Wrap(err, "could not hash tree root beacon block body"),
		).Error("ProcessBlockForStateRoot:Err")
		return nil, errors.Wrap(err, "could not hash tree root beacon block body")
	}

	bState, err = b.ProcessBlockHeaderNoVerify(ctx, bState, blk.Slot(), blk.ProposerIndex(), blk.ParentRoot(), bodyRoot[:])
	if err != nil {
		log.WithError(
			errors.Wrap(err, "could not process block header"),
		).Error("ProcessBlockForStateRoot:Err")
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not process block header")
	}

	bState, err = b.ProcessRandaoNoVerify(bState, signed.Block().Body().RandaoReveal())
	if err != nil {
		log.WithError(
			errors.Wrap(err, "could not verify and process randao"),
		).Error("ProcessBlockForStateRoot:Err")
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not verify and process randao")
	}

	bState, err = b.ProcessEth1DataInBlock(ctx, bState, signed.Block().Body().Eth1Data())
	if err != nil {
		log.WithError(
			errors.Wrap(err, "could not process eth1 data"),
		).Error("ProcessBlockForStateRoot:Err")
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not process eth1 data")
	}

	bState, err = b.ProcessDagConsensus(ctx, bState, signed)
	if err != nil {
		log.WithError(
			errors.Wrap(err, "could not process block voting data"),
		).Error("ProcessBlockForStateRoot:Err")
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not process block voting data")
	}

	bState, err = ProcessOperationsNoVerifyAttsSigs(ctx, bState, signed)
	if err != nil {
		log.WithError(
			errors.Wrap(err, "could not process block operation"),
		).Error("ProcessBlockForStateRoot:Err")
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not process block operation")
	}

	if signed.Block().Version() == version.Phase0 {
		return bState, nil
	}

	sa, err := signed.Block().Body().SyncAggregate()
	if err != nil {
		log.WithError(
			errors.Wrap(err, "could not get sync aggregate from block"),
		).Error("ProcessBlockForStateRoot:Err")
		return nil, errors.Wrap(err, "could not get sync aggregate from block")
	}

	bState, err = altair.ProcessSyncAggregate(ctx, bState, sa)
	if err != nil {
		log.WithError(
			errors.Wrap(err, "process_sync_aggregate failed"),
		).Error("ProcessBlockForStateRoot:Err")
		return nil, errors.Wrap(err, "process_sync_aggregate failed")
	}
	return bState, nil
}

// This calls altair block operations.
func altairOperations(
	ctx context.Context,
	bState state.BeaconState,
	signedBeaconBlock block.SignedBeaconBlock) (state.BeaconState, error) {
	bState, err := b.ProcessProposerSlashings(ctx, bState, signedBeaconBlock.Block().Body().ProposerSlashings(), v.SlashValidator)
	if err != nil {
		return nil, errors.Wrap(err, "could not process altair proposer slashing")
	}
	bState, err = b.ProcessAttesterSlashings(ctx, bState, signedBeaconBlock.Block().Body().AttesterSlashings(), v.SlashValidator)
	if err != nil {
		return nil, errors.Wrap(err, "could not process altair attester slashing")
	}
	bState, err = altair.ProcessAttestationsNoVerifySignature(ctx, bState, signedBeaconBlock)
	if err != nil {
		return nil, errors.Wrap(err, "could not process altair attestation")
	}
	if bState, err = altair.ProcessDeposits(ctx, bState, signedBeaconBlock.Block().Body().Deposits()); err != nil {
		return nil, errors.Wrap(err, "could not process altair deposit")
	}
	if bState, err = b.ProcessWithdrawal(ctx, bState, signedBeaconBlock.Block().Body().Withdrawals()); err != nil {
		return nil, errors.Wrap(err, "could not process altair withdrawals")
	}
	return b.ProcessVoluntaryExits(ctx, bState, signedBeaconBlock.Block().Body().VoluntaryExits())
}

// This calls phase 0 block operations.
func phase0Operations(
	ctx context.Context,
	bState state.BeaconStateAltair,
	signedBeaconBlock block.SignedBeaconBlock) (state.BeaconState, error) {
	bState, err := b.ProcessProposerSlashings(ctx, bState, signedBeaconBlock.Block().Body().ProposerSlashings(), v.SlashValidator)
	if err != nil {
		return nil, errors.Wrap(err, "could not process block proposer slashings")
	}
	bState, err = b.ProcessAttesterSlashings(ctx, bState, signedBeaconBlock.Block().Body().AttesterSlashings(), v.SlashValidator)
	if err != nil {
		return nil, errors.Wrap(err, "could not process block attester slashings")
	}
	bState, err = b.ProcessAttestationsNoVerifySignature(ctx, bState, signedBeaconBlock)
	if err != nil {
		return nil, errors.Wrap(err, "could not process block attestations")
	}
	if bState, err = b.ProcessDeposits(ctx, bState, signedBeaconBlock.Block().Body().Deposits()); err != nil {
		return nil, errors.Wrap(err, "could not process deposits")
	}
	if bState, err = b.ProcessWithdrawal(ctx, bState, signedBeaconBlock.Block().Body().Withdrawals()); err != nil {
		return nil, errors.Wrap(err, "could not process withdrawals")
	}
	return b.ProcessVoluntaryExits(ctx, bState, signedBeaconBlock.Block().Body().VoluntaryExits())
}

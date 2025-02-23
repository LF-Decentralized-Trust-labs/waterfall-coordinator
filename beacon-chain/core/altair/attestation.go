package altair

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	log "github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/blocks"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/attestation"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"go.opencensus.io/trace"
)

// ProcessAttestationsNoVerifySignature applies processing operations to a block's inner attestation
// records. The only difference would be that the attestation signature would not be verified.
func ProcessAttestationsNoVerifySignature(
	ctx context.Context,
	beaconState state.BeaconState,
	b block.SignedBeaconBlock,
) (state.BeaconState, error) {
	if err := helpers.BeaconBlockIsNil(b); err != nil {
		return nil, err
	}
	handledIndexes := map[[32]byte]map[uint64]bool{}
	body := b.Block().Body()
	for idx, att := range body.Attestations() {
		var err error
		beaconState, err = ProcessAttestationNoVerifySignature(ctx, beaconState, att, handledIndexes)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"i":        idx,
				"slot":     b.Block().Slot(),
				"att.root": fmt.Sprintf("%#x", att.Data.BeaconBlockRoot),
				"att.slot": fmt.Sprintf("%d", att.Data.Slot),
				"parent":   fmt.Sprintf("%#x", b.Block().ParentRoot()),
			}).Error("Process attestations err")
			return nil, errors.Wrapf(err, "could not verify attestation at index %d in block", idx)
		}
	}
	return beaconState, nil
}

// ProcessAttestationNoVerifySignature processes the attestation without verifying the attestation signature. This
// method is used to validate attestations whose signatures have already been verified or will be verified later.
func ProcessAttestationNoVerifySignature(
	ctx context.Context,
	beaconState state.BeaconStateAltair,
	att *ethpb.Attestation,
	handledIndexes map[[32]byte]map[uint64]bool,
) (state.BeaconStateAltair, error) {
	ctx, span := trace.StartSpan(ctx, "altair.ProcessAttestationNoVerifySignature")
	defer span.End()

	if err := blocks.VerifyAttestationNoVerifySignature(ctx, beaconState, att); err != nil {
		return nil, err
	}

	delay, err := beaconState.Slot().SafeSubSlot(att.Data.Slot)
	if err != nil {
		return nil, fmt.Errorf("att slot %d can't be greater than state slot %d", att.Data.Slot, beaconState.Slot())
	}
	//participatedFlags: map[uint8]bool{sourceFlagIndex: true, targetFlagIndex: true, headFlagIndex: true,}
	participatedFlags, err := AttestationParticipationFlagIndices(beaconState, att.Data, delay)
	if err != nil {
		return nil, err
	}
	// validator indexes of committee
	committee, err := helpers.BeaconCommitteeFromState(ctx, beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	if err != nil {
		return nil, err
	}
	// aggregated validator indexes
	indices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	if err != nil {
		return nil, err
	}

	//rm handled indexes
	var attDataRoot [32]byte
	if handledIndexes != nil {
		attDataRoot, err = att.Data.HashTreeRoot()
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"att.slot":            att.Data.Slot,
				"att.CommitteeIndex":  att.Data.CommitteeIndex,
				"att.AggregationBits": fmt.Sprintf("%b", att.AggregationBits),
			}).Error("Calc attestation HashTreeRoot failed")
			return beaconState, err
		}

		if hixs, ok := handledIndexes[attDataRoot]; ok {
			upIxs := make([]uint64, 0, len(indices))
			for _, ix := range indices {
				isHandled := hixs[ix]
				if !isHandled {
					upIxs = append(upIxs, ix)
				} else {
					log.WithFields(log.Fields{
						"att.slot":           att.Data.Slot,
						"att.CommitteeIndex": att.Data.CommitteeIndex,
						"validator":          ix,
					}).Info("Handled attestation detected")
				}
			}
			indices = upIxs
		}
	}

	bState, err := SetParticipationAndRewardProposer(ctx, beaconState, att.Data.Target.Epoch, indices, participatedFlags, att.Data.BeaconBlockRoot)

	//update handledIndexes
	if err == nil && handledIndexes != nil {
		if _, ok := handledIndexes[attDataRoot]; !ok {
			handledIndexes[attDataRoot] = map[uint64]bool{}
		}
		for _, ix := range indices {
			handledIndexes[attDataRoot][ix] = true
		}
	}
	return bState, err
}

// SetParticipationAndRewardProposer performs
// 1. retrieves and sets the epoch participation bits in state,
// 2. calculate reward for proposer of the current block,
// 3. rewards proposer of the current block,
// 4. rewards the block proposer voted for by the participants,
func SetParticipationAndRewardProposer(
	ctx context.Context,
	beaconState state.BeaconState,
	targetEpoch types.Epoch,
	indices []uint64,
	participatedFlags map[uint8]bool, beaconBlockRoot []byte) (state.BeaconState, error) {
	var proposerReward uint64
	currentEpoch := time.CurrentEpoch(beaconState)
	var stateErr error
	// 1. retrieves and sets the epoch participation bits in state,
	// 2. calculate reward for proposer of the current block,
	if targetEpoch == currentEpoch {
		stateErr = beaconState.ModifyCurrentParticipationBits(func(val []byte) ([]byte, error) {
			propReward, epochParticipation, err := EpochParticipation(beaconState, indices, val, participatedFlags)
			if err != nil {
				return nil, err
			}
			proposerReward = propReward
			return epochParticipation, nil
		})
	} else {
		stateErr = beaconState.ModifyPreviousParticipationBits(func(val []byte) ([]byte, error) {
			rewardNum, epochParticipation, err := EpochParticipation(beaconState, indices, val, participatedFlags)
			if err != nil {
				return nil, err
			}
			proposerReward = rewardNum
			return epochParticipation, nil
		})
	}
	if stateErr != nil {
		return nil, stateErr
	}

	// 3. rewards proposer of the current block,
	proposerIndex, err := helpers.BeaconProposerIndex(ctx, beaconState)
	if err != nil {
		return nil, err
	}

	// write Rewards And Penalties log
	if err = helpers.LogBeforeRewardsAndPenalties(beaconState, proposerIndex, proposerReward, indices, helpers.BalanceIncrease, helpers.OpProposing); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"Slot":           beaconState.Slot(),
			"Proposer":       proposerIndex,
			"ProposerReward": proposerReward,
		}).Error("Log rewards and penalties failed: SetParticipationAndRewardProposer")
	}

	if err = helpers.IncreaseBalance(beaconState, proposerIndex, proposerReward); err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"Slot":           beaconState.Slot(),
		"Proposer":       proposerIndex,
		"ProposerReward": proposerReward,
	}).Debug("Reward proposer: current block incr")

	// 4. rewards the block proposer voted for by the participants,
	if err := RewardBeaconBlockRootProposer(ctx, beaconState, beaconBlockRoot, proposerReward, indices); err != nil {
		return nil, err
	}

	return beaconState, nil
}

// HasValidatorFlag returns true if the flag at position has set.
func HasValidatorFlag(flag, flagPosition uint8) (bool, error) {
	if flagPosition > 7 {
		return false, errors.New("flag position exceeds length")
	}
	return ((flag >> flagPosition) & 1) == 1, nil
}

// AddValidatorFlag adds new validator flag to existing one.
func AddValidatorFlag(flag, flagPosition uint8) (uint8, error) {
	if flagPosition > 7 {
		return flag, errors.New("flag position exceeds length")
	}
	return flag | (1 << flagPosition), nil
}

// EpochParticipation sets and returns the proposer reward numerator and epoch participation.
//
// Spec code:
//
//	proposer_reward_numerator = 0
//	for index in get_attesting_indices(state, data, attestation.aggregation_bits):
//	    for flag_index, weight in enumerate(PARTICIPATION_FLAG_WEIGHTS):
//	        if flag_index in participation_flag_indices and not has_flag(epoch_participation[index], flag_index):
//	            epoch_participation[index] = add_flag(epoch_participation[index], flag_index)
//	            proposer_reward_numerator += get_base_reward(state, index) * weight
func EpochParticipation(
	beaconState state.BeaconState,
	indices []uint64,
	epochParticipation []byte,
	participatedFlags map[uint8]bool,
) (uint64, []byte, error) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	numOfValidators := beaconState.NumValidators() // N in formula, number of registered validators
	activeValidatorsForSlot, err := helpers.ActiveValidatorForSlotCount(ctx, beaconState, beaconState.Slot())
	if err != nil || activeValidatorsForSlot == 0 {
		activeValidatorsForSlot = cfg.MaxCommitteesPerSlot * cfg.TargetCommitteeSize
	}
	baseReward := CalculateBaseReward(cfg, numOfValidators, activeValidatorsForSlot, cfg.BaseRewardMultiplier)
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex
	votingFlagIndex := cfg.DAGTimelyVotingFlagIndex
	proposerReward := uint64(0)
	for _, index := range indices {
		if index >= uint64(len(epochParticipation)) {
			return 0, nil, fmt.Errorf("index %d exceeds participation length %d", index, len(epochParticipation))
		}
		has, err := HasValidatorFlag(epochParticipation[index], sourceFlagIndex)
		if err != nil {
			return 0, nil, err
		}
		if participatedFlags[sourceFlagIndex] && !has {
			epochParticipation[index], err = AddValidatorFlag(epochParticipation[index], sourceFlagIndex)
			if err != nil {
				return 0, nil, err
			}
			proposerReward += uint64(float64(baseReward) * (cfg.DAGTimelySourceWeight / 2))
		}
		has, err = HasValidatorFlag(epochParticipation[index], targetFlagIndex)
		if err != nil {
			return 0, nil, err
		}
		if participatedFlags[targetFlagIndex] && !has {
			epochParticipation[index], err = AddValidatorFlag(epochParticipation[index], targetFlagIndex)
			if err != nil {
				return 0, nil, err
			}
			proposerReward += uint64(float64(baseReward) * (cfg.DAGTimelyTargetWeight / 2))
		}
		has, err = HasValidatorFlag(epochParticipation[index], headFlagIndex)
		if err != nil {
			return 0, nil, err
		}
		if participatedFlags[headFlagIndex] && !has {
			epochParticipation[index], err = AddValidatorFlag(epochParticipation[index], headFlagIndex)
			if err != nil {
				return 0, nil, err
			}
			proposerReward += uint64(float64(baseReward) * (cfg.DAGTimelyHeadWeight / 2))
		}
		has, err = HasValidatorFlag(epochParticipation[index], votingFlagIndex)
		if err != nil {
			return 0, nil, err
		}
		if participatedFlags[headFlagIndex] && !has {
			epochParticipation[index], err = AddValidatorFlag(epochParticipation[index], votingFlagIndex)
			if err != nil {
				return 0, nil, err
			}
			proposerReward += uint64(float64(baseReward) * (cfg.DAGTimelyVotingWeight / 2))
		}
		//log.WithFields(log.Fields{
		//	"Slot":             beaconState.Slot(),
		//	"Validator":        index,
		//	"NumValidators":    numOfValidators,
		//	"ActiveValidators": activeValidatorsForSlot,
		//	"BaseReward":       baseReward,
		//	"sourceVoting":     participatedFlags[sourceFlagIndex],
		//	"targetVoting":     participatedFlags[targetFlagIndex],
		//	"headVoting":       participatedFlags[headFlagIndex],
		//	"timelyVoting":     participatedFlags[sourceFlagIndex] && participatedFlags[targetFlagIndex] && participatedFlags[headFlagIndex],
		//}).Debug("Reward proposer: calc by epoch participation incr")
	}

	return proposerReward, epochParticipation, nil
}

// RewardBeaconBlockRootProposer rewards the block proposer voted for by the participants
func RewardBeaconBlockRootProposer(
	ctx context.Context,
	beaconState state.BeaconState,
	attRoot []byte,
	reward uint64,
	indices []uint64,
) error {
	blockFetcher, ok := ctx.Value(params.BeaconConfig().CtxBlockFetcherKey).(params.CtxBlockFetcher)
	if !ok {
		err := errors.New("Cannot cast to CtxBlockFetcher")
		log.WithError(err).WithFields(log.Fields{
			"Slot":   beaconState.Slot(),
			"reward": reward,
		}).Error("Proposer reward error: get CtxBlockFetcher failed")
		return err
	}
	proposerIndex, proposedAtSlot, _, err := blockFetcher(ctx, bytesutil.ToBytes32(attRoot))
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"SlotBlockWasProposedAt": proposedAtSlot,
			"Slot":                   beaconState.Slot(),
			"attestationRoot":        fmt.Sprintf("%#x", attRoot),
			"ProposerIndex":          proposerIndex,
			"reward":                 reward,
			"attestors":              indices,
		}).Error("Proposer reward error: retrieving block failed")
		// if no block on local node
		if strings.Contains(err.Error(), "not found in db") {
			log.WithFields(log.Fields{
				"Slot":            beaconState.Slot(),
				"attestationRoot": fmt.Sprintf("%#x", attRoot),
				"reward":          reward,
				"canonical":       false,
			}).Debug("Reward proposer: skip reward of voting for root (not found)")
			return nil
		}
		return err
	}
	slotRoot, err := helpers.BlockRootAtSlot(beaconState, proposedAtSlot)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"proposedAtSlot":  proposedAtSlot,
			"Slot":            beaconState.Slot(),
			"attestationRoot": fmt.Sprintf("%#x", attRoot),
			"reward":          reward,
			"attestors":       indices,
		}).Error("Proposer reward error: retrieving historical root failed")
		return err
	}
	// if attested root is not in past of state - skip reward
	if !bytes.Equal(slotRoot, attRoot) {
		log.WithFields(log.Fields{
			"Slot":            beaconState.Slot(),
			"slotRoot":        fmt.Sprintf("%#x", slotRoot),
			"attestationRoot": fmt.Sprintf("%#x", attRoot),
			"reward":          reward,
			"canonical":       false,
		}).Debug("Reward proposer: skip reward of voting for root (not canonical)")
		return nil
	}

	log.WithFields(log.Fields{
		"SlotBlockWasProposedAt": proposedAtSlot,
		"Slot":                   beaconState.Slot(),
		"attestationRoot":        fmt.Sprintf("%#x", attRoot),
		"Proposer":               proposerIndex,
		"reward":                 reward,
	}).Debug("Reward proposer: voting for root incr")

	// write Rewards And Penalties log
	if err = helpers.LogBeforeRewardsAndPenalties(beaconState, proposerIndex, reward, indices, helpers.BalanceIncrease, helpers.OpBlockAttested); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"Slot":     beaconState.Slot(),
			"Proposer": proposerIndex,
			"reward":   reward,
		}).Error("Log rewards and penalties failed: RewardBeaconBlockRootProposer")
	}

	// Should we have the state at beacon block root slot or current is ok??? seems current is ok
	if err = helpers.IncreaseBalance(beaconState, proposerIndex, reward); err != nil {
		return err
	}
	return nil
}

// AttestationParticipationFlagIndices retrieves a map of attestation scoring based on Altair's participation flag indices.
// This is used to facilitate process attestation during state transition and during upgrade to altair state.
//
// Spec code:
// def get_attestation_participation_flag_indices(state: BeaconState,
//
//	                                           data: AttestationData,
//	                                           inclusion_delay: uint64) -> Sequence[int]:
//	"""
//	Return the flag indices that are satisfied by an attestation.
//	"""
//	if data.target.epoch == get_current_epoch(state):
//	    justified_checkpoint = state.current_justified_checkpoint
//	else:
//	    justified_checkpoint = state.previous_justified_checkpoint
//
//	# Matching roots
//	is_matching_source = data.source == justified_checkpoint
//	is_matching_target = is_matching_source and data.target.root == get_block_root(state, data.target.epoch)
//	is_matching_head = is_matching_target and data.beacon_block_root == get_block_root_at_slot(state, data.slot)
//	assert is_matching_source
//
//	participation_flag_indices = []
//	if is_matching_source and inclusion_delay <= integer_squareroot(SLOTS_PER_EPOCH):
//	    participation_flag_indices.append(TIMELY_SOURCE_FLAG_INDEX)
//	if is_matching_target and inclusion_delay <= SLOTS_PER_EPOCH:
//	    participation_flag_indices.append(TIMELY_TARGET_FLAG_INDEX)
//	if is_matching_head and inclusion_delay == MIN_ATTESTATION_INCLUSION_DELAY:
//	    participation_flag_indices.append(TIMELY_HEAD_FLAG_INDEX)
//
//	return participation_flag_indices
func AttestationParticipationFlagIndices(beaconState state.BeaconStateAltair, data *ethpb.AttestationData, delay types.Slot) (map[uint8]bool, error) {
	currEpoch := time.CurrentEpoch(beaconState)
	var justifiedCheckpt *ethpb.Checkpoint
	if data.Target.Epoch == currEpoch {
		justifiedCheckpt = beaconState.CurrentJustifiedCheckpoint()
	} else {
		justifiedCheckpt = beaconState.PreviousJustifiedCheckpoint()
	}

	matchedSrc, matchedTgt, matchedHead, err := MatchingStatus(beaconState, data, justifiedCheckpt)
	if err != nil {
		return nil, err
	}
	if !matchedSrc {
		return nil, errors.New("source epoch does not match")
	}

	participatedFlags := make(map[uint8]bool)
	cfg := params.BeaconConfig()
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex
	votingFlagIndex := cfg.DAGTimelyVotingFlagIndex
	slotsPerEpoch := cfg.SlotsPerEpoch
	sqtRootSlots := cfg.SqrRootSlotsPerEpoch
	if matchedSrc && delay <= sqtRootSlots {
		participatedFlags[sourceFlagIndex] = true
	}
	matchedSrcTgt := matchedSrc && matchedTgt
	if matchedSrcTgt && delay <= slotsPerEpoch {
		participatedFlags[targetFlagIndex] = true
	}
	matchedSrcTgtHead := matchedHead && matchedSrcTgt
	if matchedSrcTgtHead && delay == cfg.MinAttestationInclusionDelay {
		participatedFlags[headFlagIndex] = true
	}
	// Participated in attestation in timely manner for source, target and head
	participatedFlags[votingFlagIndex] = participatedFlags[sourceFlagIndex] &&
		participatedFlags[targetFlagIndex] &&
		participatedFlags[headFlagIndex]
	return participatedFlags, nil
}

// MatchingStatus returns the matching statues for attestation data's source target and head.
//
// Spec code:
//
//	is_matching_source = data.source == justified_checkpoint
//	is_matching_target = is_matching_source and data.target.root == get_block_root(state, data.target.epoch)
//	is_matching_head = is_matching_target and data.beacon_block_root == get_block_root_at_slot(state, data.slot)
func MatchingStatus(beaconState state.BeaconState, data *ethpb.AttestationData, cp *ethpb.Checkpoint) (matchedSrc, matchedTgt, matchedHead bool, err error) {
	matchedSrc = attestation.CheckPointIsEqual(data.Source, cp)

	r, err := helpers.BlockRoot(beaconState, data.Target.Epoch)
	if err != nil {
		return false, false, false, err
	}
	matchedTgt = bytes.Equal(r, data.Target.Root)

	r, err = helpers.BlockRootAtSlot(beaconState, data.Slot)
	if err != nil {
		return false, false, false, err
	}
	matchedHead = bytes.Equal(r, data.BeaconBlockRoot)
	return
}

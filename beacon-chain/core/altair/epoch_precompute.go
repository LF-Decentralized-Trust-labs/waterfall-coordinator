package altair

import (
	"context"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/epoch/precompute"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/math"
	"go.opencensus.io/trace"
)

// InitializePrecomputeValidators precomputes individual validator for its attested balances and the total sum of validators attested balances of the epoch.
func InitializePrecomputeValidators(ctx context.Context, beaconState state.BeaconStateAltair) ([]*precompute.Validator, *precompute.Balance, error) {
	_, span := trace.StartSpan(ctx, "altair.InitializePrecomputeValidators")
	defer span.End()
	vals := make([]*precompute.Validator, beaconState.NumValidators())
	bal := &precompute.Balance{}
	prevEpoch := time.PrevEpoch(beaconState)
	currentEpoch := time.CurrentEpoch(beaconState)
	inactivityScores, err := beaconState.InactivityScores()
	if err != nil {
		return nil, nil, err
	}

	// This shouldn't happen with a correct beacon state,
	// but rather be safe to defend against index out of bound panics.
	if beaconState.NumValidators() != len(inactivityScores) {
		return nil, nil, errors.New("num of validators is different than num of inactivity scores")
	}
	if err := beaconState.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		// Set validator's balance, inactivity score and slashed/withdrawable status.
		v := &precompute.Validator{
			CurrentEpochEffectiveBalance: val.EffectiveBalance(),
			InactivityScore:              inactivityScores[idx],
			IsSlashed:                    val.Slashed(),
			IsWithdrawableCurrentEpoch:   currentEpoch >= val.WithdrawableEpoch(),
		}
		// Set validator's active status for current epoch.
		if helpers.IsActiveValidatorUsingTrie(val, currentEpoch) {
			v.IsActiveCurrentEpoch = true
			bal.ActiveCurrentEpoch, err = math.Add64(bal.ActiveCurrentEpoch, val.EffectiveBalance())
			if err != nil {
				return err
			}
		}
		// Set validator's active status for preivous epoch.
		if helpers.IsActiveValidatorUsingTrie(val, prevEpoch) {
			v.IsActivePrevEpoch = true
			bal.ActivePrevEpoch, err = math.Add64(bal.ActivePrevEpoch, val.EffectiveBalance())
			if err != nil {
				return err
			}
		}
		vals[idx] = v
		return nil
	}); err != nil {
		return nil, nil, errors.Wrap(err, "could not read every validator")
	}
	return vals, bal, nil
}

// ProcessInactivityScores of beacon chain. This updates inactivity scores of beacon chain and
// updates the precompute validator struct for later processing. The inactivity scores work as following:
// For fully inactive validators and perfect active validators, the effect is the same as before Altair.
// For a validator is inactive and the chain fails to finalize, the inactivity score increases by a fixed number, the total loss after N epochs is proportional to N**2/2.
// For imperfectly active validators. The inactivity score's behavior is specified by this function:
//
//	If a validator fails to submit an attestation with the correct target, their inactivity score goes up by 4.
//	If they successfully submit an attestation with the correct source and target, their inactivity score drops by 1
//	If the chain has recently finalized, each validator's score drops by 16.
func ProcessInactivityScores(
	ctx context.Context,
	beaconState state.BeaconState,
	vals []*precompute.Validator,
) (state.BeaconState, []*precompute.Validator, error) {
	_, span := trace.StartSpan(ctx, "altair.ProcessInactivityScores")
	defer span.End()

	cfg := params.BeaconConfig()
	if time.CurrentEpoch(beaconState) == cfg.GenesisEpoch {
		return beaconState, vals, nil
	}

	inactivityScores, err := beaconState.InactivityScores()
	if err != nil {
		return nil, nil, err
	}

	bias := cfg.InactivityScoreBias
	recoveryRate := cfg.InactivityScoreRecoveryRate
	prevEpoch := time.PrevEpoch(beaconState)
	finalizedEpoch := beaconState.FinalizedCheckpointEpoch()
	for i, v := range vals {
		if !precompute.EligibleForRewards(v) {
			continue
		}

		if v.IsPrevEpochTargetAttester && !v.IsSlashed {
			// Decrease inactivity score when validator gets target correct.
			if v.InactivityScore > 0 {
				v.InactivityScore -= 1
			}
		} else {
			v.InactivityScore, err = math.Add64(v.InactivityScore, bias)
			if err != nil {
				return nil, nil, err
			}
		}

		if !helpers.IsInInactivityLeak(prevEpoch, finalizedEpoch) {
			score := recoveryRate
			// Prevents underflow below 0.
			if score > v.InactivityScore {
				score = v.InactivityScore
			}
			v.InactivityScore -= score
		}
		inactivityScores[i] = v.InactivityScore
	}

	if err := beaconState.SetInactivityScores(inactivityScores); err != nil {
		return nil, nil, err
	}

	return beaconState, vals, nil
}

// ProcessEpochParticipation processes the epoch participation in state and updates individual validator's pre computes,
// it also tracks and updates epoch attesting balances.
// Spec code:
// if epoch == get_current_epoch(state):
//
//	    epoch_participation = state.current_epoch_participation
//	else:
//	    epoch_participation = state.previous_epoch_participation
//	active_validator_indices = get_active_validator_indices(state, epoch)
//	participating_indices = [i for i in active_validator_indices if has_flag(epoch_participation[i], flag_index)]
//	return set(filter(lambda index: not state.validators[index].slashed, participating_indices))
func ProcessEpochParticipation(
	ctx context.Context,
	beaconState state.BeaconState,
	bal *precompute.Balance,
	vals []*precompute.Validator,
) ([]*precompute.Validator, *precompute.Balance, error) {
	_, span := trace.StartSpan(ctx, "altair.ProcessEpochParticipation")
	defer span.End()

	cp, err := beaconState.CurrentEpochParticipation()
	if err != nil {
		return nil, nil, err
	}
	cfg := params.BeaconConfig()
	targetIdx := cfg.TimelyTargetFlagIndex
	sourceIdx := cfg.TimelySourceFlagIndex
	headIdx := cfg.TimelyHeadFlagIndex
	votingIdx := cfg.DAGTimelyVotingFlagIndex
	for i, b := range cp {
		has, err := HasValidatorFlag(b, sourceIdx)
		if err != nil {
			return nil, nil, err
		}
		if has && vals[i].IsActiveCurrentEpoch {
			vals[i].IsCurrentEpochAttester = true
		}
		has, err = HasValidatorFlag(b, targetIdx)
		if err != nil {
			return nil, nil, err
		}
		if has && vals[i].IsActiveCurrentEpoch {
			vals[i].IsCurrentEpochAttester = true
			vals[i].IsCurrentEpochTargetAttester = true
		}
	}
	pp, err := beaconState.PreviousEpochParticipation()
	if err != nil {
		return nil, nil, err
	}
	for i, b := range pp {
		has, err := HasValidatorFlag(b, sourceIdx)
		if err != nil {
			return nil, nil, err
		}
		if has && vals[i].IsActivePrevEpoch {
			vals[i].IsPrevEpochAttester = true
			vals[i].IsPrevEpochSourceAttester = true
		}
		has, err = HasValidatorFlag(b, targetIdx)
		if err != nil {
			return nil, nil, err
		}
		if has && vals[i].IsActivePrevEpoch {
			vals[i].IsPrevEpochAttester = true
			vals[i].IsPrevEpochTargetAttester = true
		}
		has, err = HasValidatorFlag(b, headIdx)
		if err != nil {
			return nil, nil, err
		}
		if has && vals[i].IsActivePrevEpoch {
			vals[i].IsPrevEpochHeadAttester = true
		}
		has, err = HasValidatorFlag(b, votingIdx)
		if err != nil {
			return nil, nil, err
		}
		if has && vals[i].IsActivePrevEpoch {
			vals[i].IsPrevEpochHeadAttester = true
		}
	}
	bal = precompute.UpdateBalance(vals, bal, beaconState.Version())
	return vals, bal, nil
}

// ProcessRewardsAndPenaltiesPrecompute processes the rewards and penalties of individual validator.
// This is an optimized version by passing in precomputed validator attesting records and and total epoch balances.
func ProcessRewardsAndPenaltiesPrecompute(
	beaconState state.BeaconStateAltair,
	bal *precompute.Balance,
	vals []*precompute.Validator,
) (state.BeaconStateAltair, error) {
	// Don't process rewards and penalties in genesis epoch.
	cfg := params.BeaconConfig()
	if time.CurrentEpoch(beaconState) == cfg.GenesisEpoch {
		return beaconState, nil
	}

	numOfVals := beaconState.NumValidators()
	// Guard against an out-of-bounds using validator balance precompute.
	if len(vals) != numOfVals || len(vals) != beaconState.BalancesLength() {
		return beaconState, errors.New("validator registries not the same length as state's validator registries")
	}

	attsRewards, attsPenalties, err := AttestationsDelta(beaconState, bal, vals)
	if err != nil {
		return nil, errors.Wrap(err, "could not get attestation delta")
	}

	balances := beaconState.Balances()
	for valIndex := 0; valIndex < numOfVals; valIndex++ {
		isLocked, err := helpers.IsWithdrawBalanceLocked(beaconState, types.ValidatorIndex(valIndex))
		if err != nil {
			return nil, err
		}
		if isLocked {
			continue
		}

		vals[valIndex].BeforeEpochTransitionBalance = balances[valIndex]

		// Compute the post balance of the validator after accounting for the
		// attester and proposer rewards and penalties.

		//log.WithFields(log.Fields{
		//	"Slot":              beaconState.Slot(),
		//	"Validator":         valIndex,
		//	"AttestationReward": attsRewards[valIndex],
		//}).Debug("Reward attestor: incr")

		// write Rewards And Penalties log
		if attsRewards[valIndex] != 0 {
			aftBal, err := helpers.IncreaseBalanceWithVal(balances[valIndex], attsRewards[valIndex])
			if err != nil {
				return nil, err
			}
			if err = helpers.LogBalanceChanges(types.ValidatorIndex(valIndex), balances[valIndex], attsRewards[valIndex], aftBal, beaconState.Slot(), nil, helpers.BalanceIncrease, helpers.OpAttestation); err != nil {
				return nil, err
			}
		}

		balances[valIndex], err = helpers.IncreaseBalanceWithVal(balances[valIndex], attsRewards[valIndex])
		if err != nil {
			return nil, err
		}

		//log.WithFields(log.Fields{
		//	"Slot":               beaconState.Slot(),
		//	"Validator":          valIndex,
		//	"AttestationPenalty": attsPenalties[valIndex],
		//}).Debug("Reward attestor: decr")

		// write Rewards And Penalties log
		if attsPenalties[valIndex] != 0 {
			if err = helpers.LogBalanceChanges(types.ValidatorIndex(valIndex), balances[valIndex], attsPenalties[valIndex], helpers.DecreaseBalanceWithVal(balances[valIndex], attsPenalties[valIndex]), beaconState.Slot(), nil, helpers.BalanceDecrease, helpers.OpAttestation); err != nil {
				return nil, err
			}
		}

		balances[valIndex] = helpers.DecreaseBalanceWithVal(balances[valIndex], attsPenalties[valIndex])

		vals[valIndex].AfterEpochTransitionBalance = balances[valIndex]
	}

	if err := beaconState.SetBalances(balances); err != nil {
		return nil, errors.Wrap(err, "could not set validator balances")
	}

	return beaconState, nil
}

// AttestationsDelta computes and returns the rewards and penalties differences for individual validators based on the
// voting records.
func AttestationsDelta(beaconState state.BeaconState, bal *precompute.Balance, vals []*precompute.Validator) (rewards, penalties []uint64, err error) {
	numOfVals := beaconState.NumValidators()
	rewards = make([]uint64, numOfVals)
	penalties = make([]uint64, numOfVals)

	cfg := params.BeaconConfig()
	prevEpoch := time.PrevEpoch(beaconState)
	finalizedEpoch := beaconState.FinalizedCheckpointEpoch()
	leak := helpers.IsInInactivityLeak(prevEpoch, finalizedEpoch)

	ctx := context.Background()
	activeValidatorsForSlot, err := helpers.ActiveValidatorForSlotCount(ctx, beaconState, beaconState.Slot())
	if err != nil || activeValidatorsForSlot == 0 {
		activeValidatorsForSlot = cfg.MaxCommitteesPerSlot * cfg.TargetCommitteeSize
	}

	bias := cfg.InactivityScoreBias
	inactivityDenominator := bias * cfg.InactivityPenaltyQuotientAltair

	for i, v := range vals {
		rewards[i], penalties[i], err = attestationDelta(bal, v, inactivityDenominator, leak, numOfVals, activeValidatorsForSlot)
		if err != nil {
			return nil, nil, err
		}
	}

	return rewards, penalties, nil
}

func attestationDelta(
	bal *precompute.Balance,
	val *precompute.Validator,
	inactivityDenominator uint64,
	inactivityLeak bool,
	validatorsNum int,
	activeValidatorsFroSlot uint64,
) (reward, penalty uint64, err error) {
	eligible := val.IsActivePrevEpoch || (val.IsSlashed && !val.IsWithdrawableCurrentEpoch)
	// Per spec `ActiveCurrentEpoch` can't be 0 to process attestation delta.
	if !eligible || bal.ActiveCurrentEpoch == 0 {
		return 0, 0, nil
	}

	cfg := params.BeaconConfig()
	effectiveBalance := val.CurrentEpochEffectiveBalance
	baseReward := CalculateBaseReward(cfg, validatorsNum, activeValidatorsFroSlot, cfg.BaseRewardMultiplier)

	weightDenominator := cfg.WeightDenominator
	srcWeight := cfg.DAGTimelySourceWeight
	tgtWeight := cfg.DAGTimelyTargetWeight
	headWeight := cfg.DAGTimelyHeadWeight
	votingWeight := cfg.DAGTimelyVotingWeight
	reward, penalty = uint64(0), uint64(0)
	// Process source reward / penalty
	if val.IsPrevEpochSourceAttester && !val.IsSlashed {
		if !inactivityLeak {
			reward += uint64(float64(baseReward) * srcWeight)
		}
	} else {
		penalty += uint64(float64(baseReward)*srcWeight) / weightDenominator
	}

	// Process target reward / penalty
	if val.IsPrevEpochTargetAttester && !val.IsSlashed {
		if !inactivityLeak {
			reward += uint64(float64(baseReward) * tgtWeight)
		}
	} else {
		penalty += uint64(float64(baseReward)*tgtWeight) / weightDenominator
	}

	// Process head reward / penalty
	if val.IsPrevEpochHeadAttester && !val.IsSlashed {
		if !inactivityLeak {
			reward += uint64(float64(baseReward) * headWeight)
		}
	}

	// Process timely voting reward / penalty
	if val.IsPrevEpochHeadAttester && !val.IsSlashed {
		if !inactivityLeak {
			reward += uint64(float64(baseReward) * votingWeight)
		}
	}

	// Process finality delay penalty
	// Apply an additional penalty to validators that did not vote on the correct target or slashed
	if !val.IsPrevEpochTargetAttester || val.IsSlashed {
		n, err := math.Mul64(effectiveBalance, val.InactivityScore)
		if err != nil {
			return 0, 0, err
		}
		penalty += n / inactivityDenominator
	}

	//log.WithFields(log.Fields{
	//	"NumValidators":             validatorsNum,
	//	"ActiveValidators":          activeValidatorsFroSlot,
	//	"BaseReward":                baseReward,
	//	"Reward":                    reward,
	//	"Penalty":                   penalty,
	//	"IsPrevEpochSourceAttester": val.IsPrevEpochSourceAttester,
	//	"IsPrevEpochTargetAttester": val.IsPrevEpochTargetAttester,
	//	"IsSlashed":                 val.IsSlashed,
	//	"inactivityLeak":            inactivityLeak,
	//}).Debug("Reward attestor: calc delta")

	return reward, penalty, nil
}

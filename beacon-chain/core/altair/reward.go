package altair

import (
	stdmath "math"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/math"
)

// BaseReward takes state and validator index and calculate
// individual validator's base reward.
//
// Spec code:
//
//	def get_base_reward(state: BeaconState, index: ValidatorIndex) -> Gwei:
//	  """
//	  Return the base reward for the validator defined by ``index`` with respect to the current ``state``.
//
//	  Note: An optimally performing validator can earn one base reward per epoch over a long time horizon.
//	  This takes into account both per-epoch (e.g. attestation) and intermittent duties (e.g. block proposal
//	  and sync committees).
//	  """
//	  increments = state.validators[index].effective_balance // EFFECTIVE_BALANCE_INCREMENT
//	  return Gwei(increments * get_base_reward_per_increment(state))
func BaseReward(s state.ReadOnlyBeaconState, index types.ValidatorIndex) (uint64, error) {
	totalBalance, err := helpers.TotalActiveBalance(s)
	if err != nil {
		return 0, errors.Wrap(err, "could not calculate active balance")
	}
	return BaseRewardWithTotalBalance(s, index, totalBalance)
}

// BaseRewardWithTotalBalance calculates the base reward with the provided total balance.
func BaseRewardWithTotalBalance(s state.ReadOnlyBeaconState, index types.ValidatorIndex, totalBalance uint64) (uint64, error) {
	val, err := s.ValidatorAtIndexReadOnly(index)
	if err != nil {
		return 0, err
	}
	cfg := params.BeaconConfig()
	increments := val.EffectiveBalance() / cfg.EffectiveBalanceIncrement
	baseRewardPerInc, err := BaseRewardPerIncrement(totalBalance)
	if err != nil {
		return 0, err
	}
	return increments * baseRewardPerInc, nil
}

// BaseRewardPerIncrement of the beacon state
//
// Spec code:
// def get_base_reward_per_increment(state: BeaconState) -> Gwei:
//
//	return Gwei(EFFECTIVE_BALANCE_INCREMENT * BASE_REWARD_FACTOR // integer_squareroot(get_total_active_balance(state)))
func BaseRewardPerIncrement(activeBalance uint64) (uint64, error) {
	if activeBalance == 0 {
		return 0, errors.New("active balance can't be 0")
	}
	cfg := params.BeaconConfig()
	return cfg.EffectiveBalanceIncrement * cfg.BaseRewardFactor / math.IntegerSquareRoot(activeBalance), nil
}

// CalculateBaseReward calculates the base reward value
// for further calculation of amounts validators rewards and penalties.
func CalculateBaseReward(config *params.BeaconChainConfig, validatorsNum int, activeValidatorsForSlot uint64, rewardMultiplier float64) uint64 {
	var (
		secondsInYear = 60 * 60 * 24 * 365.25 // Number of seconds in year
	)

	numOfSlotsPerYear := secondsInYear / float64(config.SecondsPerSlot) // Bi-th in formula
	annualMintedCoins := config.MaxAnnualizedReturnRate *
		float64(config.MaxEffectiveBalance) *
		stdmath.Sqrt(float64(uint64(validatorsNum)*config.OptValidatorsNum)) // v in formula
	rewardPerBlock := annualMintedCoins / numOfSlotsPerYear // Wi-th in formula
	baseReward := rewardPerBlock / (rewardMultiplier * float64(activeValidatorsForSlot))
	return uint64(baseReward)
}

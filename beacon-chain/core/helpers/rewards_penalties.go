package helpers

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
	types "github.com/prysmaticlabs/eth2-types"
	log "github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/hash"
	mathutil "gitlab.waterfall.network/waterfall/protocol/coordinator/math"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
)

var balanceCache = cache.NewEffectiveBalanceCache()

// TotalBalance returns the total amount at stake in Gwei
// of input validators.
//
// Spec pseudocode definition:
//
//	def get_total_balance(state: BeaconState, indices: Set[ValidatorIndex]) -> Gwei:
//	 """
//	 Return the combined effective balance of the ``indices``.
//	 ``EFFECTIVE_BALANCE_INCREMENT`` Gwei minimum to avoid divisions by zero.
//	 Math safe up to ~10B ETH, afterwhich this overflows uint64.
//	 """
//	 return Gwei(max(EFFECTIVE_BALANCE_INCREMENT, sum([state.validators[index].effective_balance for index in indices])))
func TotalBalance(state state.ReadOnlyValidators, indices []types.ValidatorIndex) uint64 {
	total := uint64(0)

	for _, idx := range indices {
		val, err := state.ValidatorAtIndexReadOnly(idx)
		if err != nil {
			continue
		}
		total += val.EffectiveBalance()
	}

	// EFFECTIVE_BALANCE_INCREMENT is the lower bound for total balance.
	if total < params.BeaconConfig().EffectiveBalanceIncrement {
		return params.BeaconConfig().EffectiveBalanceIncrement
	}

	return total
}

// TotalActiveBalance returns the total amount at stake in Gwei
// of active validators.
//
// Spec pseudocode definition:
//
//	def get_total_active_balance(state: BeaconState) -> Gwei:
//	 """
//	 Return the combined effective balance of the active validators.
//	 Note: ``get_total_balance`` returns ``EFFECTIVE_BALANCE_INCREMENT`` Gwei minimum to avoid divisions by zero.
//	 """
//	 return get_total_balance(state, set(get_active_validator_indices(state, get_current_epoch(state))))
func TotalActiveBalance(s state.ReadOnlyBeaconState) (uint64, error) {
	bal, err := balanceCache.Get(s)
	switch {
	case err == nil:
		return bal, nil
	case errors.Is(err, cache.ErrNotFound):
		// Do nothing if we receive a not found error.
	default:
		// In the event, we encounter another error we return it.
		return 0, err
	}

	total := uint64(0)
	epoch := slots.ToEpoch(s.Slot())
	if err := s.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		if IsActiveValidatorUsingTrie(val, epoch) {
			total += val.EffectiveBalance()
		}
		return nil
	}); err != nil {
		return 0, err
	}

	if err := balanceCache.AddTotalEffectiveBalance(s, total); err != nil {
		return 0, err
	}

	return total, nil
}

// IncreaseBalance increases validator with the given 'index' balance by 'delta' in Gwei.
//
// Spec pseudocode definition:
//
//	def increase_balance(state: BeaconState, index: ValidatorIndex, delta: Gwei) -> None:
//	  """
//	  Increase the validator balance at index ``index`` by ``delta``.
//	  """
//	  state.balances[index] += delta
func IncreaseBalance(state state.BeaconState, idx types.ValidatorIndex, delta uint64) error {
	isLocked, err := IsWithdrawBalanceLocked(state, idx)
	if err != nil || isLocked {
		return err
	}
	log.WithFields(log.Fields{
		"Slot":      state.Slot(),
		"Validator": idx,
		"Delta":     delta,
	}).Debug("Reward: IncreaseBalance")

	balAtIdx, err := state.BalanceAtIndex(idx)
	if err != nil {
		return err
	}
	newBal, err := IncreaseBalanceWithVal(balAtIdx, delta)
	if err != nil {
		return err
	}
	return state.UpdateBalancesAtIndex(idx, newBal)
}

// IncreaseBalanceWithVal increases validator with the given 'index' balance by 'delta' in Gwei.
// This method is flattened version of the spec method, taking in the raw balance and returning
// the post balance.
//
// Spec pseudocode definition:
//
//	def increase_balance(state: BeaconState, index: ValidatorIndex, delta: Gwei) -> None:
//	  """
//	  Increase the validator balance at index ``index`` by ``delta``.
//	  """
//	  state.balances[index] += delta
func IncreaseBalanceWithVal(currBalance, delta uint64) (uint64, error) {
	return mathutil.Add64(currBalance, delta)
}

// DecreaseBalance decreases validator with the given 'index' balance by 'delta' in Gwei.
//
// Spec pseudocode definition:
//
//	def decrease_balance(state: BeaconState, index: ValidatorIndex, delta: Gwei) -> None:
//	  """
//	  Decrease the validator balance at index ``index`` by ``delta``, with underflow protection.
//	  """
//	  state.balances[index] = 0 if delta > state.balances[index] else state.balances[index] - delta
func DecreaseBalance(state state.BeaconState, idx types.ValidatorIndex, delta uint64) error {
	isLocked, err := IsWithdrawBalanceLocked(state, idx)
	if err != nil || isLocked {
		return err
	}
	log.WithFields(log.Fields{
		"Slot":      state.Slot(),
		"Validator": idx,
		"Delta":     delta,
	}).Debug("Reward: DecreaseBalance")
	balAtIdx, err := state.BalanceAtIndex(idx)
	if err != nil {
		return err
	}
	return state.UpdateBalancesAtIndex(idx, DecreaseBalanceWithVal(balAtIdx, delta))
}

// IsWithdrawBalanceLocked define period to block all rewards and penalties operations
// of balance while procedure of exit.
func IsWithdrawBalanceLocked(state state.BeaconState, idx types.ValidatorIndex) (bool, error) {
	val, err := state.ValidatorAtIndexReadOnly(idx)
	if err != nil {
		return false, err
	}
	stateEpoche := slots.ToEpoch(state.Slot())
	lockEpoch := val.WithdrawableEpoch() - params.BeaconConfig().WithdrawalBalanceLockPeriod
	return stateEpoche >= lockEpoch && stateEpoche < val.WithdrawableEpoch(), nil
}

// DecreaseBalanceWithVal decreases validator with the given 'index' balance by 'delta' in Gwei.
// This method is flattened version of the spec method, taking in the raw balance and returning
// the post balance.
//
// Spec pseudocode definition:
//
//	def decrease_balance(state: BeaconState, index: ValidatorIndex, delta: Gwei) -> None:
//	  """
//	  Decrease the validator balance at index ``index`` by ``delta``, with underflow protection.
//	  """
//	  state.balances[index] = 0 if delta > state.balances[index] else state.balances[index] - delta
func DecreaseBalanceWithVal(currBalance, delta uint64) uint64 {
	if delta > currBalance {
		return 0
	}
	return currBalance - delta
}

// IsInInactivityLeak returns true if the state is experiencing inactivity leak.
//
// Spec code:
// def is_in_inactivity_leak(state: BeaconState) -> bool:
//
//	return get_finality_delay(state) > MIN_EPOCHS_TO_INACTIVITY_PENALTY
func IsInInactivityLeak(prevEpoch, finalizedEpoch types.Epoch) bool {
	return FinalityDelay(prevEpoch, finalizedEpoch) > params.BeaconConfig().MinEpochsToInactivityPenalty
}

// FinalityDelay returns the finality delay using the beacon state.
//
// Spec code:
// def get_finality_delay(state: BeaconState) -> uint64:
//
//	return get_previous_epoch(state) - state.finalized_checkpoint.epoch
func FinalityDelay(prevEpoch, finalizedEpoch types.Epoch) types.Epoch {
	return prevEpoch - finalizedEpoch
}

/* functionality of Reward and Penalty logging */
const (
	BalanceIncrease   = "+"
	BalanceDecrease   = "-"
	OpAttestation     = "attesting"
	OpProposing       = "blk-props"
	OpBlockAttested   = "blk-attsd"
	OpSyncCommittee   = "sync-comm"
	OpSyncAggregation = "sync-aggr"
	OpWithdrawal      = "withdrawal"
	MetaGlobalParams  = "META"
)

var (
	rpLogCache *lru.Cache
	rpLogFile  string
)

func initLogRewardsAndPenalties(st state.BeaconState) error {
	if !params.BeaconConfig().WriteRewardLogFlag {
		return nil
	}
	if rpLogCache != nil {
		return nil
	}
	vCount := st.NumValidators()
	cacheLen := int(params.BeaconConfig().SlotsPerEpoch) * vCount
	var err error
	rpLogCache, err = lru.New(cacheLen)
	if err != nil {
		panic(fmt.Errorf("lru new rpLogCache failed: %w", err))
	}

	now := time.Now()
	rpLogFileName := fmt.Sprintf("rewards-%s.log", now.Format("060102t150405"))
	rpLogFile = path.Join(params.BeaconConfig().DataDir, rpLogFileName)
	file, err := os.OpenFile(rpLogFile, os.O_CREATE|os.O_WRONLY, params.BeaconIoConfig().ReadWritePermissions) // #nosec G304
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()

	// collect metadata params
	numberOfCoordinators := vCount
	optimalNumberOfCoordinators := params.BeaconConfig().OptValidatorsNum
	stakeAmount := params.BeaconConfig().MaxEffectiveBalance / uint64(math.Pow(10, 9))
	annualizedReturnRate := params.BeaconConfig().MaxAnnualizedReturnRate
	slotDuration := params.BeaconConfig().SecondsPerSlot

	// Write the metadata params
	writer := bufio.NewWriter(file)
	_, err = fmt.Fprintf(writer, fmt.Sprintf(
		"%d %s %d %d %f %d %d %d %s %s\n",
		numberOfCoordinators,
		MetaGlobalParams,
		optimalNumberOfCoordinators,
		stakeAmount,
		annualizedReturnRate,
		slotDuration,
		-1,
		-1,
		"NOOP",
		"-",
	))
	if err != nil {
		return err
	}

	// Flush the writer to ensure the line is written to the file
	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}

// LogBalanceChanges prepare data and writes log of operation changes balance in state.
func LogBeforeRewardsAndPenalties(
	st state.BeaconState,
	validator types.ValidatorIndex,
	amount uint64,
	votesIncluded []uint64,
	operation,
	role string,
) error {
	if !params.BeaconConfig().WriteRewardLogFlag {
		return nil
	}
	if err := initLogRewardsAndPenalties(st); err != nil {
		return err
	}
	// skip zero values
	if amount == 0 {
		return nil
	}
	before, err := st.BalanceAtIndex(validator)
	if err != nil {
		return err
	}
	after := uint64(0)
	switch operation {
	case BalanceIncrease:
		after = before + amount
	case BalanceDecrease:
		after = before - amount
	default:
		return fmt.Errorf("bad operation %s", operation)
	}
	return LogBalanceChanges(validator, before, amount, after, st.Slot(), votesIncluded, operation, role)
}

// LogBalanceChanges writes log for operation changes balance in state.
func LogBalanceChanges(
	index types.ValidatorIndex,
	before, delta, after uint64,
	slot types.Slot,
	votesIncluded []uint64,
	operation, role string,
) error {
	if !params.BeaconConfig().WriteRewardLogFlag {
		return nil
	}
	//start write after 3-d epoch
	if slots.ToEpoch(slot) < 3 {
		return nil
	}
	if rpLogCache == nil {
		return fmt.Errorf("Reward and penalty logging is not initialized")
	}

	votesIncludedNum := uint64(0)
	votesList := ""
	if votesIncluded != nil {
		votesIncludedNum = uint64(len(votesIncluded))

		strNumbers := make([]string, len(votesIncluded))
		for i, num := range votesIncluded {
			strNumbers[i] = strconv.FormatUint(num, 10)
		}
		votesList = strings.Join(strNumbers, ",")
		if votesIncludedNum == 0 {
			votesList = "-"
		}
	} else {
		votesList = "-"
	}

	line := fmt.Sprintf(
		"%d %s %d %d %d %d %d %d %s %s",
		index,
		role,
		votesIncludedNum,
		before,
		delta,
		after,
		slot,
		slots.ToEpoch(slot),
		operation,
		votesList,
	)
	key := hash.FastSum64([]byte(line))
	if _, ok := rpLogCache.Get(key); ok {
		return nil
	}
	rpLogCache.Add(key, true)

	// Open the file in append mode
	file, err := os.OpenFile(rpLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, params.BeaconIoConfig().ReadWritePermissions) // #nosec G304
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()
	// Create a new writer for the file
	writer := bufio.NewWriter(file)
	// Write the new line to the file
	_, err = fmt.Fprintln(writer, line)
	if err != nil {
		return err
	}

	// Flush the writer to ensure the line is written to the file
	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}

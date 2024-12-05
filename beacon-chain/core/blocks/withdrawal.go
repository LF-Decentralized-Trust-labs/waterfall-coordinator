//Copyright 2024   Blue Wave Inc.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package blocks

import (
	"bytes"
	"context"
	"fmt"
	"math"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
)

var (
	ErrWithdrawalIsNil             = errors.New("nil withdrawal")
	ErrWithdrawalBadValidatorIndex = errors.New("withdrawal bad ValidatorIndex")
	ErrWithdrawalBadPublicKey      = errors.New("withdrawal bad PublicKey")
	ErrWithdrawalBadEpoch          = errors.New("withdrawal bad Epoch")
	ErrWithdrawalLowBalance        = errors.New("withdrawal low balance")
	ErrWithdrawalBadAmount         = errors.New("withdrawal bad amount")
	ErrWithdrawalAlreadyApplied    = errors.New("withdrawal already applied")
)

// ProcessWithdrawal is one of the operations
// performed on each processed beacon block to
// withdraw amount from validator balance.
func ProcessWithdrawal(
	ctx context.Context,
	beaconState state.BeaconState,
	withdrawals []*ethpb.Withdrawal,
) (state.BeaconState, error) {
	for _, withdrawal := range withdrawals {
		// validate withdrawal operation
		if withdrawal == nil {
			return nil, ErrWithdrawalIsNil
		}
		roVal, err := beaconState.ValidatorAtIndexReadOnly(withdrawal.ValidatorIndex)
		if err != nil {
			return nil, err
		}
		availableBalance, err := helpers.AvailableWithdrawalAmount(withdrawal.ValidatorIndex, beaconState)
		if err != nil {
			return nil, err
		}
		if err = VerifyWithdrawalData(withdrawal, roVal, beaconState.Slot(), availableBalance); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"VIndex":           fmt.Sprintf("%d", withdrawal.ValidatorIndex),
				"PublicKey":        fmt.Sprintf("%#x", withdrawal.PublicKey),
				"Epoch":            fmt.Sprintf("%d", withdrawal.Epoch),
				"Amount":           fmt.Sprintf("%d", withdrawal.Amount),
				"InitTxHash":       fmt.Sprintf("%#x", withdrawal.InitTxHash),
				"availableBalance": availableBalance,
			}).Error("Withdrawal transition: validation failed")

			// if withdrawal already applied - skip err
			if err == ErrWithdrawalAlreadyApplied {
				return beaconState, nil
			}
			return nil, errors.Wrapf(err, "withdrawal verify %d", withdrawal.ValidatorIndex)
		}

		// apply withdrawal
		amount := withdrawal.Amount
		// if amount is zero - withdraw the entire available balance
		if amount == 0 {
			amount = availableBalance
		}

		// write Rewards And Penalties log
		if err = helpers.LogBeforeRewardsAndPenalties(
			beaconState,
			withdrawal.ValidatorIndex,
			amount,
			nil,
			helpers.BalanceDecrease,
			helpers.OpWithdrawal,
		); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"Slot":       beaconState.Slot(),
				"Validator":  withdrawal.ValidatorIndex,
				"withdrawal": amount,
			}).Error("Log rewards and penalties failed: ProcessWithdrawal")
		}

		// update balance
		if err = ApplyWithdrawals(beaconState, withdrawal.ValidatorIndex, amount); err != nil {
			return nil, err
		}
		// update validator
		upVal, err := beaconState.ValidatorAtIndex(withdrawal.ValidatorIndex)
		if err != nil {
			return nil, err
		}
		if upVal.WithdrawalOps == nil {
			upVal.WithdrawalOps = make([]*ethpb.WithdrawalOp, 1)
		}
		// check WithdrawalOps length limit
		if uint64(len(upVal.WithdrawalOps)) >= params.BeaconConfig().WithdrawalOpsLimit {
			upVal.WithdrawalOps = upVal.WithdrawalOps[1:]
		}

		upVal.WithdrawalOps = append(upVal.WithdrawalOps, &ethpb.WithdrawalOp{
			Amount: amount,
			Hash:   bytesutil.SafeCopyBytes(withdrawal.InitTxHash),
			Slot:   beaconState.Slot(),
		})
		if err = beaconState.UpdateValidatorAtIndex(withdrawal.ValidatorIndex, upVal); err != nil {
			return nil, err
		}

		log.WithFields(logrus.Fields{
			"stSlot":           beaconState.Slot(),
			"VIndex":           fmt.Sprintf("%d", withdrawal.ValidatorIndex),
			"PublicKey":        fmt.Sprintf("%#x", withdrawal.PublicKey),
			"Epoch":            fmt.Sprintf("%d", withdrawal.Epoch),
			"Amount":           fmt.Sprintf("%d", withdrawal.Amount),
			"InitTxHash":       fmt.Sprintf("%#x", withdrawal.InitTxHash),
			"availableBalance": availableBalance,
			"WithdrawalOps":    len(upVal.WithdrawalOps),
		}).Info("Withdrawal transition: success")
	}
	return beaconState, nil
}

func VerifyWithdrawalData(
	withdrawal *ethpb.Withdrawal,
	validator state.ReadOnlyValidator,
	currentSlot types.Slot,
	availableBalance uint64,
) error {
	if withdrawal == nil {
		return ErrWithdrawalIsNil
	}
	if withdrawal.ValidatorIndex == math.MaxUint64 {
		return ErrWithdrawalBadValidatorIndex
	}

	if bytesutil.ToBytes48(withdrawal.PublicKey) != validator.PublicKey() {
		return ErrWithdrawalBadPublicKey
	}

	currentEpoch := slots.ToEpoch(currentSlot)
	if withdrawal.Epoch > currentEpoch {
		return ErrWithdrawalBadEpoch
	}

	// refunds of insufficient deposit to activate validator
	if validator.ActivationEligibilityEpoch() == params.BeaconConfig().FarFutureEpoch &&
		availableBalance < params.BeaconConfig().MaxEffectiveBalance {
		// for this mod amount must be strictly defined by shard node.
		// withdrawal whole balance (by set op.Amount to 0) is not acceptable.
		if withdrawal.Amount == 0 {
			return ErrWithdrawalBadAmount
		}
	}

	if availableBalance < withdrawal.Amount {
		return ErrWithdrawalLowBalance
	}

	for _, v := range validator.WithdrawalOps() {
		if bytes.Equal(v.Hash, withdrawal.InitTxHash) {
			return ErrWithdrawalAlreadyApplied
		}
	}

	return nil
}

func ApplyWithdrawals(state state.BeaconState, idx types.ValidatorIndex, delta uint64) error {
	balAtIdx, err := state.BalanceAtIndex(idx)
	if err != nil {
		return err
	}
	return state.UpdateBalancesAtIndex(idx, helpers.DecreaseBalanceWithVal(balAtIdx, delta))
}

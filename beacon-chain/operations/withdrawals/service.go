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

package withdrawals

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	types "github.com/prysmaticlabs/eth2-types"
	log "github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	"go.opencensus.io/trace"
)

// PoolManager maintains pending and seen withdrawals.
// This pool is used by proposers to insert withdrawals into new blocks.
type PoolManager interface {
	PendingWithdrawals(slot types.Slot, st state.ReadOnlyBeaconState, noLimit bool) []*ethpb.Withdrawal
	InsertWithdrawal(ctx context.Context, withdrawal *ethpb.Withdrawal)
	MarkIncluded(withdrawal *ethpb.Withdrawal)
	OnSlot(st state.ReadOnlyBeaconState)
	Verify(withdrawal *ethpb.Withdrawal) error
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock    sync.RWMutex
	pending []*ethpb.Withdrawal
}

// NewPool accepts a head fetcher (for reading the validator set) and returns an initialized
// withdrawals pool.
func NewPool() *Pool {
	return &Pool{
		pending: make([]*ethpb.Withdrawal, 0),
	}
}

// PendingWithdrawals returns withdrawals that are ready for inclusion at the given slot. This method will not
// return more than the block enforced MaxWithdrawals.
func (p *Pool) PendingWithdrawals(slot types.Slot, st state.ReadOnlyBeaconState, noLimit bool) []*ethpb.Withdrawal {
	p.lock.RLock()
	defer p.lock.RUnlock()

	// Allocate pending slice with a capacity of min(len(p.pending), maxWithdrawals) since the
	// array cannot exceed the max and is typically less than the max value.
	maxWithdrawals := params.BeaconConfig().MaxWithdrawals
	if noLimit {
		maxWithdrawals = uint64(len(p.pending))
	}
	availBalMap := map[types.ValidatorIndex]uint64{}
	pending := make([]*ethpb.Withdrawal, 0, maxWithdrawals)
	for _, itm := range p.pending {
		// not activated validator withdrawal
		if itm.ValidatorIndex == math.MaxUint64 {
			continue
		}
		if itm.Epoch > slots.ToEpoch(slot) {
			continue
		}
		// check in withdrawal already in state
		roVal, err := st.ValidatorAtIndexReadOnly(itm.ValidatorIndex)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"VIndex":     fmt.Sprintf("%d", itm.ValidatorIndex),
				"PublicKey":  fmt.Sprintf("%#x", itm.PublicKey),
				"Epoch":      fmt.Sprintf("%d", itm.Epoch),
				"Amount":     fmt.Sprintf("%d", itm.Amount),
				"InitTxHash": fmt.Sprintf("%#x", itm.InitTxHash),
			}).Error("WithdrawalPool pool: get pending: get validator err")
			continue
		}
		for _, wop := range roVal.WithdrawalOps() {
			if bytes.Equal(wop.Hash, itm.InitTxHash) {
				log.WithFields(log.Fields{
					"VIndex":       fmt.Sprintf("%d", itm.ValidatorIndex),
					"PublicKey":    fmt.Sprintf("%#x", itm.PublicKey),
					"Epoch":        fmt.Sprintf("%d", itm.Epoch),
					"Amount":       fmt.Sprintf("%d", itm.Amount),
					"availBalance": fmt.Sprintf("%d", availBalMap[itm.ValidatorIndex]),
					"InitTxHash":   fmt.Sprintf("%#x", itm.InitTxHash),
				}).Error("WithdrawalPool pool: get pending: skip already handled item item")
				continue
			}
		}
		// check available balance
		if _, ok := availBalMap[itm.ValidatorIndex]; !ok {
			bal, err := helpers.AvailableWithdrawalAmount(itm.ValidatorIndex, st)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"VIndex":     fmt.Sprintf("%d", itm.ValidatorIndex),
					"PublicKey":  fmt.Sprintf("%#x", itm.PublicKey),
					"Epoch":      fmt.Sprintf("%d", itm.Epoch),
					"Amount":     fmt.Sprintf("%d", itm.Amount),
					"InitTxHash": fmt.Sprintf("%#x", itm.InitTxHash),
				}).Error("WithdrawalPool pool: get pending: check balance err")
				continue
			}
			availBalMap[itm.ValidatorIndex] = bal
		}
		if availBalMap[itm.ValidatorIndex] < itm.Amount {
			log.WithFields(log.Fields{
				"VIndex":       fmt.Sprintf("%d", itm.ValidatorIndex),
				"PublicKey":    fmt.Sprintf("%#x", itm.PublicKey),
				"Epoch":        fmt.Sprintf("%d", itm.Epoch),
				"Amount":       fmt.Sprintf("%d", itm.Amount),
				"availBalance": fmt.Sprintf("%d", availBalMap[itm.ValidatorIndex]),
				"InitTxHash":   fmt.Sprintf("%#x", itm.InitTxHash),
			}).Error("WithdrawalPool pool: get pending: skip item due to low available balance")
			continue
		}
		availBalMap[itm.ValidatorIndex] -= itm.Amount

		pending = append(pending, itm)
		if uint64(len(pending)) == maxWithdrawals {
			break
		}
	}
	return pending
}

// InsertWithdrawal into the pool. This method is a no-op if the pending withdrawal already exists,
// or the validator is already withdrawaled.
func (p *Pool) InsertWithdrawal(ctx context.Context, withdrawal *ethpb.Withdrawal) {
	_, span := trace.StartSpan(ctx, "withdrawalPool.InsertWithdrawal")
	defer span.End()
	p.lock.Lock()
	defer p.lock.Unlock()

	// Prevent malformed messages from being inserted.
	if withdrawal == nil {
		return
	}
	if withdrawal.InitTxHash == nil {
		log.WithFields(log.Fields{
			"VIndex":     fmt.Sprintf("%d", withdrawal.ValidatorIndex),
			"PublicKey":  fmt.Sprintf("%#x", withdrawal.PublicKey),
			"Epoch":      fmt.Sprintf("%d", withdrawal.Epoch),
			"Amount":     fmt.Sprintf("%d", withdrawal.Amount),
			"InitTxHash": fmt.Sprintf("%#x", withdrawal.InitTxHash),
		}).Warn("WithdrawalPool pool: insert malformed data: InitTxHash")
		return
	}

	//check exists
	if exists, _ := existsInList(p.pending, withdrawal); exists {
		return
	}

	// Insert into pending list and sort.
	p.pending = append(p.pending, withdrawal)
	sort.Slice(p.pending, func(i, j int) bool {
		return p.pending[i].Epoch < p.pending[j].Epoch
	})
}

// MarkIncluded is used when an withdrawal has been included in a beacon block. Every block seen by this
// node should call this method to include the withdrawal. This will remove the withdrawal from
// the pending withdrawals slice.
func (p *Pool) MarkIncluded(withdrawal *ethpb.Withdrawal) {
	p.lock.Lock()
	defer p.lock.Unlock()
	exists, index := existsInList(p.pending, withdrawal)
	if exists {
		// WithdrawalPool we want is present at p.pending[index], so we remove it.
		p.pending = append(p.pending[:index], p.pending[index+1:]...)
	}
}

func (p *Pool) Verify(withdrawal *ethpb.Withdrawal) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	exists, index := existsInList(p.pending, withdrawal)
	if !exists {
		return fmt.Errorf("not found")
	}
	poolItm := p.pending[index]

	//if poolItm.Epoch != withdrawal.Epoch {
	//	return fmt.Errorf("mismatch epochs pool=%d received=%d", poolItm.Epoch, withdrawal.Epoch)
	//}
	if poolItm.Amount != withdrawal.Amount {
		return fmt.Errorf("mismatch amounts pool=%d received=%d", poolItm.Amount, withdrawal.Amount)
	}
	if poolItm.ValidatorIndex != withdrawal.ValidatorIndex {
		return fmt.Errorf("mismatch validators indices pool=%d received=%d", poolItm.ValidatorIndex, withdrawal.ValidatorIndex)
	}
	if !bytes.Equal(poolItm.PublicKey, withdrawal.PublicKey) {
		return fmt.Errorf("mismatch publick keys pool=%#x received=%#x", poolItm.PublicKey, withdrawal.PublicKey)
	}
	if !bytes.Equal(poolItm.InitTxHash, withdrawal.InitTxHash) {
		return fmt.Errorf("mismatch init tx hashes pool=%#x received=%#x", poolItm.InitTxHash, withdrawal.InitTxHash)
	}
	return nil
}

// Binary search to check if the index exists in the list of pending withdrawals.
func existsInList(pending []*ethpb.Withdrawal, withdrawal *ethpb.Withdrawal) (bool, int) {
	if withdrawal == nil {
		return false, -1
	}
	for i, w := range pending {
		if bytes.Equal(w.InitTxHash, withdrawal.InitTxHash) {
			return true, i
		}
	}
	return false, -1
}

// OnSlot removes invalid items from pool
func (p *Pool) OnSlot(st state.ReadOnlyBeaconState) {
	p.lock.Lock()
	defer p.lock.Unlock()

	// check validator activation
	p.handleValidatorActivation(st)

	// remove invalid or stale items
	pending := make([]*ethpb.Withdrawal, 0, len(p.pending))
	for _, itm := range p.pending {
		if err := validateWithdrawal(itm, st); err == nil {
			pending = append(pending, itm)
		} else {
			log.WithError(err).WithFields(log.Fields{
				"VIndex":     fmt.Sprintf("%d", itm.ValidatorIndex),
				"PublicKey":  fmt.Sprintf("%#x", itm.PublicKey),
				"Epoch":      fmt.Sprintf("%d", itm.Epoch),
				"Amount":     fmt.Sprintf("%d", itm.Amount),
				"InitTxHash": fmt.Sprintf("%#x", itm.InitTxHash),
			}).Warn("WithdrawalPool pool: validation failed")
		}
	}
	p.pending = pending
}

// handleValidatorActivation set validator index for activated validators.
func (p *Pool) handleValidatorActivation(st state.ReadOnlyBeaconState) {
	for i, itm := range p.pending {
		if itm.ValidatorIndex != math.MaxUint64 {
			continue
		}
		vix, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(itm.PublicKey))
		if ok {
			p.pending[i].ValidatorIndex = vix
			log.WithFields(log.Fields{
				"PublicKey":  fmt.Sprintf("%#x", itm.PublicKey),
				"VIndex":     fmt.Sprintf("%d", itm.ValidatorIndex),
				"Epoch":      fmt.Sprintf("%d", itm.Epoch),
				"Amount":     fmt.Sprintf("%d", itm.Amount),
				"InitTxHash": fmt.Sprintf("%#x", itm.InitTxHash),
			}).Info("WithdrawalPool pool: validator activation")
		}
	}
}

func validateWithdrawal(itm *ethpb.Withdrawal, st state.ReadOnlyBeaconState) error {
	// stale not activated validator withdrawal
	if itm.ValidatorIndex == math.MaxUint64 && itm.Epoch < st.FinalizedCheckpointEpoch() {
		return fmt.Errorf("stale not activated validator")
	}
	// not activated validator withdrawal
	if itm.ValidatorIndex == math.MaxUint64 {
		return nil
	}
	// check validator data
	vld, err := st.ValidatorAtIndexReadOnly(itm.ValidatorIndex)
	if err != nil {
		return err
	}
	if vld == nil {
		return fmt.Errorf("validator not found by index=%d", itm.ValidatorIndex)
	}
	if vld.PublicKey() != bytesutil.ToBytes48(itm.PublicKey) {
		return fmt.Errorf("mismatch public keys validator=%#x withdrawal=%#x", vld.PublicKey(), itm.PublicKey)
	}
	for _, wop := range vld.WithdrawalOps() {
		if bytes.Equal(wop.Hash, itm.InitTxHash) {
			return fmt.Errorf("withdrawal already applied")
		}
	}

	// check validator balance
	bal, err := helpers.AvailableWithdrawalAmount(itm.ValidatorIndex, st)
	if err != nil {
		return err
	}
	if bal < itm.Amount {
		return fmt.Errorf("validate: low balance bal=%d < amt=%d", bal, itm.Amount)
	}
	return nil
}

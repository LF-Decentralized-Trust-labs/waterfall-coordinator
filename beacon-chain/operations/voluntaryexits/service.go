package voluntaryexits

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"

	types "github.com/prysmaticlabs/eth2-types"
	log "github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	"go.opencensus.io/trace"
)

// PoolManager maintains pending and seen voluntary exits.
// This pool is used by proposers to insert voluntary exits into new blocks.
type PoolManager interface {
	PendingExits(state state.ReadOnlyBeaconState, slot types.Slot, noLimit bool) []*ethpb.VoluntaryExit
	InsertVoluntaryExitByGwat(ctx context.Context, exit *ethpb.VoluntaryExit)
	MarkIncluded(exit *ethpb.VoluntaryExit)
	OnSlot(st state.ReadOnlyBeaconState)
	Verify(exit *ethpb.VoluntaryExit) error
	CopyItems() []*ethpb.VoluntaryExit
	// Deprecated
	InsertVoluntaryExit(ctx context.Context, state state.ReadOnlyBeaconState, exit *ethpb.VoluntaryExit)
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock    sync.RWMutex
	pending []*ethpb.VoluntaryExit
}

// NewPool accepts a head fetcher (for reading the validator set) and returns an initialized
// voluntary exit pool.
func NewPool() *Pool {
	return &Pool{
		pending: make([]*ethpb.VoluntaryExit, 0),
	}
}

// PendingExits returns exits that are ready for inclusion at the given slot. This method will not
// return more than the block enforced MaxVoluntaryExits.
func (p *Pool) PendingExits(state state.ReadOnlyBeaconState, slot types.Slot, noLimit bool) []*ethpb.VoluntaryExit {
	p.lock.RLock()
	defer p.lock.RUnlock()

	// Allocate pending slice with a capacity of min(len(p.pending), maxVoluntaryExits) since the
	// array cannot exceed the max and is typically less than the max value.
	maxExits := params.BeaconConfig().MaxVoluntaryExits
	if noLimit {
		maxExits = uint64(len(p.pending))
	}
	pending := make([]*ethpb.VoluntaryExit, 0, maxExits)
	for _, e := range p.pending {
		if e.Epoch > slots.ToEpoch(slot) {
			continue
		}
		if v, err := state.ValidatorAtIndexReadOnly(e.ValidatorIndex); err == nil &&
			v.ExitEpoch() == params.BeaconConfig().FarFutureEpoch {
			pending = append(pending, e)
			if uint64(len(pending)) == maxExits {
				break
			}
		}
	}
	return pending
}

// InsertVoluntaryExit into the pool. This method is a no-op if the pending exit already exists,
// or the validator is already exited.
// Deprecated
func (p *Pool) InsertVoluntaryExit(ctx context.Context, state state.ReadOnlyBeaconState, exit *ethpb.VoluntaryExit) {
	_, span := trace.StartSpan(ctx, "exitPool.InsertVoluntaryExit")
	defer span.End()
	p.lock.Lock()
	defer p.lock.Unlock()

	// Prevent malformed messages from being inserted.
	if exit == nil {
		return
	}

	existsInPending, index := existsInList(p.pending, exit.ValidatorIndex)
	// If the item exists in the pending list and includes a more favorable, earlier
	// exit epoch, we replace it in the pending list. If it exists but the prior condition is false,
	// we simply return.
	if existsInPending {
		if exit.Epoch < p.pending[index].Epoch {
			p.pending[index] = exit
		}
		return
	}

	// Has the validator been exited already?
	if v, err := state.ValidatorAtIndexReadOnly(exit.ValidatorIndex); err != nil ||
		v.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
		return
	}

	// Insert into pending list and sort.
	p.pending = append(p.pending, exit)
	sort.Slice(p.pending, func(i, j int) bool {
		return p.pending[i].ValidatorIndex < p.pending[j].ValidatorIndex
	})
}

// InsertVoluntaryExit into the pool. This method is a no-op if the pending exit already exists,
// or the validator is already exited.
func (p *Pool) InsertVoluntaryExitByGwat(ctx context.Context, exit *ethpb.VoluntaryExit) {
	_, span := trace.StartSpan(ctx, "exitPool.InsertVoluntaryExit")
	defer span.End()
	p.lock.Lock()
	defer p.lock.Unlock()

	// Prevent malformed messages from being inserted.
	if exit == nil {
		return
	}
	if exit.InitTxHash == nil {
		log.WithFields(log.Fields{
			"InitTxHash": exit.InitTxHash,
		}).Warn("InsertVoluntaryExitByGwat malformed data: InitTxHash")
		return
	}

	existsInPending, index := existsInList(p.pending, exit.ValidatorIndex)
	// If the item exists in the pending list and includes a more favorable, earlier
	// exit epoch, we replace it in the pending list. If it exists but the prior condition is false,
	// we simply return.
	if existsInPending {
		if exit.Epoch < p.pending[index].Epoch {
			p.pending[index] = exit
		}
		return
	}

	// Insert into pending list and sort.
	p.pending = append(p.pending, exit)
	sort.Slice(p.pending, func(i, j int) bool {
		return p.pending[i].ValidatorIndex < p.pending[j].ValidatorIndex
	})
}

// MarkIncluded is used when an exit has been included in a beacon block. Every block seen by this
// node should call this method to include the exit. This will remove the exit from
// the pending exits slice.
func (p *Pool) MarkIncluded(exit *ethpb.VoluntaryExit) {
	p.lock.Lock()
	defer p.lock.Unlock()
	exists, index := existsInList(p.pending, exit.ValidatorIndex)
	if exists {
		// Exit we want is present at p.pending[index], so we remove it.
		p.pending = append(p.pending[:index], p.pending[index+1:]...)
	}
}

func (p *Pool) Verify(exit *ethpb.VoluntaryExit) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	exists, index := existsInList(p.pending, exit.ValidatorIndex)
	if !exists {
		return fmt.Errorf("not found")
	}
	poolItm := p.pending[index]

	//if poolItm.Epoch != exit.Epoch {
	//	return fmt.Errorf("mismatch epochs pool=%d received=%d", poolItm.Epoch, exit.Epoch)
	//}
	if poolItm.ValidatorIndex != exit.ValidatorIndex {
		return fmt.Errorf("mismatch validators indices pool=%d received=%d", poolItm.ValidatorIndex, exit.ValidatorIndex)
	}
	if !bytes.Equal(poolItm.InitTxHash, exit.InitTxHash) {
		return fmt.Errorf("mismatch init tx hashes pool=%#x received=%#x", poolItm.InitTxHash, exit.InitTxHash)
	}
	return nil
}

// Binary search to check if the index exists in the list of pending exits.
func existsInList(pending []*ethpb.VoluntaryExit, searchingFor types.ValidatorIndex) (bool, int) {
	i := sort.Search(len(pending), func(j int) bool {
		return pending[j].ValidatorIndex >= searchingFor
	})
	if i < len(pending) && pending[i].ValidatorIndex == searchingFor {
		return true, i
	}
	return false, -1
}

// OnSlot removes invalid items from pool
func (p *Pool) OnSlot(st state.ReadOnlyBeaconState) {
	p.lock.Lock()
	defer p.lock.Unlock()

	pending := make([]*ethpb.VoluntaryExit, 0, len(p.pending))
	for _, itm := range p.pending {
		if validateVoluntaryExit(itm, st) {
			pending = append(pending, itm)
		}
	}
	p.pending = pending
}

func validateVoluntaryExit(itm *ethpb.VoluntaryExit, st state.ReadOnlyBeaconState) bool {
	v, err := st.ValidatorAtIndexReadOnly(itm.ValidatorIndex)
	if err != nil || v == nil || v.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
		return false
	}
	return true
}

func (p *Pool) CopyItems() []*ethpb.VoluntaryExit {
	p.lock.RLock()
	defer p.lock.RUnlock()

	result := make([]*ethpb.VoluntaryExit, len(p.pending))
	for i, op := range p.pending {
		result[i] = &ethpb.VoluntaryExit{
			Epoch:          op.Epoch,
			ValidatorIndex: op.ValidatorIndex,
			InitTxHash:     bytesutil.SafeCopyBytes(op.InitTxHash),
		}
	}
	return result
}

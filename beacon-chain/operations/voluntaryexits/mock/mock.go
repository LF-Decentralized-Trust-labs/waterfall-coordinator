package mock

import (
	"bytes"
	"context"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	eth "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Exits []*eth.VoluntaryExit
}

func (m *PoolMock) OnSlot(st state.ReadOnlyBeaconState) {
	//TODO implement me
	panic("implement me")
}

func (m *PoolMock) Verify(exit *eth.VoluntaryExit) error {
	//TODO implement me
	panic("implement me")
}

func (m *PoolMock) InsertVoluntaryExitByGwat(ctx context.Context, exit *eth.VoluntaryExit) {
	m.Exits = append(m.Exits, exit)
}

// PendingExits --
func (m *PoolMock) PendingExits(_ state.ReadOnlyBeaconState, _ types.Slot, _ bool) []*eth.VoluntaryExit {
	return m.Exits
}

func (m *PoolMock) CopyItems() []*eth.VoluntaryExit {
	return m.Exits
}

// InsertVoluntaryExit --
func (m *PoolMock) InsertVoluntaryExit(_ context.Context, _ state.ReadOnlyBeaconState, exit *eth.VoluntaryExit) {
	m.Exits = append(m.Exits, exit)
}

// MarkIncluded --
func (m *PoolMock) MarkIncluded(exit *eth.VoluntaryExit) {
	res := make([]*eth.VoluntaryExit, 0, len(m.Exits))
	for _, w := range m.Exits {
		if bytes.Equal(w.InitTxHash, exit.InitTxHash) {
			continue
		}
		res = append(res, w)
	}
	m.Exits = res
}

package testutil

import (
	"time"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
)

// MockGenesisTimeFetcher is a fake implementation of the blockchain.TimeFetcher
type MockGenesisTimeFetcher struct {
	Genesis time.Time
}

func (m *MockGenesisTimeFetcher) GenesisTime() time.Time {
	return m.Genesis
}

func (m *MockGenesisTimeFetcher) CurrentSlot() types.Slot {
	return types.Slot(uint64(time.Now().Unix()-m.Genesis.Unix()) / params.BeaconConfig().SecondsPerSlot)
}

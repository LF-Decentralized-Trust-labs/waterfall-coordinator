package testing

import (
	"context"
	"math/big"

	"github.com/pkg/errors"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/async/event"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

// FaultyMockPOWChain defines an incorrectly functioning powchain service.
type FaultyMockPOWChain struct {
	ChainFeed      *event.Feed
	HashesByHeight map[int][]byte
}

// Eth2GenesisPowchainInfo --
func (_ *FaultyMockPOWChain) Eth2GenesisPowchainInfo() (uint64, *big.Int) {
	return 0, big.NewInt(0)
}

// BlockExists --
func (f *FaultyMockPOWChain) BlockExists(_ context.Context, _ common.Hash) (bool, *big.Int, error) {
	if f.HashesByHeight == nil {
		return false, big.NewInt(1), errors.New("failed")
	}

	return true, big.NewInt(1), nil
}

// BlockHashByHeight --
func (_ *FaultyMockPOWChain) BlockHashByHeight(_ context.Context, _ *big.Int) (common.Hash, error) {
	return [32]byte{}, errors.New("failed")
}

// BlockTimeByHeight --
func (_ *FaultyMockPOWChain) BlockTimeByHeight(_ context.Context, _ *big.Int) (uint64, error) {
	return 0, errors.New("failed")
}

// ChainStartEth1Data --
func (_ *FaultyMockPOWChain) ChainStartEth1Data() *ethpb.Eth1Data {
	return &ethpb.Eth1Data{}
}

// PreGenesisState --
func (_ *FaultyMockPOWChain) PreGenesisState() state.BeaconState {
	s, err := v1.InitializeFromProtoUnsafe(&ethpb.BeaconState{})
	if err != nil {
		panic("could not initialize state")
	}
	return s
}

// ClearPreGenesisData --
func (_ *FaultyMockPOWChain) ClearPreGenesisData() {
	// no-op
}

// IsConnectedToETH1 --
func (_ *FaultyMockPOWChain) IsConnectedToETH1() bool {
	return true
}

// BlockExistsWithCache --
func (f *FaultyMockPOWChain) BlockExistsWithCache(ctx context.Context, hash common.Hash) (bool, *big.Int, error) {
	return f.BlockExists(ctx, hash)
}

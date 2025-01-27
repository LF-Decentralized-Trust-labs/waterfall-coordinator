package v2

import (
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

// Eth1Data corresponding to the proof-of-work chain information stored in the beacon state.
func (b *BeaconState) Eth1Data() *ethpb.Eth1Data {
	if !b.hasInnerState() {
		return nil
	}
	if b.state.Eth1Data == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.eth1Data()
}

// eth1Data corresponding to the proof-of-work chain information stored in the beacon state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) eth1Data() *ethpb.Eth1Data {
	if !b.hasInnerState() {
		return nil
	}
	if b.state.Eth1Data == nil {
		return nil
	}

	return ethpb.CopyETH1Data(b.state.Eth1Data)
}

// Eth1DataVotes corresponds to votes from coordinator on the canonical shard chain
// data retrieved from eth1.
func (b *BeaconState) Eth1DataVotes() []*ethpb.Eth1Data {
	if !b.hasInnerState() {
		return nil
	}
	if b.state.Eth1DataVotes == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.eth1DataVotes()
}

// eth1DataVotes corresponds to votes from coordinator on the canonical shard chain
// data retrieved from eth1.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) eth1DataVotes() []*ethpb.Eth1Data {
	if !b.hasInnerState() {
		return nil
	}
	if b.state.Eth1DataVotes == nil {
		return nil
	}

	res := make([]*ethpb.Eth1Data, len(b.state.Eth1DataVotes))
	for i := 0; i < len(res); i++ {
		res[i] = ethpb.CopyETH1Data(b.state.Eth1DataVotes[i])
	}
	return res
}

// Eth1DepositIndex corresponds to the index of the deposit made to the
// validator deposit contract at the time of this state's eth1 data.
func (b *BeaconState) Eth1DepositIndex() uint64 {
	if !b.hasInnerState() {
		return 0
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.eth1DepositIndex()
}

// eth1DepositIndex corresponds to the index of the deposit made to the
// validator deposit contract at the time of this state's eth1 data.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) eth1DepositIndex() uint64 {
	if !b.hasInnerState() {
		return 0
	}

	return b.state.Eth1DepositIndex
}

// BlockVoting .
func (b *BeaconState) BlockVoting() []*ethpb.BlockVoting {
	if !b.hasInnerState() {
		return nil
	}
	if b.state.BlockVoting == nil {
		return nil
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.blockVoting()
}

// BlockVoting.
func (b *BeaconState) blockVoting() []*ethpb.BlockVoting {
	if !b.hasInnerState() {
		return nil
	}
	if b.state.BlockVoting == nil {
		return nil
	}
	res := make([]*ethpb.BlockVoting, len(b.state.BlockVoting))
	for i := 0; i < len(res); i++ {
		res[i] = ethpb.CopyBlockVoting(b.state.BlockVoting[i])
	}
	return res
}

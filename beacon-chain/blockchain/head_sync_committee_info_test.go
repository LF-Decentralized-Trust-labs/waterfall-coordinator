package blockchain

import (
	"context"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	dbtest "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/stategen"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
)

func TestService_headSyncCommitteeFetcher_Errors(t *testing.T) {
	beaconDB := dbtest.SetupDB(t)
	c := &Service{
		cfg: &config{
			StateGen: stategen.New(beaconDB),
		},
	}
	c.head = &head{}
	_, err := c.headCurrentSyncCommitteeIndices(context.Background(), types.ValidatorIndex(0), types.Slot(0))
	require.ErrorContains(t, "nil state", err)

	_, err = c.headNextSyncCommitteeIndices(context.Background(), types.ValidatorIndex(0), types.Slot(0))
	require.ErrorContains(t, "nil state", err)

	_, err = c.HeadSyncCommitteePubKeys(context.Background(), types.Slot(0), types.CommitteeIndex(0))
	require.ErrorContains(t, "nil state", err)
}

func TestService_HeadDomainFetcher_Errors(t *testing.T) {
	beaconDB := dbtest.SetupDB(t)
	c := &Service{
		cfg: &config{
			StateGen: stategen.New(beaconDB),
		},
	}
	c.head = &head{}
	_, err := c.HeadSyncCommitteeDomain(context.Background(), types.Slot(0))
	require.ErrorContains(t, "nil state", err)

	_, err = c.HeadSyncSelectionProofDomain(context.Background(), types.Slot(0))
	require.ErrorContains(t, "nil state", err)

	_, err = c.HeadSyncSelectionProofDomain(context.Background(), types.Slot(0))
	require.ErrorContains(t, "nil state", err)
}

func TestService_HeadSyncCommitteeIndices(t *testing.T) {
	s, _ := util.DeterministicGenesisState(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{}
	c.head = &head{state: s}

	// Current period
	slot := 2*uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	a, err := c.HeadSyncCommitteeIndices(context.Background(), 0, types.Slot(slot))
	require.NoError(t, err)

	// Current period where slot-2 across EPOCHS_PER_SYNC_COMMITTEE_PERIOD
	slot = 3*uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) - 2
	b, err := c.HeadSyncCommitteeIndices(context.Background(), 0, types.Slot(slot))
	require.NoError(t, err)
	require.DeepEqual(t, a, b)
}

func TestService_headCurrentSyncCommitteeIndices(t *testing.T) {
	s, _ := util.DeterministicGenesisState(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{}
	c.head = &head{state: s}

	// Process slot up to `EpochsPerSyncCommitteePeriod` so it can `ProcessSyncCommitteeUpdates`.
	slot := uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	indices, err := c.headCurrentSyncCommitteeIndices(context.Background(), 0, types.Slot(slot))
	require.NoError(t, err)

	// NextSyncCommittee becomes CurrentSyncCommittee so it should be empty by default.
	require.Equal(t, 4, len(indices))
}

func TestService_headNextSyncCommitteeIndices(t *testing.T) {
	s, _ := util.DeterministicGenesisState(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{}
	c.head = &head{state: s}

	// Process slot up to `EpochsPerSyncCommitteePeriod` so it can `ProcessSyncCommitteeUpdates`.
	slot := uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	indices, err := c.headNextSyncCommitteeIndices(context.Background(), 0, types.Slot(slot))
	require.NoError(t, err)

	// NextSyncCommittee should be be empty after `ProcessSyncCommitteeUpdates`. Validator should get indices.
	require.NotEqual(t, 0, len(indices))
}

func TestService_HeadSyncCommitteePubKeys(t *testing.T) {
	s, _ := util.DeterministicGenesisState(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{}
	c.head = &head{state: s}

	// Process slot up to 2 * `EpochsPerSyncCommitteePeriod` so it can run `ProcessSyncCommitteeUpdates` twice.
	slot := uint64(2*params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	pubkeys, err := c.HeadSyncCommitteePubKeys(context.Background(), types.Slot(slot), 0)
	require.NoError(t, err)

	// Any subcommittee should match the subcommittee size.
	subCommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount
	require.Equal(t, int(subCommitteeSize), len(pubkeys))
}

func TestService_HeadSyncCommitteeDomain(t *testing.T) {
	t.Skip()
	s, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{}
	c.head = &head{state: s}

	wanted, err := signing.Domain(s.Fork(), slots.ToEpoch(s.Slot()), params.BeaconConfig().DomainSyncCommittee, s.GenesisValidatorsRoot())
	require.NoError(t, err)

	d, err := c.HeadSyncCommitteeDomain(context.Background(), 0)
	require.NoError(t, err)

	require.DeepEqual(t, wanted, d)
}

func TestService_HeadSyncContributionProofDomain(t *testing.T) {
	t.Skip()
	s, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{}
	c.head = &head{state: s}

	wanted, err := signing.Domain(s.Fork(), slots.ToEpoch(s.Slot()), params.BeaconConfig().DomainContributionAndProof, s.GenesisValidatorsRoot())
	require.NoError(t, err)

	d, err := c.HeadSyncContributionProofDomain(context.Background(), 0)
	require.NoError(t, err)

	require.DeepEqual(t, wanted, d)
}

func TestService_HeadSyncSelectionProofDomain(t *testing.T) {
	s, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{}
	c.head = &head{state: s}

	wanted, err := signing.Domain(s.Fork(), slots.ToEpoch(s.Slot()), params.BeaconConfig().DomainSyncCommitteeSelectionProof, s.GenesisValidatorsRoot())
	require.NoError(t, err)

	d, err := c.HeadSyncSelectionProofDomain(context.Background(), 0)
	require.NoError(t, err)

	require.DeepEqual(t, wanted, d)
}

func TestSyncCommitteeHeadStateCache_RoundTrip(t *testing.T) {
	c := syncCommitteeHeadStateCache
	t.Cleanup(func() {
		syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState()
	})
	beaconState, _ := util.DeterministicGenesisStateAltair(t, 100)
	require.NoError(t, beaconState.SetSlot(100))
	cachedState, err := c.Get(101)
	require.ErrorContains(t, cache.ErrNotFound.Error(), err)
	require.Equal(t, nil, cachedState)
	require.NoError(t, c.Put(101, beaconState))
	cachedState, err = c.Get(101)
	require.NoError(t, err)
	require.DeepEqual(t, beaconState, cachedState)
}

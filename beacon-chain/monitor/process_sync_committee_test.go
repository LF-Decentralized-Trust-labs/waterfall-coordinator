package monitor

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestProcessSyncCommitteeContribution(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)

	contrib := &ethpb.SignedContributionAndProof{
		Message: &ethpb.ContributionAndProof{
			AggregatorIndex: 1,
		},
	}

	s.processSyncCommitteeContribution(contrib)
	require.LogsContain(t, hook, "\"Sync committee aggregation processed\" ValidatorIndex=1")
	require.LogsDoNotContain(t, hook, "ValidatorIndex=2")
}

func TestProcessSyncAggregate(t *testing.T) {
	params.UseTestConfig()
	hook := logTest.NewGlobal()
	s := setupService(t)
	beaconState, _ := util.DeterministicGenesisStateAltair(t, 256)

	block := &ethpb.BeaconBlockAltair{
		Slot: 2,
		Body: &ethpb.BeaconBlockBodyAltair{
			SyncAggregate: &ethpb.SyncAggregate{
				SyncCommitteeBits: bitfield.Bitvector512{
					0x31, 0xff, 0xff, 0xff, 0xff, 0x3f, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				},
			},
		},
	}

	wrappedBlock, err := wrapper.WrappedAltairBeaconBlock(block)
	require.NoError(t, err)

	s.processSyncAggregate(beaconState, wrappedBlock)
	require.LogsContain(t, hook, "\"Sync committee contribution included\" BalanceChange=3168000000000 ContribCount=1 ExpectedContribCount=4 NewBalance=3200000000000 ValidatorIndex=1 prefix=monitor")
	require.LogsContain(t, hook, "\"Sync committee contribution included\" BalanceChange=3168100000000 ContribCount=2 ExpectedContribCount=2 NewBalance=3200000000000 ValidatorIndex=12 prefix=monitor")
	require.LogsDoNotContain(t, hook, "ValidatorIndex=2")
}

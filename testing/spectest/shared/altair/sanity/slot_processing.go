package sanity

import (
	"context"
	"strconv"
	"testing"

	"github.com/golang/snappy"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/transition"
	stateAltair "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v2"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/spectest/utils"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
	"google.golang.org/protobuf/proto"
	"gopkg.in/d4l3k/messagediff.v1"
)

func init() {
	transition.SkipSlotCache.Disable()
}

// RunSlotProcessingTests executes "sanity/slots" tests.
func RunSlotProcessingTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "altair", "sanity/slots/pyspec_tests")

	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "pre.ssz_snappy")
			require.NoError(t, err)
			preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			base := &ethpb.BeaconStateAltair{}
			require.NoError(t, base.UnmarshalSSZ(preBeaconStateSSZ), "Failed to unmarshal")
			beaconState, err := stateAltair.InitializeFromProto(base)
			require.NoError(t, err)

			file, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "slots.yaml")
			require.NoError(t, err)
			fileStr := string(file)
			slotsCount, err := strconv.Atoi(fileStr[:len(fileStr)-5])
			require.NoError(t, err)

			postBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "post.ssz_snappy")
			require.NoError(t, err)
			postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			postBeaconState := &ethpb.BeaconStateAltair{}
			require.NoError(t, postBeaconState.UnmarshalSSZ(postBeaconStateSSZ), "Failed to unmarshal")
			postState, err := transition.ProcessSlots(context.Background(), beaconState, beaconState.Slot().Add(uint64(slotsCount)))
			require.NoError(t, err)

			pbState, err := stateAltair.ProtobufBeaconState(postState.CloneInnerState())
			require.NoError(t, err)
			if !proto.Equal(pbState, postBeaconState) {
				diff, _ := messagediff.PrettyDiff(beaconState, postBeaconState)
				t.Fatalf("Post state does not match expected. Diff between states %s", diff)
			}
		})
	}
}

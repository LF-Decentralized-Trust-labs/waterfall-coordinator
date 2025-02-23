package operations

import (
	"context"
	"io/ioutil"
	"path"
	"strings"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/golang/snappy"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	stateAltair "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v2"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
	"google.golang.org/protobuf/proto"
	"gopkg.in/d4l3k/messagediff.v1"
)

type blockOperation func(context.Context, state.BeaconState, block.SignedBeaconBlock) (state.BeaconState, error)

// RunBlockOperationTest takes in the prestate and the beacon block body, processes it through the
// passed in block operation function and checks the post state with the expected post state.
func RunBlockOperationTest(
	t *testing.T,
	folderPath string,
	body *ethpb.BeaconBlockBodyAltair,
	operationFn blockOperation,
) {
	preBeaconStateFile, err := util.BazelFileBytes(path.Join(folderPath, "pre.ssz_snappy"))
	require.NoError(t, err)
	preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
	require.NoError(t, err, "Failed to decompress")
	preStateBase := &ethpb.BeaconStateAltair{}
	if err := preStateBase.UnmarshalSSZ(preBeaconStateSSZ); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	preState, err := stateAltair.InitializeFromProto(preStateBase)
	require.NoError(t, err)

	// If the post.ssz is not present, it means the test should fail on our end.
	postSSZFilepath, err := bazel.Runfile(path.Join(folderPath, "post.ssz_snappy"))
	postSSZExists := true
	if err != nil && strings.Contains(err.Error(), "could not locate file") {
		postSSZExists = false
	} else if err != nil {
		t.Fatal(err)
	}

	helpers.ClearCache()
	b := util.NewBeaconBlockAltair()
	b.Block.Body = body
	wsb, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	beaconState, err := operationFn(context.Background(), preState, wsb)
	if postSSZExists {
		require.NoError(t, err)

		postBeaconStateFile, err := ioutil.ReadFile(postSSZFilepath) // #nosec G304
		require.NoError(t, err)
		postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
		require.NoError(t, err, "Failed to decompress")

		postBeaconState := &ethpb.BeaconStateAltair{}
		if err := postBeaconState.UnmarshalSSZ(postBeaconStateSSZ); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}
		pbState, err := stateAltair.ProtobufBeaconState(beaconState.InnerStateUnsafe())
		require.NoError(t, err)
		if !proto.Equal(pbState, postBeaconState) {
			diff, _ := messagediff.PrettyDiff(beaconState.InnerStateUnsafe(), postBeaconState)
			t.Log(diff)
			t.Fatal("Post state does not match expected")
		}
	} else {
		// Note: This doesn't test anything worthwhile. It essentially tests
		// that *any* error has occurred, not any specific error.
		if err == nil {
			t.Fatal("Did not fail when expected")
		}
		t.Logf("Expected failure; failure reason = %v", err)
		return
	}
}

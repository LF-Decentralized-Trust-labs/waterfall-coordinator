package v3

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	testtmpl "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/testing"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

func TestBeaconState_JustificationBitsNil(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBitsNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_JustificationBits(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBits(
		t,
		func(bits bitfield.Bitvector4) (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{JustificationBits: bits})
		})
}

func TestBeaconState_PreviousJustifiedCheckpointNil(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_PreviousJustifiedCheckpoint(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpoint(
		t,
		func(cp *ethpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{PreviousJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_CurrentJustifiedCheckpointNil(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_CurrentJustifiedCheckpoint(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpoint(
		t,
		func(cp *ethpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{CurrentJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_FinalizedCheckpointNil(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_FinalizedCheckpoint(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpoint(
		t,
		func(cp *ethpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafe(&ethpb.BeaconStateBellatrix{FinalizedCheckpoint: cp})
		})
}

package blocks_test

import (
	"context"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/blocks"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	v "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/validators"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestSlashableAttestationData_CanSlash(t *testing.T) {
	att1 := util.HydrateAttestationData(&ethpb.AttestationData{
		Target: &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
		Source: &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{'A'}, 32)},
	})
	att2 := util.HydrateAttestationData(&ethpb.AttestationData{
		Target: &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
		Source: &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{'B'}, 32)},
	})
	assert.Equal(t, true, blocks.IsSlashableAttestationData(att1, att2), "Atts should have been slashable")
	att1.Target.Epoch = 4
	att1.Source.Epoch = 2
	att2.Source.Epoch = 3
	assert.Equal(t, true, blocks.IsSlashableAttestationData(att1, att2), "Atts should have been slashable")
}

func TestProcessAttesterSlashings_DataNotSlashable(t *testing.T) {
	slashings := []*ethpb.AttesterSlashing{{
		Attestation_1: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{}),
		Attestation_2: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
			Data: &ethpb.AttestationData{
				Source: &ethpb.Checkpoint{Epoch: 1},
				Target: &ethpb.Checkpoint{Epoch: 1}},
		})}}

	var registry []*ethpb.Validator
	currentSlot := types.Slot(0)

	beaconState, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: registry,
		Slot:       currentSlot,
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			AttesterSlashings: slashings,
		},
	}
	_, err = blocks.ProcessAttesterSlashings(context.Background(), beaconState, b.Block.Body.AttesterSlashings, v.SlashValidator)
	assert.ErrorContains(t, "attestations are not slashable", err)
}

func TestProcessAttesterSlashings_IndexedAttestationFailedToVerify(t *testing.T) {
	var registry []*ethpb.Validator
	currentSlot := types.Slot(0)

	beaconState, err := v1.InitializeFromProto(&ethpb.BeaconState{
		Validators: registry,
		Slot:       currentSlot,
	})
	require.NoError(t, err)

	slashings := []*ethpb.AttesterSlashing{
		{
			Attestation_1: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 1},
				},
				AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+1),
			}),
			Attestation_2: util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
				AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+1),
			}),
		},
	}

	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			AttesterSlashings: slashings,
		},
	}

	_, err = blocks.ProcessAttesterSlashings(context.Background(), beaconState, b.Block.Body.AttesterSlashings, v.SlashValidator)
	assert.ErrorContains(t, "validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE", err)
}

func TestProcessAttesterSlashings_AppliesCorrectStatus(t *testing.T) {
	params.UseTestConfig()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	for _, vv := range beaconState.Validators() {
		vv.WithdrawableEpoch = types.Epoch(params.BeaconConfig().SlotsPerEpoch)
	}

	att1 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 := privKeys[0].Sign(signingRoot[:])
	sig1 := privKeys[1].Sign(signingRoot[:])
	aggregateSig := bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att1.Signature = aggregateSig.Marshal()

	att2 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		AttestingIndices: []uint64{0, 1},
	})
	signingRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 = privKeys[0].Sign(signingRoot[:])
	sig1 = privKeys[1].Sign(signingRoot[:])
	aggregateSig = bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att2.Signature = aggregateSig.Marshal()

	slashings := []*ethpb.AttesterSlashing{
		{
			Attestation_1: att1,
			Attestation_2: att2,
		},
	}

	currentSlot := 2 * params.BeaconConfig().SlotsPerEpoch
	require.NoError(t, beaconState.SetSlot(currentSlot))

	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			AttesterSlashings: slashings,
		},
	}

	newState, err := blocks.ProcessAttesterSlashings(context.Background(), beaconState, b.Block.Body.AttesterSlashings, v.SlashValidator)
	require.NoError(t, err)
	newRegistry := newState.Validators()

	// Given the intersection of slashable indices is [1], only validator
	// at index 1 should be slashed and exited. We confirm this below.
	if newRegistry[1].ExitEpoch != beaconState.Validators()[1].ExitEpoch {
		t.Errorf(
			`
			Expected validator at index 1's exit epoch to match
			%d, received %d instead
			`,
			beaconState.Validators()[1].ExitEpoch,
			newRegistry[1].ExitEpoch,
		)
	}

	require.Equal(t, uint64(3175000000000), newState.Balances()[1])
	require.Equal(t, uint64(3200000000000), newState.Balances()[2])
}

func TestProcessAttesterSlashings_AppliesCorrectStatusAltair(t *testing.T) {
	params.UseTestConfig()
	beaconState, privKeys := util.DeterministicGenesisStateAltair(t, 100)
	for _, vv := range beaconState.Validators() {
		vv.WithdrawableEpoch = types.Epoch(params.BeaconConfig().SlotsPerEpoch)
	}

	att1 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 := privKeys[0].Sign(signingRoot[:])
	sig1 := privKeys[1].Sign(signingRoot[:])
	aggregateSig := bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att1.Signature = aggregateSig.Marshal()

	att2 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		AttestingIndices: []uint64{0, 1},
	})
	signingRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 = privKeys[0].Sign(signingRoot[:])
	sig1 = privKeys[1].Sign(signingRoot[:])
	aggregateSig = bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att2.Signature = aggregateSig.Marshal()

	slashings := []*ethpb.AttesterSlashing{
		{
			Attestation_1: att1,
			Attestation_2: att2,
		},
	}

	currentSlot := 2 * params.BeaconConfig().SlotsPerEpoch
	require.NoError(t, beaconState.SetSlot(currentSlot))

	b := util.NewBeaconBlock()
	b.Block = &ethpb.BeaconBlock{
		Body: &ethpb.BeaconBlockBody{
			AttesterSlashings: slashings,
		},
	}

	newState, err := blocks.ProcessAttesterSlashings(context.Background(), beaconState, b.Block.Body.AttesterSlashings, v.SlashValidator)
	require.NoError(t, err)
	newRegistry := newState.Validators()

	// Given the intersection of slashable indices is [1], only validator
	// at index 1 should be slashed and exited. We confirm this below.
	if newRegistry[1].ExitEpoch != beaconState.Validators()[1].ExitEpoch {
		t.Errorf(
			`
			Expected validator at index 1's exit epoch to match
			%d, received %d instead
			`,
			beaconState.Validators()[1].ExitEpoch,
			newRegistry[1].ExitEpoch,
		)
	}

	require.Equal(t, uint64(3150000000000), newState.Balances()[1])
	require.Equal(t, uint64(3200000000000), newState.Balances()[2])
}

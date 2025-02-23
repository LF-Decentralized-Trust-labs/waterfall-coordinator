package stateutil

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/ssz"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"go.opencensus.io/trace"
)

// ComputeFieldRootsWithHasherPhase0 hashes the provided phase 0 state and returns its respective field roots.
func ComputeFieldRootsWithHasherPhase0(ctx context.Context, state *ethpb.BeaconState) ([][]byte, error) {
	_, span := trace.StartSpan(ctx, "ComputeFieldRootsWithHasherPhase0")
	defer span.End()

	if state == nil {
		return nil, errors.New("nil state")
	}
	fieldRoots := make([][]byte, params.BeaconConfig().BeaconStateFieldCount)

	// Genesis time root.
	genesisRoot := ssz.Uint64Root(state.GenesisTime)
	fieldRoots[0] = genesisRoot[:]

	// Genesis validators root.
	r := [32]byte{}
	copy(r[:], state.GenesisValidatorsRoot)
	fieldRoots[1] = r[:]

	// Slot root.
	slotRoot := ssz.Uint64Root(uint64(state.Slot))
	fieldRoots[2] = slotRoot[:]

	// Fork data structure root.
	forkHashTreeRoot, err := ssz.ForkRoot(state.Fork)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute fork merkleization")
	}
	fieldRoots[3] = forkHashTreeRoot[:]

	// BeaconBlockHeader data structure root.
	headerHashTreeRoot, err := BlockHeaderRoot(state.LatestBlockHeader)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block header merkleization")
	}
	fieldRoots[4] = headerHashTreeRoot[:]

	// BlockRoots array root.
	blockRootsRoot, err := arraysRoot(state.BlockRoots, fieldparams.BlockRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block roots merkleization")
	}
	fieldRoots[5] = blockRootsRoot[:]

	// StateRoots array root.
	stateRootsRoot, err := arraysRoot(state.StateRoots, fieldparams.StateRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute state roots merkleization")
	}
	fieldRoots[6] = stateRootsRoot[:]

	// HistoricalRoots slice root.
	historicalRootsRt, err := ssz.ByteArrayRootWithLimit(state.HistoricalRoots, fieldparams.HistoricalRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute historical roots merkleization")
	}
	fieldRoots[7] = historicalRootsRt[:]

	// Eth1Data data structure root.
	eth1HashTreeRoot, err := Eth1Root(state.Eth1Data)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute eth1data merkleization")
	}
	fieldRoots[8] = eth1HashTreeRoot[:]

	// Eth1DataVotes slice root.
	eth1VotesRoot, err := eth1DataVotesRoot(state.Eth1DataVotes)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute eth1data votes merkleization")
	}
	fieldRoots[9] = eth1VotesRoot[:]

	// Eth1DepositIndex root.
	eth1DepositIndexBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(eth1DepositIndexBuf, state.Eth1DepositIndex)
	eth1DepositBuf := bytesutil.ToBytes32(eth1DepositIndexBuf)
	fieldRoots[10] = eth1DepositBuf[:]

	// Validators slice root.
	validatorsRoot, err := validatorRegistryRoot(state.Validators)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator registry merkleization")
	}
	fieldRoots[11] = validatorsRoot[:]

	// spineData root.
	spineDataRoot, err := SpineDataRoot(state.SpineData)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute spine data merkleization")
	}
	fieldRoots[12] = spineDataRoot[:]

	// Balances slice root.
	balancesRoot, err := Uint64ListRootWithRegistryLimit(state.Balances)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator balances merkleization")
	}
	fieldRoots[13] = balancesRoot[:]

	// RandaoMixes array root.
	randaoRootsRoot, err := arraysRoot(state.RandaoMixes, fieldparams.RandaoMixesLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute randao roots merkleization")
	}
	fieldRoots[14] = randaoRootsRoot[:]

	// Slashings array root.
	slashingsRootsRoot, err := ssz.SlashingsRoot(state.Slashings)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute slashings merkleization")
	}
	fieldRoots[15] = slashingsRootsRoot[:]

	// PreviousEpochAttestations slice root.
	prevAttsRoot, err := epochAttestationsRoot(state.PreviousEpochAttestations)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous epoch attestations merkleization")
	}
	fieldRoots[16] = prevAttsRoot[:]

	// CurrentEpochAttestations slice root.
	currAttsRoot, err := epochAttestationsRoot(state.CurrentEpochAttestations)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current epoch attestations merkleization")
	}
	fieldRoots[17] = currAttsRoot[:]

	// JustificationBits root.
	justifiedBitsRoot := bytesutil.ToBytes32(state.JustificationBits)
	fieldRoots[18] = justifiedBitsRoot[:]

	// PreviousJustifiedCheckpoint data structure root.
	prevCheckRoot, err := ssz.CheckpointRoot(state.PreviousJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous justified checkpoint merkleization")
	}
	fieldRoots[19] = prevCheckRoot[:]

	// CurrentJustifiedCheckpoint data structure root.
	currJustRoot, err := ssz.CheckpointRoot(state.CurrentJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current justified checkpoint merkleization")
	}
	fieldRoots[20] = currJustRoot[:]

	// FinalizedCheckpoint data structure root.
	finalRoot, err := ssz.CheckpointRoot(state.FinalizedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute finalized checkpoint merkleization")
	}
	fieldRoots[21] = finalRoot[:]

	// BlockVoting slice root.
	blockVotingHashTreeRoot, err := blockVotingRoot(state.BlockVoting)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute blockVoting merkleization")
	}
	fieldRoots[22] = blockVotingHashTreeRoot[:]

	return fieldRoots, nil
}

// ComputeFieldRootsWithHasherAltair hashes the provided altair state and returns its respective field roots.
func ComputeFieldRootsWithHasherAltair(ctx context.Context, state *ethpb.BeaconStateAltair) ([][]byte, error) {
	_, span := trace.StartSpan(ctx, "ComputeFieldRootsWithHasherAltair")
	defer span.End()

	if state == nil {
		return nil, errors.New("nil state")
	}
	fieldRoots := make([][]byte, params.BeaconConfig().BeaconStateAltairFieldCount)

	// Genesis time root.
	genesisRoot := ssz.Uint64Root(state.GenesisTime)
	fieldRoots[0] = genesisRoot[:]

	// Genesis validators root.
	r := [32]byte{}
	copy(r[:], state.GenesisValidatorsRoot)
	fieldRoots[1] = r[:]

	// Slot root.
	slotRoot := ssz.Uint64Root(uint64(state.Slot))
	fieldRoots[2] = slotRoot[:]

	// Fork data structure root.
	forkHashTreeRoot, err := ssz.ForkRoot(state.Fork)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute fork merkleization")
	}
	fieldRoots[3] = forkHashTreeRoot[:]

	// BeaconBlockHeader data structure root.
	headerHashTreeRoot, err := BlockHeaderRoot(state.LatestBlockHeader)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block header merkleization")
	}
	fieldRoots[4] = headerHashTreeRoot[:]

	// BlockRoots array root.
	blockRootsRoot, err := arraysRoot(state.BlockRoots, fieldparams.BlockRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block roots merkleization")
	}
	fieldRoots[5] = blockRootsRoot[:]

	// StateRoots array root.
	stateRootsRoot, err := arraysRoot(state.StateRoots, fieldparams.StateRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute state roots merkleization")
	}
	fieldRoots[6] = stateRootsRoot[:]

	// HistoricalRoots slice root.
	historicalRootsRt, err := ssz.ByteArrayRootWithLimit(state.HistoricalRoots, fieldparams.HistoricalRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute historical roots merkleization")
	}
	fieldRoots[7] = historicalRootsRt[:]

	// Eth1Data data structure root.
	eth1HashTreeRoot, err := Eth1Root(state.Eth1Data)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute eth1data merkleization")
	}
	fieldRoots[8] = eth1HashTreeRoot[:]

	// Eth1DataVotes slice root.
	eth1VotesRoot, err := eth1DataVotesRoot(state.Eth1DataVotes)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute eth1data votes merkleization")
	}
	fieldRoots[9] = eth1VotesRoot[:]

	// Eth1DepositIndex root.
	eth1DepositIndexBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(eth1DepositIndexBuf, state.Eth1DepositIndex)
	eth1DepositBuf := bytesutil.ToBytes32(eth1DepositIndexBuf)
	fieldRoots[10] = eth1DepositBuf[:]

	// Validators slice root.
	validatorsRoot, err := validatorRegistryRoot(state.Validators)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator registry merkleization")
	}
	fieldRoots[11] = validatorsRoot[:]

	// spineData root.
	spineDataRoot, err := SpineDataRoot(state.SpineData)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute spine data merkleization")
	}
	fieldRoots[12] = spineDataRoot[:]

	// Balances slice root.
	balancesRoot, err := Uint64ListRootWithRegistryLimit(state.Balances)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator balances merkleization")
	}
	fieldRoots[13] = balancesRoot[:]

	// RandaoMixes array root.
	randaoRootsRoot, err := arraysRoot(state.RandaoMixes, fieldparams.RandaoMixesLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute randao roots merkleization")
	}
	fieldRoots[14] = randaoRootsRoot[:]

	// Slashings array root.
	slashingsRootsRoot, err := ssz.SlashingsRoot(state.Slashings)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute slashings merkleization")
	}
	fieldRoots[15] = slashingsRootsRoot[:]

	// PreviousEpochParticipation slice root.
	prevParticipationRoot, err := ParticipationBitsRoot(state.PreviousEpochParticipation)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous epoch participation merkleization")
	}
	fieldRoots[16] = prevParticipationRoot[:]

	// CurrentEpochParticipation slice root.
	currParticipationRoot, err := ParticipationBitsRoot(state.CurrentEpochParticipation)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current epoch participation merkleization")
	}
	fieldRoots[17] = currParticipationRoot[:]

	// JustificationBits root.
	justifiedBitsRoot := bytesutil.ToBytes32(state.JustificationBits)
	fieldRoots[18] = justifiedBitsRoot[:]

	// PreviousJustifiedCheckpoint data structure root.
	prevCheckRoot, err := ssz.CheckpointRoot(state.PreviousJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous justified checkpoint merkleization")
	}
	fieldRoots[19] = prevCheckRoot[:]

	// CurrentJustifiedCheckpoint data structure root.
	currJustRoot, err := ssz.CheckpointRoot(state.CurrentJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current justified checkpoint merkleization")
	}
	fieldRoots[20] = currJustRoot[:]

	// FinalizedCheckpoint data structure root.
	finalRoot, err := ssz.CheckpointRoot(state.FinalizedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute finalized checkpoint merkleization")
	}
	fieldRoots[21] = finalRoot[:]

	// BlockVoting slice root.
	blockVotingHashTreeRoot, err := blockVotingRoot(state.BlockVoting)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute blockVoting merkleization")
	}
	fieldRoots[22] = blockVotingHashTreeRoot[:]

	// Inactivity scores root.
	inactivityScoresRoot, err := Uint64ListRootWithRegistryLimit(state.InactivityScores)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute inactivityScoreRoot")
	}
	fieldRoots[23] = inactivityScoresRoot[:]

	// Current sync committee root.
	currentSyncCommitteeRoot, err := SyncCommitteeRoot(state.CurrentSyncCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[24] = currentSyncCommitteeRoot[:]

	// Next sync committee root.
	nextSyncCommitteeRoot, err := SyncCommitteeRoot(state.NextSyncCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[25] = nextSyncCommitteeRoot[:]

	return fieldRoots, nil
}

// ComputeFieldRootsWithHasherBellatrix hashes the provided bellatrix state and returns its respective field roots.
func ComputeFieldRootsWithHasherBellatrix(ctx context.Context, state *ethpb.BeaconStateBellatrix) ([][]byte, error) {
	_, span := trace.StartSpan(ctx, "ComputeFieldRootsWithHasherBellatrix")
	defer span.End()

	if state == nil {
		return nil, errors.New("nil state")
	}
	fieldRoots := make([][]byte, params.BeaconConfig().BeaconStateBellatrixFieldCount)

	// Genesis time root.
	genesisRoot := ssz.Uint64Root(state.GenesisTime)
	fieldRoots[0] = genesisRoot[:]

	// Genesis validators root.
	r := [32]byte{}
	copy(r[:], state.GenesisValidatorsRoot)
	fieldRoots[1] = r[:]

	// Slot root.
	slotRoot := ssz.Uint64Root(uint64(state.Slot))
	fieldRoots[2] = slotRoot[:]

	// Fork data structure root.
	forkHashTreeRoot, err := ssz.ForkRoot(state.Fork)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute fork merkleization")
	}
	fieldRoots[3] = forkHashTreeRoot[:]

	// BeaconBlockHeader data structure root.
	headerHashTreeRoot, err := BlockHeaderRoot(state.LatestBlockHeader)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block header merkleization")
	}
	fieldRoots[4] = headerHashTreeRoot[:]

	// BlockRoots array root.
	blockRootsRoot, err := arraysRoot(state.BlockRoots, fieldparams.BlockRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block roots merkleization")
	}
	fieldRoots[5] = blockRootsRoot[:]

	// StateRoots array root.
	stateRootsRoot, err := arraysRoot(state.StateRoots, fieldparams.StateRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute state roots merkleization")
	}
	fieldRoots[6] = stateRootsRoot[:]

	// HistoricalRoots slice root.
	historicalRootsRt, err := ssz.ByteArrayRootWithLimit(state.HistoricalRoots, fieldparams.HistoricalRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute historical roots merkleization")
	}
	fieldRoots[7] = historicalRootsRt[:]

	// Eth1Data data structure root.
	eth1HashTreeRoot, err := Eth1Root(state.Eth1Data)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute eth1data merkleization")
	}
	fieldRoots[8] = eth1HashTreeRoot[:]

	// Eth1DataVotes slice root.
	eth1VotesRoot, err := eth1DataVotesRoot(state.Eth1DataVotes)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute eth1data votes merkleization")
	}
	fieldRoots[9] = eth1VotesRoot[:]

	// Eth1DepositIndex root.
	eth1DepositIndexBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(eth1DepositIndexBuf, state.Eth1DepositIndex)
	eth1DepositBuf := bytesutil.ToBytes32(eth1DepositIndexBuf)
	fieldRoots[10] = eth1DepositBuf[:]

	// Validators slice root.
	validatorsRoot, err := validatorRegistryRoot(state.Validators)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator registry merkleization")
	}
	fieldRoots[11] = validatorsRoot[:]

	// spineData root.
	spineDataRoot, err := SpineDataRoot(state.SpineData)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute spine data merkleization")
	}
	fieldRoots[12] = spineDataRoot[:]

	// Balances slice root.
	balancesRoot, err := Uint64ListRootWithRegistryLimit(state.Balances)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator balances merkleization")
	}
	fieldRoots[13] = balancesRoot[:]

	// RandaoMixes array root.
	randaoRootsRoot, err := arraysRoot(state.RandaoMixes, fieldparams.RandaoMixesLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute randao roots merkleization")
	}
	fieldRoots[14] = randaoRootsRoot[:]

	// Slashings array root.
	slashingsRootsRoot, err := ssz.SlashingsRoot(state.Slashings)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute slashings merkleization")
	}
	fieldRoots[15] = slashingsRootsRoot[:]

	// PreviousEpochParticipation slice root.
	prevParticipationRoot, err := ParticipationBitsRoot(state.PreviousEpochParticipation)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous epoch participation merkleization")
	}
	fieldRoots[16] = prevParticipationRoot[:]

	// CurrentEpochParticipation slice root.
	currParticipationRoot, err := ParticipationBitsRoot(state.CurrentEpochParticipation)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current epoch participation merkleization")
	}
	fieldRoots[17] = currParticipationRoot[:]

	// JustificationBits root.
	justifiedBitsRoot := bytesutil.ToBytes32(state.JustificationBits)
	fieldRoots[18] = justifiedBitsRoot[:]

	// PreviousJustifiedCheckpoint data structure root.
	prevCheckRoot, err := ssz.CheckpointRoot(state.PreviousJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous justified checkpoint merkleization")
	}
	fieldRoots[19] = prevCheckRoot[:]

	// CurrentJustifiedCheckpoint data structure root.
	currJustRoot, err := ssz.CheckpointRoot(state.CurrentJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current justified checkpoint merkleization")
	}
	fieldRoots[20] = currJustRoot[:]

	// FinalizedCheckpoint data structure root.
	finalRoot, err := ssz.CheckpointRoot(state.FinalizedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute finalized checkpoint merkleization")
	}
	fieldRoots[21] = finalRoot[:]

	// BlockVoting slice root.
	blockVotingHashTreeRoot, err := blockVotingRoot(state.BlockVoting)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute blockVoting merkleization")
	}
	fieldRoots[22] = blockVotingHashTreeRoot[:]

	// Inactivity scores root.
	inactivityScoresRoot, err := Uint64ListRootWithRegistryLimit(state.InactivityScores)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute inactivityScoreRoot")
	}
	fieldRoots[23] = inactivityScoresRoot[:]

	// Current sync committee root.
	currentSyncCommitteeRoot, err := SyncCommitteeRoot(state.CurrentSyncCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[24] = currentSyncCommitteeRoot[:]

	// Next sync committee root.
	nextSyncCommitteeRoot, err := SyncCommitteeRoot(state.NextSyncCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[25] = nextSyncCommitteeRoot[:]

	// Execution payload root.
	//executionPayloadRoot, err := state.LatestExecutionPayloadHeader.HashTreeRoot()
	//if err != nil {
	//	return nil, err
	//}
	//fieldRoots[26] = executionPayloadRoot[:]
	fieldRoots[26] = bytesutil.PadTo([]byte{0}, 32)

	return fieldRoots, nil
}

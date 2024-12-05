//Copyright 2024   Blue Wave Inc.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package stateutil

import (
	"bytes"
	"encoding/binary"

	"github.com/pkg/errors"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/ssz"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

// BlockVotingDataRootWithHasher returns the hash tree root of input `BlockVoting`.
func BlockVotingDataRootWithHasher(blockVoting *ethpb.BlockVoting) ([32]byte, error) {
	if blockVoting == nil {
		return [32]byte{}, errors.New("nil blockVoting data")
	}
	var (
		rootRoot, slotRoot, candRoot, votesRoot [32]byte
		err                                     error
	)
	if len(blockVoting.Root) > 0 {
		rootRoot = bytesutil.ToBytes32(blockVoting.Root)
	}
	binary.LittleEndian.PutUint64(slotRoot[:8], uint64(blockVoting.Slot))
	candRoot, err = BytesRoot(blockVoting.Candidates)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "block voting candidates")
	}
	votesRoot, err = CommitteeVotesListRoot(blockVoting.Votes)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "block voting votes")
	}

	fieldRoots := [][32]byte{rootRoot, slotRoot, candRoot, votesRoot}
	root, err := ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	if err != nil {
		return [32]byte{}, err
	}
	return root, nil
}

// BlockVotingsRoot returns the hash tree root of input `blockVoting`.
func BlockVotingsRoot(blockVotings []*ethpb.BlockVoting) ([32]byte, error) {
	BlockVotingRoots := make([][32]byte, len(blockVotings))
	for i := 0; i < len(blockVotings); i++ {
		root, err := BlockVotingDataRootWithHasher(blockVotings[i])
		if err != nil {
			return [32]byte{}, errors.Wrap(err, "could not compute blockVoting merkleization")
		}
		BlockVotingRoots[i] = root
	}

	blockVotingRootsRoot, err := ssz.BitwiseMerkleize(
		BlockVotingRoots,
		uint64(len(BlockVotingRoots)),
		fieldparams.BlockVotingLength,
	)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute blockVoting votes merkleization")
	}
	blockVotingRootBuf := new(bytes.Buffer)
	if err := binary.Write(blockVotingRootBuf, binary.LittleEndian, uint64(len(blockVotings))); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not marshal blockVoting votes length")
	}
	// We need to mix in the length of the slice.
	blockVotingRootBufRoot := make([]byte, 32)
	copy(blockVotingRootBufRoot, blockVotingRootBuf.Bytes())
	root := ssz.MixInLength(blockVotingRootsRoot, blockVotingRootBufRoot)

	return root, nil
}

// CommitteeVotesListRoot returns the hash tree root of input `CommitteeVote`.
func CommitteeVotesListRoot(committeeVotes []*ethpb.CommitteeVote) ([32]byte, error) {
	bvsRoots := make([][32]byte, len(committeeVotes))
	for i := 0; i < len(committeeVotes); i++ {
		root, err := CommitteeVoteRoot(committeeVotes[i])
		if err != nil {
			return [32]byte{}, err
		}
		bvsRoots[i] = root
	}

	blockVotingRootsRoot, err := ssz.BitwiseMerkleize(bvsRoots, uint64(len(bvsRoots)), uint64(len(bvsRoots)))
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute committee votes merkleization")
	}
	dataLenRoot := make([]byte, 32)
	binary.LittleEndian.PutUint64(dataLenRoot, uint64(len(bvsRoots)))
	root := ssz.MixInLength(blockVotingRootsRoot, dataLenRoot)
	return root, nil
}

// CommitteeVoteRoot returns the hash tree root of input `CommitteeVote`.
func CommitteeVoteRoot(committeeVote *ethpb.CommitteeVote) ([32]byte, error) {
	if committeeVote == nil {
		return [32]byte{}, nil
	}
	var slotBuf, indexBuf [32]byte
	binary.LittleEndian.PutUint64(slotBuf[:8], uint64(committeeVote.Slot))
	binary.LittleEndian.PutUint64(indexBuf[:8], uint64(committeeVote.Index))
	agrBuf, err := BytesRoot(committeeVote.AggregationBits.Bytes())
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute committee vote aggr bits")
	}
	votingRoots := [][32]byte{slotBuf, indexBuf, agrBuf}

	root, err := ssz.BitwiseMerkleize(votingRoots, uint64(len(votingRoots)), uint64(len(votingRoots)))
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute committee vote merkleization")
	}
	return root, nil
}

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

package validator

import (
	"bytes"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

// processPrevoteData method processes prevote data to define chain which has most votes
func (vs *Server) processPrevoteData(prevoteData []*ethpb.PreVote, optCandidates gwatCommon.HashArray) gwatCommon.HashArray {
	// Divide prevote candidates to subchains of spines
	chains, votes := vs.getChainsAndVotes(prevoteData, optCandidates)

	return vs.defineMostVotedChain(chains, votes)
}

// getChainsAndVotes receives an array of prevote structs, defines unique subchains of spines and calculates total
// amount of votes for these subchains and return data in corresponding maps: map[spinesSubchainHash]spinesSubchain and
// map [spinesSubchainHash]amount of votes
func (vs *Server) getChainsAndVotes(prevote []*ethpb.PreVote, optCandidates gwatCommon.HashArray) (map[[gwatCommon.HashLength]byte]gwatCommon.HashArray,
	map[[gwatCommon.HashLength]byte]uint64) {
	hashAndChain := make(map[[gwatCommon.HashLength]byte]gwatCommon.HashArray)
	hashAndVotes := make(map[[gwatCommon.HashLength]byte]uint64)
	opc := optCandidates.ToBytes()

	for _, pv := range prevote {
		can := gwatCommon.HashArrayFromBytes(pv.Data.Candidates)
		for i := 1; i <= len(can); i++ {
			chain := can[:i]
			// Check if optimistic candidates contain subchain got from prevote data
			if !bytes.Contains(opc, chain.ToBytes()) {
				break
			}
			if chain.IsUniq() {
				h := chain.Key()
				hashAndChain[h] = chain
				if helpers.IsAggregatedPrevote(pv) {
					hashAndVotes[h] += pv.GetAggregationBits().Count()
				} else {
					hashAndVotes[h]++
				}
			} else {
				log.Warnf("Prevote spine subchain contains duplicates of hashes for prevote: %v", can)
			}
		}
	}

	return hashAndChain, hashAndVotes
}

// defineMostVotedChain defines and returns longest subchain with most of the votes
func (vs *Server) defineMostVotedChain(chainsMap map[[gwatCommon.HashLength]byte]gwatCommon.HashArray,
	votesMap map[[gwatCommon.HashLength]byte]uint64) gwatCommon.HashArray {

	// Define most votes number
	var mostVotes uint64
	for _, v := range votesMap {
		if v > mostVotes {
			mostVotes = v
		}
	}

	// Define longest subchain which have most of the votes
	var chain gwatCommon.HashArray
	for k, v := range votesMap {
		if v >= mostVotes && len(chainsMap[k]) > len(chain) {
			chain = chainsMap[k]
		}
	}

	log.WithFields(logrus.Fields{
		"1.votesAmount": mostVotes,
		"2.chain":       chain,
	}).Info("defineMostVotedChain: longest prevote candidates chain with the most votes")
	return chain
}

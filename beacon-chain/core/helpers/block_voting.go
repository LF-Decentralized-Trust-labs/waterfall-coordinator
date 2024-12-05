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

package helpers

import (
	"bytes"
	"fmt"

	"github.com/prysmaticlabs/go-bitfield"
	log "github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/hash"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

// CountCommitteeVotes counts votes of BlockVoting
func CountCommitteeVotes(committeeVotes []*ethpb.CommitteeVote) uint64 {
	count := uint64(0)
	for _, att := range committeeVotes {
		count += att.GetAggregationBits().Count()
	}
	return count
}

// CommitteeVoteKey create key for CommitteeVote instance.
func CommitteeVoteKey(committeeVote *ethpb.CommitteeVote) [32]byte {
	if committeeVote == nil {
		return [32]byte{}
	}
	bf := bytesutil.Bytes8(uint64(committeeVote.AggregationBits.Len()))
	slot := bytesutil.Bytes8(uint64(committeeVote.Slot))
	index := bytesutil.Bytes8(uint64(committeeVote.Index))
	rawKey := append(bf, slot...)
	rawKey = append(rawKey, index...)
	key := hash.Hash(rawKey)
	return key
}

// AggregateCommitteeVote aggregates votes of committee for blockVoting.
// Deprecated
func AggregateCommitteeVote(committeeVotes []*ethpb.CommitteeVote) []*ethpb.CommitteeVote {
	var err error
	// mapping by keys
	votesMap := map[[32]byte][]*ethpb.CommitteeVote{}
	for _, v := range committeeVotes {
		k := CommitteeVoteKey(v)
		if votesMap[k] == nil {
			votesMap[k] = []*ethpb.CommitteeVote{}
		}
		votesMap[k] = append(votesMap[k], v)
	}
	//aggregate
	res := make([]*ethpb.CommitteeVote, len(votesMap))
	i := 0
	for _, avs := range votesMap {
		baseData := avs[0]
		aggrBits := bitfield.NewBitlist(baseData.AggregationBits.Len())
		if len(avs) > 1 {
			for _, v := range avs {
				aggrBits, err = aggrBits.Or(v.AggregationBits)
				if err != nil {
					log.WithError(err).Error("CommitteeVotes aggregation failed: should never happened")
					panic("CommitteeVotes aggregation failed: should never happened")
				}
			}
		} else {
			aggrBits = bytesutil.SafeCopyBytes(baseData.AggregationBits)
		}
		res[i] = &ethpb.CommitteeVote{
			AggregationBits: aggrBits,
			Slot:            baseData.Slot,
			Index:           baseData.Index,
		}
		i++
	}
	return res
}

// AddAggregateCommitteeVote aggregates votes of committee for blockVoting.
// Optimized version.
func AddAggregateCommitteeVote(committeeVotes []*ethpb.CommitteeVote, addVote *ethpb.CommitteeVote) []*ethpb.CommitteeVote {
	var isAggregated bool
	for _, v := range committeeVotes {
		//find target committee
		if v.Slot == addVote.Slot && v.Index == addVote.Index {
			//aggregate
			bits, err := v.AggregationBits.Or(addVote.AggregationBits)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"committee":       v.Index,
					"slot":            v.Slot,
					"curCommitteeLen": v.AggregationBits.Len(),
					"addCommitteeLen": addVote.AggregationBits.Len(),
				}).Error("Block voting: aggregate committee votes error")
				continue
			}
			v.AggregationBits = bits
			isAggregated = true
			break
		}
	}
	if !isAggregated {
		committeeVotes = append(committeeVotes, addVote)
	}
	return committeeVotes
}

func BlockVotingCopy(vote *ethpb.BlockVoting) *ethpb.BlockVoting {
	cpyVotes := make([]*ethpb.CommitteeVote, len(vote.Votes))
	for i, att := range vote.Votes {
		cpyVotes[i] = &ethpb.CommitteeVote{
			AggregationBits: bytesutil.SafeCopyBytes(att.AggregationBits),
			Slot:            vote.Slot,
			Index:           att.Index,
		}
	}
	return &ethpb.BlockVoting{
		Root:       bytesutil.SafeCopyBytes(vote.Root),
		Slot:       vote.Slot,
		Candidates: bytesutil.SafeCopyBytes(vote.Candidates),
		Votes:      cpyVotes,
	}
}

func BlockVotingArrCopy(votes []*ethpb.BlockVoting) []*ethpb.BlockVoting {
	cpy := make([]*ethpb.BlockVoting, len(votes))
	for i, vote := range votes {
		cpy[i] = BlockVotingCopy(vote)
	}
	return cpy
}

// BlockVotingArrStateOrder put BlockVoting array to order to calculate state hash.
func BlockVotingArrStateOrder(votes []*ethpb.BlockVoting) ([]*ethpb.BlockVoting, error) {
	var err error
	cpyAttOrd := make([]*ethpb.BlockVoting, len(votes))
	for i, itm := range votes {
		cpyItm := BlockVotingCopy(itm)
		cpyItm.Votes, err = CommitteeVoteArrSort(cpyItm.Votes)
		if err != nil {
			return nil, err
		}
		cpyAttOrd[i] = cpyItm
	}
	return BlockVotingArrSort(cpyAttOrd)
}

// BlockVotingArrSort sorts BlockVoting array.
func BlockVotingArrSort(votes []*ethpb.BlockVoting) ([]*ethpb.BlockVoting, error) {
	keys := gwatCommon.HashArray{}
	mapKeyData := map[[32]byte]*ethpb.BlockVoting{}
	for _, itm := range votes {
		k, err := itm.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		mapKeyData[k] = itm
	}
	for k := range mapKeyData {
		keys = append(keys, k)
	}
	keys = keys.Sort()
	sorted := make([]*ethpb.BlockVoting, len(keys))
	for i, k := range keys {
		sorted[i] = mapKeyData[k]
	}
	return sorted, nil
}

// CommitteeVoteArrSort sorts attestations array.
func CommitteeVoteArrSort(votes []*ethpb.CommitteeVote) ([]*ethpb.CommitteeVote, error) {
	keys := gwatCommon.HashArray{}
	mapKeyData := map[[32]byte]*ethpb.CommitteeVote{}
	for _, itm := range votes {
		k, err := itm.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		mapKeyData[k] = itm
	}
	for k := range mapKeyData {
		keys = append(keys, k)
	}
	keys = keys.Sort()
	sorted := make([]*ethpb.CommitteeVote, len(keys))
	for i, k := range keys {
		sorted[i] = mapKeyData[k]
	}
	return sorted, nil
}

func IndexOfRoot(arrRoots [][]byte, root []byte) int {
	for i, r := range arrRoots {
		if bytes.Equal(r, root) {
			return i
		}
	}
	return -1
}

// PrintBlockVotingArr returns formatted string of BlockVoting array.
func PrintBlockVotingArr(blockVotings []*ethpb.BlockVoting) string {
	str := "["
	for i, bv := range blockVotings {
		str += PrintBlockVoting(bv)
		if i < len(blockVotings)-1 {
			str += ","
		}
	}
	str += "]"
	return str
}

// PrintBlockVoting returns formatted string of BlockVoting.
func PrintBlockVoting(blockVoting *ethpb.BlockVoting) string {
	candStr := "["
	cands := gwatCommon.HashArrayFromBytes(blockVoting.GetCandidates())
	for i, c := range cands {
		candStr += fmt.Sprintf("\"%#x\"", c)
		if i < len(cands)-1 {
			candStr += ","
		}
	}
	candStr += "]"

	str := "{"
	str += fmt.Sprintf("root: \"%#x\",", blockVoting.Root)
	str += fmt.Sprintf("candidates: %s,", candStr)
	str += fmt.Sprintf("slot: %d,", blockVoting.GetSlot())
	str += "Votes: ["
	for i, att := range blockVoting.Votes {
		str += "{"
		str += fmt.Sprintf("aggrBits: %b,", att.GetAggregationBits())
		str += fmt.Sprintf("aggrBitLen: %d,", att.GetAggregationBits().Len())
		str += fmt.Sprintf("aggrBitCount: %d,", att.GetAggregationBits().Count())
		str += fmt.Sprintf("slot: %d,", att.GetSlot())
		str += fmt.Sprintf("committeeIndex: %d", att.GetIndex())
		str += "}"
		if i < len(blockVoting.Votes)-1 {
			str += ","
		}
	}
	str += "]}"
	return str
}

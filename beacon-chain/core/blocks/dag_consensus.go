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

package blocks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	lru "github.com/hashicorp/golang-lru"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

type mapVoting map[gwatCommon.Hash]int
type mapPriority map[int]gwatCommon.HashArray
type mapCandidates map[gwatCommon.Hash]gwatCommon.HashArray

// ProcessDagConsensus is an operation performed on each beacon block
// to calculate state transition data related to dag consensus:
// 1. calculate new prefix of spines
// 2. collect attestations and calculate consensus of finalization.
func ProcessDagConsensus(ctx context.Context, beaconState state.BeaconState, signed block.SignedBeaconBlock) (state.BeaconState, error) {
	if beaconState == nil || beaconState.IsNil() {
		return nil, errors.New("nil state")
	}

	var (
		beaconBlock  = signed.Block()
		attestations = beaconBlock.Body().Attestations()
		candidates   = beaconBlock.Body().Eth1Data().GetCandidates()
		blockVoting  = helpers.BlockVotingArrCopy(beaconState.BlockVoting())
	)

	//calculate prefix
	prefix, unpubChains, err := CalcPrefixAndParentSpines(beaconState.SpineData(), candidates)
	if err != nil {
		return nil, err
	}

	//add item of block voting for the current block
	if params.BeaconConfig().IsPrefixFinForkSlot(beaconBlock.Slot()) {
		if len(prefix) > 0 {
			blockVoting = addBlockVoting(blockVoting, beaconBlock.ParentRoot(), beaconBlock.Slot()-1, prefix.ToBytes())
		}
	} else {
		if len(candidates) > 0 {
			blockVoting = addBlockVoting(blockVoting, beaconBlock.ParentRoot(), beaconBlock.Slot()-1, prefix.ToBytes())
		}
	}
	//append attestations of the current block to block voting
	isBlockVotingForkSlot := params.BeaconConfig().IsBlockVotingForkSlot(beaconState.Slot())
	for _, att := range attestations {
		blockVoting = appendBlockVotingAtt(blockVoting, att, isBlockVotingForkSlot)
	}

	//calculation of finalization sequence
	finalization, err := calcFinalization(ctx, beaconState, blockVoting)
	if err != nil {
		return nil, err
	}

	// cutoff the finalization
	prefix = prefix.Difference(finalization)

	// cutoff the finalization & prefix and cast to []*SpinesSeq
	parentSpines := make([]*ethpb.SpinesSeq, 0, len(unpubChains))
	for _, chain := range unpubChains {
		dif := chain.Difference(finalization).Difference(prefix)
		if len(dif) > 0 {
			parentSpines = append(parentSpines, &ethpb.SpinesSeq{Spines: dif.ToBytes()})
		}
	}

	// update Eth1Data of state
	eth1Data := beaconState.Eth1Data()
	eth1Data.Candidates = candidates
	if err = beaconState.SetEth1Data(eth1Data); err != nil {
		return nil, err
	}

	spineData := &ethpb.SpineData{
		Spines:       candidates,
		Prefix:       prefix.ToBytes(),
		Finalization: finalization.ToBytes(),
		CpFinalized:  beaconState.SpineData().CpFinalized,
		ParentSpines: parentSpines,
	}
	if err = beaconState.SetSpineData(spineData); err != nil {
		return nil, err
	}

	// removes BlockVoting with completely finalized candidates
	deprecatedRoots := getBlockVotingsDeprecatedRoots(blockVoting, finalization)
	blockVoting = removeBlockVoting(blockVoting, deprecatedRoots)

	if blockVoting, err = handleBlockVotingVotesLimit(ctx, blockVoting, beaconState); err != nil {
		return nil, err
	}

	// if it's a new epoch - removes stale BlockVoting.
	if slots.IsEpochStart(beaconBlock.Slot()) {

		if blockVoting, err = cleanBlockVotingStaleVotes(ctx, blockVoting, beaconState); err != nil {
			return nil, err
		}

		cpSlot, err := slots.EpochStart(beaconState.FinalizedCheckpointEpoch())
		if err != nil {
			return nil, err
		}
		staleRoots := getBlockVotingRootsLtSlot(blockVoting, cpSlot)

		blockVoting = removeBlockVoting(blockVoting, staleRoots)

		log.WithFields(logrus.Fields{
			"slot":             beaconState.Slot(),
			"BlockVoting":      len(blockVoting),
			"StateBlockVoting": len(beaconState.BlockVoting()),
			"staleRoots":       fmt.Sprintf("%#x", staleRoots),
		}).Info("Block Voting processing: removes stale at new epoch")

	}

	blockVoting, err = helpers.BlockVotingArrStateOrder(blockVoting)
	if err != nil {
		return nil, err
	}

	if err := beaconState.SetBlockVoting(blockVoting); err != nil {
		return nil, err
	}
	return beaconState, nil
}

// calcFinalization calculates finalization sequence by BlockVotings.
func calcFinalization(ctx context.Context, beaconState state.BeaconState, blockVotingArr []*ethpb.BlockVoting) (gwatCommon.HashArray, error) {
	var (
		blockVotings    = helpers.BlockVotingArrCopy(blockVotingArr)
		supportedVotes  = make([]*ethpb.BlockVoting, 0)
		candidatesParts = make([][]gwatCommon.HashArray, 0)
		tabPriority     = mapPriority{}
		tabVoting       = mapVoting{}
		tabCandidates   = mapCandidates{}
		slotsToConfirm  = params.BeaconConfig().VotingRequiredSlots
		badVotes        = make([]*ethpb.BlockVoting, 0)
		finalization    = gwatCommon.HashArrayFromBytes(beaconState.SpineData().Finalization)
		lastFinSpine    = helpers.GetTerminalFinalizedSpine(beaconState)
		resFinalization = finalization.Copy()
	)

	for _, bv := range blockVotings {
		// candidates must be uniq
		candidates := gwatCommon.HashArrayFromBytes(bv.Candidates)
		if !candidates.IsUniq() {
			log.WithFields(logrus.Fields{
				"slot":         beaconState.Slot(),
				"root":         fmt.Sprintf("%#x", bv.GetRoot()),
				"candidates":   candidates,
				"lastFinSpine": lastFinSpine.Hex(),
			}).Warn("skip: bad candidates (not uniq)")
			badVotes = append(badVotes, bv)
			continue
		}

		// handle data for each attestations' slot
		mapSlotAtt := map[types.Slot][]*ethpb.CommitteeVote{}
		for _, att := range bv.GetVotes() {
			mapSlotAtt[att.GetSlot()] = append(mapSlotAtt[att.GetSlot()], att)
		}
		for slot, atts := range mapSlotAtt {
			minSupport, err := BlockVotingMinSupport(ctx, beaconState, slot)
			if err != nil {
				return nil, err
			}
			// if provided enough support for slot adds data as separated item
			if helpers.CountCommitteeVotes(atts) >= uint64(minSupport) {
				supportedVotes = append(supportedVotes, helpers.BlockVotingCopy(bv))
				break
			}
		}
	}

	log.WithFields(logrus.Fields{
		"slot":                  beaconState.Slot(),
		"total-BlockVoting":     len(blockVotings),
		"supported-BlockVoting": len(supportedVotes),
		"VotingRequiredSlots":   params.BeaconConfig().VotingRequiredSlots,
		"SecondsPerSlot":        params.BeaconConfig().SecondsPerSlot,
	}).Info("Voting info")

	// handle supported votes
	for _, bv := range supportedVotes {
		candidates := gwatCommon.HashArrayFromBytes(bv.Candidates)
		//exclude finalized spines
		fullLen := len(candidates)
		if i := candidates.IndexOf(lastFinSpine); i >= 0 {
			candidates = candidates[i+1:]
		}
		// if all current candidates handled
		if len(candidates) == 0 && fullLen > len(candidates) {
			log.WithFields(logrus.Fields{
				"slot":         beaconState.Slot(),
				"root":         fmt.Sprintf("%#x", bv.GetRoot()),
				"candidates":   candidates,
				"lastFinSpine": lastFinSpine.Hex(),
			}).Warn("skip: no candidates")
			badVotes = append(badVotes, bv)
			continue
		}

		reductedParts := []gwatCommon.HashArray{candidates}
		// reduction of sequence up to single item
		for i := len(candidates) - 1; i > 0; i-- {
			reduction := candidates[:i]
			reductedParts = append(reductedParts, reduction)
		}
		candidatesParts = append(candidatesParts, reductedParts)
	}

	//calculate voting params
	for i, candidatesList := range candidatesParts {
		var restList []gwatCommon.HashArray
		restParts := candidatesParts[i+1:]
		for _, rp := range restParts {
			restList = append(restList, rp...)
		}
		for _, candidates := range candidatesList {
			for _, rc := range restList {
				if candidates.IsEqualTo(rc) {
					intersect := candidates
					key := intersect.Key()
					tabCandidates[key] = intersect
					tabPriority[len(intersect)] = append(tabPriority[len(intersect)], key).Uniq()
					tabVoting[key]++
				}
			}
		}
	}

	//sort by priority
	priorities := make([]int, 0)
	for p := range tabPriority {
		priorities = append(priorities, p)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

	// find voting result
	resKey := gwatCommon.Hash{}
	for _, p := range priorities {
		// select by max number of vote which satisfies the condition
		// of min required number of votes
		maxVotes := 0
		for _, key := range tabPriority[p] {
			votes := tabVoting[key]
			if votes >= slotsToConfirm && votes > maxVotes {
				resKey = key
			}
		}
		if resKey != (gwatCommon.Hash{}) {
			break
		}
	}

	log.WithFields(logrus.Fields{
		"slot":         beaconState.Slot(),
		"lastFinSpine": lastFinSpine.Hex(),
		"finalization": tabCandidates[resKey],
	}).Info("Calculation of finalization sequence")

	if resKey != (gwatCommon.Hash{}) {
		resFinalization = append(resFinalization, tabCandidates[resKey]...)
	}

	return resFinalization, nil
}

var minSupportCache *lru.Cache

// BlockVotingMinSupport calc minimal required number of votes for BlockVoting consensus.
func BlockVotingMinSupport(ctx context.Context, state state.BeaconState, slot types.Slot) (int, error) {
	var err error
	if minSupportCache == nil {
		minSupportCache, err = lru.New(int(params.BeaconConfig().SlotsPerEpoch) * 10)
		if err != nil {
			minSupportCache = nil
		}
	}
	if minSupportCache.Contains(slot) {
		cval, ok1 := minSupportCache.Get(slot)
		if ok1 {
			ms, ok2 := cval.(int)
			if ok2 {
				log.WithFields(logrus.Fields{
					"slot":        slot,
					"minSupport":  ms,
					"cachedSlots": minSupportCache.Len(),
				}).Info("minSupportCache used")
				return ms, nil
			}
		}
	}

	// 50% + 1
	minSupport := params.BeaconConfig().BlockVotingMinSupportPrc
	committees, err := helpers.CalcSlotCommitteesIndexes(ctx, state, slot)
	if err != nil {
		return 0, err
	}
	slotAtts := 0
	for _, cmt := range committees {
		slotAtts += len(cmt)
	}
	val := int(math.Ceil((float64(slotAtts)/100)*float64(minSupport))) + 1
	if val > slotAtts {
		val = slotAtts
	}
	if minSupportCache != nil {
		minSupportCache.Add(slot, val)
	}
	return val, nil
}

func CalcPrefixAndParentSpines(stSpineData *ethpb.SpineData, blCandidates []byte) (prefix gwatCommon.HashArray, parentSpines []gwatCommon.HashArray, err error) {
	var (
		parentPrefix                      = gwatCommon.HashArrayFromBytes(stSpineData.Prefix)
		prefixExtension, resPrefix        gwatCommon.HashArray
		parentUnpubChains, resUnpubChains []gwatCommon.HashArray
	)
	spines := gwatCommon.HashArrayFromBytes(blCandidates)
	finalized := gwatCommon.HashArrayFromBytes(stSpineData.Finalization)

	//calc parent unpublished spines
	parentUnpubChains = make([]gwatCommon.HashArray, len(stSpineData.ParentSpines))
	for i, spseq := range stSpineData.ParentSpines {
		chain := gwatCommon.HashArrayFromBytes(spseq.Spines)
		if len(chain) > 0 {
			parentUnpubChains[i] = chain
		}
	}

	// calculate unpublished spines chains
	resUnpubChains = parentUnpubChains
	if len(spines) > 0 {
		// set new spines to the first position
		if len(spines) > 0 {
			resUnpubChains = []gwatCommon.HashArray{spines}
		}
		for _, chain := range parentUnpubChains {
			if len(chain) == 0 {
				continue
			}
			chainDif := chain.Difference(parentPrefix)
			// the first spine of dif-chain must be equal to the first spine
			// otherwise - skip
			if len(chainDif) > 0 && chainDif[0] == spines[0] {
				resUnpubChains = append(resUnpubChains, chain)
			}
		}
	}

	// calculate the new prefix
	prefixExtension, err = helpers.ConsensusCalcPrefix(resUnpubChains)
	if err != nil {
		return nil, nil, err
	}
	resPrefix = append(parentPrefix, prefixExtension...).Uniq()
	resPrefix = resPrefix.Difference(finalized)

	return resPrefix, resUnpubChains, nil
}

// getBlockVotingsDeprecatedRoots returns deprecated roots of BlockVoting.
func getBlockVotingsDeprecatedRoots(blockVoting []*ethpb.BlockVoting, finalization gwatCommon.HashArray) [][]byte {
	mapRoots := map[gwatCommon.Hash][]byte{}
	for _, bv := range blockVoting {
		candidates := gwatCommon.HashArrayFromBytes(bv.GetCandidates())
		if len(candidates) > 0 {
			lastCandidat := candidates[len(candidates)-1]
			if finalization.IndexOf(lastCandidat) > -1 {
				mapRoots[gwatCommon.BytesToHash(bv.GetRoot())] = bv.GetRoot()
			}
		}
	}
	roots := make([][]byte, len(mapRoots))
	i := 0
	for _, rt := range mapRoots {
		roots[i] = rt
		i++
	}
	return roots
}

func getBlockVotingRootsLtSlot(blockVoting []*ethpb.BlockVoting, slot types.Slot) [][]byte {
	mapRoots := map[gwatCommon.Hash][]byte{}
	for _, bv := range blockVoting {
		if bv.Slot <= slot {
			mapRoots[gwatCommon.BytesToHash(bv.GetRoot())] = bv.GetRoot()
		}
	}
	roots := make([][]byte, len(mapRoots))
	i := 0
	for _, rt := range mapRoots {
		roots[i] = rt
		i++
	}
	return roots
}

// cleanBlockVotingStaleVotes removes unsupported BlockVoting.Votes and empty BlockVoting older than 2 epochs.
func cleanBlockVotingStaleVotes(ctx context.Context, blockVoting []*ethpb.BlockVoting, bState state.BeaconState) ([]*ethpb.BlockVoting, error) {
	// define slot of deprecation
	stSlot := bState.Slot()
	targetEpoch := slots.ToEpoch(bState.Slot())
	if targetEpoch <= 2 {
		return blockVoting, nil
	}
	staleVotesSlot, err := slots.EpochStart(targetEpoch - 2)
	if err != nil {
		return []*ethpb.BlockVoting{}, err
	}
	for i, bv := range blockVoting {
		// check blockVoting slot is older of stale slot
		if bv.Slot < staleVotesSlot {
			resLen := len(bv.Votes)
			slotMap := map[types.Slot][]*ethpb.CommitteeVote{}
			slotsArr := make([]types.Slot, 0, len(bv.Votes))
			for _, vote := range bv.Votes {
				slot := vote.Slot
				if params.BeaconConfig().IsBlockVotingForkSlot(stSlot) {
					if slotMap[slot] == nil {
						slotMap[slot] = []*ethpb.CommitteeVote{}
						slotsArr = append(slotsArr, slot)
					}
				} else {
					slotsArr = append(slotsArr, slot)
					if slotMap[slot] == nil {
						slotMap[slot] = []*ethpb.CommitteeVote{}
					}
				}
				slotMap[slot] = append(slotMap[slot], vote)
			}
			// asc sort slotsArr
			sort.Slice(slotsArr, func(i, j int) bool {
				return slotsArr[i] < slotsArr[j]
			})
			// find min unsupported slot
			for _, slot := range slotsArr {
				if slot >= staleVotesSlot {
					continue
				}
				votes := slotMap[slot]
				// find unsupported with min slot
				//unsupported BlockVoting and older 2 epochs
				minSupport, err := BlockVotingMinSupport(ctx, bState, slot)
				if err != nil {
					return []*ethpb.BlockVoting{}, err
				}
				// if no enough support rm slot votes
				if helpers.CountCommitteeVotes(votes) < uint64(minSupport) {

					log.WithFields(logrus.Fields{
						"staleVotesSlot": staleVotesSlot,
						"rmSlot":         slot,
						"blVoting":       helpers.PrintBlockVoting(bv),
					}).Info("BlockVoting votes clean stale votes")

					resLen -= len(slotMap[slot])
					delete(slotMap, slot)
				}
			}
			resVotes := make([]*ethpb.CommitteeVote, 0, resLen)
			for _, slot := range slotsArr {
				//for _, vote := range slotMap[slot] {
				//	resVotes = helpers.AddAggregateCommitteeVote(resVotes, vote)
				//}
				if len(slotMap[slot]) > 0 {
					resVotes = append(resVotes, slotMap[slot]...)
				}
			}
			blockVoting[i].Votes = resVotes
		}
	}
	// rm stale empty BlockVoting
	res := make([]*ethpb.BlockVoting, 0, len(blockVoting))
	for _, bv := range blockVoting {
		// check blockVoting slot is older of stale slot
		if bv.Slot >= staleVotesSlot || len(bv.Votes) > 0 {
			res = append(res, bv)
		} else {
			log.WithFields(logrus.Fields{
				"staleVotesSlot": staleVotesSlot,
				"blVoting":       helpers.PrintBlockVoting(bv),
			}).Info("BlockVoting votes clean empty item")
		}
	}
	return res, nil
}

// handleBlockVotingVotesLimit checks BlockVoting.Votes len limitation and reduces len in needed.
func handleBlockVotingVotesLimit(ctx context.Context, blockVoting []*ethpb.BlockVoting, bState state.BeaconState) ([]*ethpb.BlockVoting, error) {
	var (
		minSupport    int
		err           error
		minSupportMap = make(map[types.Slot]int)
		stSlot        = bState.Slot()
	)
	for i, bv := range blockVoting {
		// check Votes len limit
		if len(bv.Votes) > 64 {
			// reduce len
			resLen := len(bv.Votes)
			slotMap := map[types.Slot][]*ethpb.CommitteeVote{}
			slotsArr := make([]types.Slot, 0, len(bv.Votes))
			for _, vote := range bv.Votes {
				slot := vote.Slot
				if params.BeaconConfig().IsBlockVotingForkSlot(stSlot) {
					if slotMap[slot] == nil {
						slotMap[slot] = []*ethpb.CommitteeVote{}
						slotsArr = append(slotsArr, slot)
					}
				} else {
					slotsArr = append(slotsArr, slot)
					if slotMap[slot] == nil {
						slotMap[slot] = []*ethpb.CommitteeVote{}
					}
				}
				slotMap[slot] = append(slotMap[slot], vote)
			}
			// asc sort slotsArr
			sort.Slice(slotsArr, func(i, j int) bool {
				return slotsArr[i] < slotsArr[j]
			})
			// find min unsupported slot
			for _, slot := range slotsArr {
				votes := slotMap[slot]
				// find unsupported with min slot
				//unsupported BlockVoting and older 2 epochs
				if v, ok := minSupportMap[slot]; ok {
					minSupport = v
				} else {
					minSupport, err = BlockVotingMinSupport(ctx, bState, slot)
					if err != nil {
						return []*ethpb.BlockVoting{}, err
					}
					minSupportMap[slot] = minSupport
				}
				// if no enough support rm slot votes
				if helpers.CountCommitteeVotes(votes) < uint64(minSupport) {
					resLen -= len(slotMap[slot])
					delete(slotMap, slot)
					break
				}
			}
			resVotes := make([]*ethpb.CommitteeVote, 0, resLen)
			for _, slot := range slotsArr {
				//for _, vote := range slotMap[slot] {
				//	resVotes = helpers.AddAggregateCommitteeVote(resVotes, vote)
				//}
				if len(slotMap[slot]) > 0 {
					resVotes = append(resVotes, slotMap[slot]...)
				}
			}
			blockVoting[i].Votes = resVotes

			log.WithFields(logrus.Fields{
				"blVoting": helpers.PrintBlockVoting(bv),
			}).Info("BlockVoting votes reduced")
		}
	}
	return blockVoting, nil
}

func isBlockVotingExists(votes []*ethpb.BlockVoting, root []byte) bool {
	for _, itm := range votes {
		if bytes.Equal(itm.Root, root) {
			return true
		}
	}
	return false
}

func addBlockVoting(votes []*ethpb.BlockVoting, root []byte, slot types.Slot, candidates []byte) []*ethpb.BlockVoting {
	cpy := helpers.BlockVotingArrCopy(votes)
	if !isBlockVotingExists(cpy, root) {
		newItem := &ethpb.BlockVoting{
			Root:       root,
			Votes:      []*ethpb.CommitteeVote{},
			Slot:       slot,
			Candidates: candidates,
		}
		cpy = append(cpy, newItem)
		return cpy
	}
	for _, itm := range cpy {
		if bytes.Equal(itm.Root, root) {
			itm.Slot = slot
			itm.Candidates = candidates
		}
	}
	return votes
}

func appendBlockVotingAtt(votes []*ethpb.BlockVoting, val *ethpb.Attestation, isBlockVotingForkSlot bool) []*ethpb.BlockVoting {
	root := val.GetData().BeaconBlockRoot
	if !isBlockVotingExists(votes, root) {
		return votes
	}
	newVote := &ethpb.CommitteeVote{
		AggregationBits: bytesutil.SafeCopyBytes(val.AggregationBits),
		Slot:            val.Data.Slot,
		Index:           val.Data.CommitteeIndex,
	}

	cpy := helpers.BlockVotingArrCopy(votes)
	for _, itm := range cpy {
		if bytes.Equal(itm.Root, root) {
			if isBlockVotingForkSlot {
				itm.Votes = helpers.AddAggregateCommitteeVote(itm.GetVotes(), newVote)
			} else {
				itm.Votes = helpers.AggregateCommitteeVote(append(itm.GetVotes(), newVote))
			}
			break
		}
	}
	return cpy
}

func removeBlockVoting(votes []*ethpb.BlockVoting, roots [][]byte) []*ethpb.BlockVoting {
	if len(roots) == 0 {
		return votes
	}
	upVotes := make([]*ethpb.BlockVoting, 0, len(votes))
	for _, itm := range votes {
		if helpers.IndexOfRoot(roots, itm.Root) == -1 {
			upVotes = append(upVotes, itm)
		}
	}
	return upVotes
}

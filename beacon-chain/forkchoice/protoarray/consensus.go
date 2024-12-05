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

package protoarray

import (
	"context"
	"fmt"
	"sort"
	"time"

	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"go.opencensus.io/trace"
)

func (f *ForkChoice) setBalances(root [32]byte, balances []uint64) {
	f.store.balancesLock.Lock()
	defer f.store.balancesLock.Unlock()
	f.store.balances[root] = balances
}
func (f *ForkChoice) getBalances(root [32]byte) []uint64 {
	f.store.balancesLock.RLock()
	defer f.store.balancesLock.RUnlock()
	return f.store.balances[root]
}

func (f *ForkChoice) setNodeVotes(validator uint64, vote Vote, node *int) (maxNode *int) {
	var (
		maxSlot types.Slot
		index   int
	)
	if node == nil {
		for i, n := range f.store.nodes {
			if n.slot > maxSlot {
				maxSlot = n.slot
				index = i
			}
		}
	} else {
		index = *node
	}
	lastNode := f.store.nodes[index]
	lastNode.attsData.votes[validator] = vote
	return &index
}

func (f *ForkChoice) getNodeVotes(nodeRoot [32]byte) map[uint64]Vote {
	n := f.GetNode(nodeRoot)
	if n == nil {
		return nil
	}
	return n.AttestationsData().Votes()
}

// GetParentByOptimisticSpines retrieves node by root.
func (fc *ForkChoice) GetParentByOptimisticSpines(ctx context.Context, optSpines []gwatCommon.HashArray, jCpRoot [32]byte) ([32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "protoArrayForkChoice.GetParentByOptimisticSpines")
	defer span.End()

	var headRoot [32]byte
	var err error

	defer func(start time.Time) {
		log.WithField(
			"elapsed", time.Since(start),
		).WithFields(logrus.Fields{
			"optSpines": len(optSpines),
			"headRoot":  fmt.Sprintf("%#x", headRoot),
			"jCpRoot":   fmt.Sprintf("%#x", jCpRoot),
		}).Info("FC: GetParentByOptimisticSpines end")
	}(time.Now())

	//removes empty values
	_optSpines := make([]gwatCommon.HashArray, 0, len(optSpines))
	for _, ha := range optSpines {
		if len(ha) > 0 {
			_optSpines = append(_optSpines, ha)
		}
	}

	fcCpy := fc.Copy()

	// collect nodes of T(G) tree
	acceptableRootIndexMap, _ := collectTgTreeNodesByOptimisticSpines(fcCpy, _optSpines, jCpRoot)

	log.WithFields(logrus.Fields{
		"acceptableRootIndexMap": fmt.Sprintf("%d", len(acceptableRootIndexMap)),
	}).Info("FC: TG Tree")

	if len(acceptableRootIndexMap) == 0 {
		return [32]byte{}, nil
	}

	// check cached fc
	fcBase, diffRootIndexMap, diffNodes := getCompatibleFc(acceptableRootIndexMap, fcCpy)

	fcBase.balances = fcCpy.getBalances(jCpRoot)

	log.WithFields(logrus.Fields{
		"balances":               len(fcCpy.getBalances(jCpRoot)),
		"items":                  fmt.Sprintf("%d", cacheForkChoice.cache.Len()),
		"inactivity":             fmt.Sprintf("%v", cacheForkChoice.inactivity),
		"inact_len":              fmt.Sprintf("%d", len(cacheForkChoice.inactivity)),
		"acceptableRootIndexMap": fmt.Sprintf("%d", len(acceptableRootIndexMap)),
		"diff":                   fmt.Sprintf("%d", len(diffRootIndexMap)),
	}).Info("FC: cache")

	headRoot, err = calculateHeadRootByNodesIndexes(ctx, fcBase, diffNodes, acceptableRootIndexMap, jCpRoot)
	if err != nil {
		return [32]byte{}, err
	}

	updateCache(fcBase, len(diffRootIndexMap))

	return headRoot, nil
}

// calculateHeadRootOfForks retrieves root of head of passed forks.
func calculateHeadRootByNodesIndexes(
	ctx context.Context,
	fcBase *ForkChoice,
	diffNodes map[uint64]*Node,
	rootIndexMap map[[32]byte]uint64,
	justifiedRoot [32]byte,
) ([32]byte, error) {

	indexRootMap := make(map[uint64][32]byte, len(rootIndexMap))
	for r, index := range rootIndexMap {
		indexRootMap[index] = r
	}

	// sort node's indexes
	nodeIndexes := make(gwatCommon.SorterAscU64, 0, len(diffNodes))
	for index := range diffNodes {
		nodeIndexes = append(nodeIndexes, index)
	}
	sort.Sort(nodeIndexes)

	// fill ForkChoice instance
	var headRoot [32]byte
	for _, index := range nodeIndexes {
		n := diffNodes[index]
		n.bestChild = NonExistentNode
		n.bestDescendant = NonExistentNode
		n.weight = 0

		if n.parent != NonExistentNode {
			//if node.parent != NonExistentNode {
			parentRoot, ok := indexRootMap[n.parent]
			//parentRoot, ok := indexRootMap[node.parent]
			if !ok {
				return [32]byte{}, errParentNodFound
			}
			n.parent = fcBase.store.nodesIndices[parentRoot]
		}

		log.WithFields(logrus.Fields{"index": index}).Info("Calculate head root by nodes indexes: i 000")

		err := fcBase.store.insertNode(ctx, n)

		log.WithError(err).WithFields(logrus.Fields{"index": index}).Info("Calculate head root by nodes indexes: i 111")

		if err != nil {
			return [32]byte{}, err
		}

		// sort validators' indexes
		validatorIndexes := make(gwatCommon.SorterAscU64, 0, len(n.AttestationsData().Votes()))
		for ix := range n.AttestationsData().Votes() {
			validatorIndexes = append(validatorIndexes, ix)
		}
		sort.Sort(validatorIndexes)

		log.WithError(err).WithFields(logrus.Fields{
			"index":            index,
			"validatorIndexes": len(validatorIndexes),
		}).Info("Calculate head root by nodes indexes: i 333")

		for _, vi := range validatorIndexes {
			vote := n.AttestationsData().votes[vi]
			targetEpoch := vote.nextEpoch
			blockRoot := vote.nextRoot
			// Validator indices will grow the vote cache.
			for vi >= uint64(len(fcBase.votes)) {
				fcBase.votes = append(fcBase.votes, Vote{currentRoot: params.BeaconConfig().ZeroHash, nextRoot: params.BeaconConfig().ZeroHash})
			}

			// Newly allocated vote if the root fields are untouched.
			newVote := fcBase.votes[vi].nextRoot == params.BeaconConfig().ZeroHash &&
				fcBase.votes[vi].currentRoot == params.BeaconConfig().ZeroHash

			// Vote gets updated if it's newly allocated or high target epoch.
			if newVote || targetEpoch > fcBase.votes[vi].nextEpoch {
				fcBase.votes[vi].nextEpoch = targetEpoch
				fcBase.votes[vi].nextRoot = blockRoot
			}
		}
	}
	topNode := fcBase.store.nodes[len(fcBase.store.nodes)-1]

	// apply LMD GHOST
	headRoot, err := fcBase.Head(ctx, topNode.justifiedEpoch, justifiedRoot, fcBase.balances, topNode.finalizedEpoch)

	if err != nil {
		return [32]byte{}, err
	}

	log.WithFields(logrus.Fields{
		"headRoot": fmt.Sprintf("%#x", headRoot),
		"balances": len(fcBase.balances),
	}).Info("Get parent by optimistic spines res")

	return headRoot, nil
}

func collectTgTreeNodesByOptimisticSpines(fc *ForkChoice, optSpines []gwatCommon.HashArray, jCpRoot [32]byte) (map[[32]byte]uint64, map[[32]byte]int) {
	forks := fc.GetForks()
	rootIndexMap := make(map[[32]byte]uint64)
	leafs := make(map[[32]byte]int)
	nodesIndices := fc.store.cpyNodesIndices()

	for frkNr, frk := range forks {
		if frk == nil {
			continue
		}
		//logs data
		frkSlots := make([]types.Slot, len(frk.roots))
		//exclude not justified forks
		isJustified := false
		for i, r := range frk.roots {
			frkSlots[i] = frk.nodesMap[r].slot
			if r == jCpRoot {
				isJustified = true
				//break
			}
		}
		if !isJustified {
			log.WithFields(logrus.Fields{
				"frkNr":    frkNr,
				"jCpRoot":  fmt.Sprintf("%#x", jCpRoot),
				"frkRoots": fmt.Sprintf("%#x", frk.roots),
				"frkSlots": frkSlots,
			}).Warn("collectTgTreeNodesByOptimisticSpines: skip not justified fork")
			continue
		}
		for i, r := range frk.roots {
			node := frk.nodesMap[r]

			if len(node.spinesData.cpFinalized) == 0 {
				log.WithFields(logrus.Fields{
					"frkNr":             frkNr,
					"node.index":        i,
					"node.slot":         node.slot,
					"node.root":         fmt.Sprintf("%#x", node.root),
					"node.cpFinalized":  node.spinesData.cpFinalized,
					"node.Finalization": node.spinesData.Finalization(),
					"optSpines":         optSpines,
					"frkSlots":          frkSlots,
				}).Error("collectTgTreeNodesByOptimisticSpines: checkpoint finalized seq empty")
			}

			// rm finalized spines from optSpines if contains
			lastFinHash := node.spinesData.cpFinalized[len(node.spinesData.cpFinalized)-1]
			lastFinIndex := indexOfOptimisticSpines(lastFinHash, optSpines)
			forkOptSpines := optSpines
			if lastFinIndex > -1 {
				forkOptSpines = optSpines[lastFinIndex+1:]
			}

			// check finalization matches to optSpines
			finalization := node.spinesData.Finalization()
			ok := isSequenceMatchOptimisticSpines(finalization, forkOptSpines)

			log.WithFields(logrus.Fields{
				"ok":           ok,
				"frkNr":        frkNr,
				"node.index":   i,
				"node.slot":    node.slot,
				"node.root":    fmt.Sprintf("%#x", node.root),
				"jCpRoot":      fmt.Sprintf("%#x", jCpRoot),
				"finalization": len(finalization),
				"frkOptSpines": len(forkOptSpines),
				"frkSlots":     frkSlots,
			}).Info("collectTgTreeNodesByOptimisticSpines: check finalization")

			if !ok {
				continue
			}

			// check prefix matches to optSpines
			prefOptSpines := []gwatCommon.HashArray{}
			if len(forkOptSpines) > len(finalization) {
				prefOptSpines = forkOptSpines[len(finalization):]
			}
			prefix := node.spinesData.Prefix()
			ok = isSequenceMatchOptimisticSpines(prefix, prefOptSpines)

			log.WithFields(logrus.Fields{
				"ok":         ok,
				"frkNr":      frkNr,
				"node.index": i,
				"node.slot":  node.slot,
				"node.root":  fmt.Sprintf("%#x", node.root),
				"frkSlots":   frkSlots,
			}).Info("collectTgTreeNodesByOptimisticSpines: check prefix")

			if !ok {
				continue
			}

			//check prefix extension or no published spines
			published := node.spinesData.Spines()
			isExtended := len(prefix.Intersection(published)) > 0 || len(finalization.Intersection(published)) > 0
			if isExtended || len(published) == 0 {
				//collect roots of acceptable forks
				forkRoots := frk.roots[i:]
				for _, root := range forkRoots {
					rootIndexMap[root] = nodesIndices[root]
				}

				log.WithFields(logrus.Fields{
					"ok":         ok,
					"frkNr":      frkNr,
					"node.index": i,
					"node.slot":  node.slot,
					"node.root":  fmt.Sprintf("%#x", node.root),
					"frkSlots":   frkSlots,
				}).Info("collectTgTreeNodesByOptimisticSpines: check prefix extension: success")

				leafs[frk.roots[i]] = len(forkRoots)
				break
			}

			// check the first published spine matches to prefOptSpines
			pubOptSpines := []gwatCommon.HashArray{}
			if len(prefOptSpines) > len(prefix) {
				pubOptSpines = prefOptSpines[len(prefix):]
			}

			log.WithFields(logrus.Fields{
				"continue":   len(pubOptSpines) == 0,
				"frkNr":      frkNr,
				"node.index": i,
				"node.slot":  node.slot,
				"node.root":  fmt.Sprintf("%#x", node.root),
				"frkSlots":   frkSlots,
			}).Info("collectTgTreeNodesByOptimisticSpines: check published spines length")

			if len(pubOptSpines) == 0 {
				continue
			}

			log.WithFields(logrus.Fields{
				"continue":     !pubOptSpines[0].Has(published[0]),
				"published":    published,
				"pubOptSpines": pubOptSpines,
				"frkNr":        frkNr,
				"node.index":   i,
				"node.slot":    node.slot,
				"node.root":    fmt.Sprintf("%#x", node.root),
				"frkSlots":     frkSlots,
				//"forkOptSpines[0]": forkOptSpines[0],
			}).Info("collectTgTreeNodesByOptimisticSpines: check the first published spine")

			if !pubOptSpines[0].Has(published[0]) {
				continue
			}
			//collect roots of acceptable forks
			forkRoots := frk.roots[i:]
			for _, root := range forkRoots {
				rootIndexMap[root] = nodesIndices[root]
			}

			log.WithFields(logrus.Fields{
				"frkNr":      frkNr,
				"node.index": i,
				"node.slot":  node.slot,
				"node.root":  fmt.Sprintf("%#x", node.root),
				"frkSlots":   frkSlots,
			}).Info("collectTgTreeNodesByOptimisticSpines: check the first published spine: success")

			leafs[frk.roots[i]] = len(forkRoots)
			break
		}
	}
	return rootIndexMap, leafs
}

func isSequenceMatchOptimisticSpines(seq gwatCommon.HashArray, optSpines []gwatCommon.HashArray) bool {
	if len(seq) > len(optSpines) {
		return false
	}
	for i, h := range seq {
		if !optSpines[i].Has(h) {
			return false
		}
	}
	return true
}

func indexOfOptimisticSpines(hash gwatCommon.Hash, optSpines []gwatCommon.HashArray) int {
	for i, sines := range optSpines {
		if sines.Has(hash) {
			return i
		}
	}
	return -1
}

// CollectForkExcludedAttestations collect attestations
func (f *ForkChoice) CollectForkExcludedBlkRoots(leaf gwatCommon.Hash) gwatCommon.HashArray {
	f.mu.Lock()
	defer f.mu.Unlock()
	// collect nodes excluded from fork
	fork := f.GetFork(leaf)
	exIndices := make(map[[32]byte]uint64, len(f.store.nodesIndices))
	for k, v := range f.store.cpyNodesIndices() {
		exIndices[k] = v
	}
	for _, r := range fork.roots {
		delete(exIndices, r)
	}
	//collect attestation
	exRoots := make(gwatCommon.HashArray, 0, len(exIndices))
	for r := range exIndices {
		exRoots = append(exRoots, r)
	}
	return exRoots
}

// insertNode inserts node to the fork choice store's node list.
// It then updates the new node's parent with best child and descendant node.
func (s *Store) insertNode(ctx context.Context, node *Node) error {
	_, span := trace.StartSpan(ctx, "protoArrayForkChoice.insertNode")
	defer span.End()

	s.nodesLock.Lock()
	defer s.nodesLock.Unlock()

	// Return if the block has been inserted into Store before.
	if _, ok := s.nodesIndices[node.root]; ok {
		return nil
	}
	n := copyNode(node)
	index := uint64(len(s.nodes))
	s.nodesIndices[n.root] = index
	s.nodes = append(s.nodes, n)

	// Update parent with the best child and descendant only if it's available.
	if n.parent != NonExistentNode {
		if err := s.updateBestChildAndDescendant(n.parent, index); err != nil {
			return err
		}
	}
	return nil
}

// GetRoots get roots of nodes of forkchoice sorted by indexes.
func (f *ForkChoice) GetRoots() gwatCommon.HashArray {
	nodeIndexes := make(gwatCommon.SorterAscU64, 0, len(f.store.cpyNodesIndices()))
	for _, index := range f.store.cpyNodesIndices() {
		nodeIndexes = append(nodeIndexes, index)
	}
	sort.Sort(nodeIndexes)
	res := make(gwatCommon.HashArray, len(nodeIndexes))
	for i, ix := range nodeIndexes {
		res[i] = gwatCommon.BytesToHash(f.store.nodes[ix].root[:])
	}
	return res
}

// GetNode retrieves node by root.
func (f *ForkChoice) GetNode(root [32]byte) *Node {
	if !f.HasNode(root) {
		return nil
	}
	i := f.store.nodesIndices[root]
	return f.store.nodes[i]
}

// GetForks collects forks.
func (f *ForkChoice) GetForks() []*Fork {
	handledRoots := map[[32]byte]*Node{}
	res := []*Fork{}
	for i := f.NodeCount() - 1; i >= 0; i-- {
		node := f.store.nodes[i]
		if _, ok := handledRoots[node.root]; ok {
			continue
		}
		fork := f.GetFork(node.Root())
		if fork == nil {
			continue
		}
		for r, n := range fork.nodesMap {
			handledRoots[r] = n
		}
		res = append(res, fork)
	}
	return res
}

// GetFork collect nodes of tip by recursively iterate by parents.
func (f *ForkChoice) GetFork(root [32]byte) *Fork {
	head := f.GetNode(root)
	if head == nil {
		return nil
	}
	fork := Fork{
		roots:    append(make([][32]byte, 0, f.NodeCount()), root),
		nodesMap: map[[32]byte]*Node{root: head},
	}

	currIndex := head.parent
	if currIndex == NonExistentNode {
		return &fork
	}

	for {
		if currIndex >= uint64(len(f.store.nodes)) {
			log.WithFields(logrus.Fields{
				"currIndex":          currIndex,
				"len(f.store.nodes)": len(f.store.nodes),
			}).Error("FC: get fork oun of range")
			break
		}
		node := f.store.nodes[currIndex]
		if node == nil {
			break
		}
		fork.roots = append(fork.roots, node.Root())
		fork.nodesMap[node.Root()] = node
		if !f.HasParent(node.root) {
			break
		}
		currIndex = node.Parent()
	}
	return &fork
}

// GetCommonAncestor searches the highest common ancestor.
func (f *ForkChoice) GetCommonAncestor() (node *Node) {
	forks := f.GetForks()
	if len(forks) == 0 {
		return nil
	}
	if len(forks) == 1 {
		if len(forks[0].roots) > 0 {
			root := forks[0].roots[0]
			node = forks[0].nodesMap[root]
		}
		return node
	}

	var commonChain gwatCommon.HashArray
	for _, fork := range forks {
		tipRoots := fork.roots
		tipChain := make(gwatCommon.HashArray, len(tipRoots))
		for i, r := range tipRoots {
			tipChain[i] = gwatCommon.BytesToHash(r[:])
		}
		if commonChain == nil {
			commonChain = tipChain.Reverse()
		} else {
			commonChain = commonChain.SequenceIntersection(tipChain.Reverse())
		}
	}
	if len(commonChain) == 0 {
		return nil
	}
	commonRoot := commonChain[len(commonChain)-1]
	return f.GetNode(commonRoot)
}

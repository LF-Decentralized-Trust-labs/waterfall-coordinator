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
	"bytes"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/sirupsen/logrus"
	lruwrpr "gitlab.waterfall.network/waterfall/protocol/coordinator/cache/lru"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/hash"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

const (
	// maxForkChoiceCacheSize defines the max number of items can cache.
	maxForkChoiceCacheSize = 16
	// maxInactivityScore max inactivity score to keep items in cache.
	maxInactivityScore = 128
	// diffLenToCache defines diff lentgth of rootIndexMap to add item to cache.
	diffLenToCache = 8
)

// cacheForkChoice main cache instance
var cacheForkChoice *ForkChoiceCache = NewForkChoiceCache()

// ForkChoiceCache is a struct with 1 LRU cache for looking up forkchoice.
type ForkChoiceCache struct {
	cache      *lru.Cache
	lock       sync.RWMutex
	inactivity map[[32]byte]int
	keyCache   *lru.Cache
}

// NewForkChoiceCache creates a new cache of forkchoice.
func NewForkChoiceCache() *ForkChoiceCache {
	return &ForkChoiceCache{
		cache:      lruwrpr.New(maxForkChoiceCacheSize),
		inactivity: make(map[[32]byte]int),
		keyCache:   lruwrpr.New(maxForkChoiceCacheSize),
	}
}

// Add adds a new ForkChoice entry into the cache.
func (c *ForkChoiceCache) Add(fc *ForkChoice) {
	if fc == nil || fc.store == nil || len(fc.store.nodesIndices) == 0 {
		return
	}

	cpy := fc.Copy()
	key, keyData := cacheKeyByRootIndexMap(cpy.store.nodesIndices)
	_ = c.cache.Add(key, cpy)
	_ = c.keyCache.Add(key, keyData)
	return
}

// Get returns the forkchoice in cache.
func (c *ForkChoiceCache) Get(rootIndexMap map[[32]byte]uint64) *ForkChoice {
	if len(rootIndexMap) == 0 {
		return nil
	}
	key, _ := cacheKeyByRootIndexMap(rootIndexMap)
	value, exists := c.cache.Get(key)
	if !exists {
		return nil
	}
	return value.(*ForkChoice).Copy()
}

func cacheKeyByRootIndexMap(rootIndexMap map[[32]byte]uint64) ([32]byte, []byte) {
	if len(rootIndexMap) == 0 {
		return [32]byte{}, []byte{}
	}
	irMap := make(map[uint64][32]byte, len(rootIndexMap))
	iArr := make(gwatCommon.SorterDescU64, len(rootIndexMap))
	i := 0
	for r, idx := range rootIndexMap {
		irMap[idx] = r
		iArr[i] = idx
		i++
	}
	sort.Sort(iArr)
	keyData := make([]byte, 32*len(iArr))
	for i, idx := range iArr {
		root := irMap[idx]
		copy(keyData[32*i:32*(i+1)], root[:])
	}
	return hash.FastSum256(keyData), keyData
}

// incrInactivity increments all inactivity keys excluding activeKey.
func (c *ForkChoiceCache) incrInactivity(activeKey [32]byte) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, k := range c.cache.Keys() {
		key, ok := k.([32]byte)
		if !ok || key == activeKey {
			continue
		}
		c.inactivity[key]++
	}
}

// removeInactiveItems removes all items with max inactivity score.
func (c *ForkChoiceCache) removeInactiveItems() {
	c.lock.Lock()
	defer c.lock.Unlock()

	for k, score := range c.inactivity {
		//remove item
		if score > maxInactivityScore {
			c.cache.Remove(k)
			delete(c.inactivity, k)
			c.keyCache.Remove(k)
		}
	}
}

// SearchCompatibleFc searches cached forkchoice compatible with rootIndexMap
// and calculate nodes that are not included in forkchoice.
func (c *ForkChoiceCache) SearchCompatibleFc(rootIndexMap map[[32]byte]uint64) (fc *ForkChoice, diff map[[32]byte]uint64) {
	if c.cache.Len() == 0 {
		return nil, rootIndexMap
	}

	diff = make(map[[32]byte]uint64)

	// descending sort node's indexes
	nodeIndexes := make(gwatCommon.SorterDescU64, 0, len(rootIndexMap))
	indexRootMap := make(map[uint64][32]byte, len(rootIndexMap))
	for r, index := range rootIndexMap {
		nodeIndexes = append(nodeIndexes, index)
		indexRootMap[index] = r
	}
	sort.Sort(nodeIndexes)
	// collect descending sorted nodes' roots
	//optimized version
	binRoots := make([]byte, len(nodeIndexes)*32)
	for i, index := range nodeIndexes {
		root := indexRootMap[index]
		copy(binRoots[32*i:32*(i+1)], root[:])
	}

	key, keyData := c.searchCompatibleFcKey(binRoots)
	if len(keyData) > 0 {
		//calc difference
		diffBinLen := len(binRoots) - len(keyData)
		diffLen := diffBinLen / 32
		binDiff := make([]byte, diffBinLen)
		copy(binDiff, keyData[len(keyData):])
		for i := 0; i < diffLen; i++ {
			r := bytesutil.ToBytes32(binRoots[32*i : 32*(i+1)])
			diff[r] = rootIndexMap[r]
		}
		//get cached fc
		value, exists := c.cache.Get(key)
		if exists {
			fc, ok := value.(*ForkChoice)
			if ok {
				c.incrInactivity(key)
				return fc.Copy(), diff
			}
		}
	}
	return nil, rootIndexMap
}

func (c *ForkChoiceCache) searchCompatibleFcKey(binRoots []byte) (key [32]byte, keyRoots []byte) {
	for _, k := range c.keyCache.Keys() {
		val, exists := c.keyCache.Get(k)
		if exists {
			keyData, isOk := val.([]byte)
			if isOk && bytes.Contains(binRoots, keyData) && len(keyRoots) < len(keyData) {
				if v, ok := k.([32]byte); ok {
					key = v
					keyRoots = make([]byte, len(keyData))
					copy(keyRoots, keyData)
				}
			}
		}
	}
	return key, keyRoots
}

func isValidCachedFc(fc *ForkChoice, rootIndexMap map[[32]byte]uint64) bool {
	if fc == nil || fc.store == nil || len(fc.store.nodesIndices) == 0 {
		return false
	}
	fc.store.nodesLock.RLock()
	defer fc.store.nodesLock.RUnlock()
	for _, n := range fc.store.nodes {
		if _, ok := rootIndexMap[n.root]; !ok {
			return false
		}
	}
	return true
}

// getCompatibleFc searches/create forkchoice inctance compatible with rootIndexMap
// and calculate nodes that are not included in forkchoice.
// Helper function for workflow optimization.
func getCompatibleFc(nodesRootIndexMap map[[32]byte]uint64, currFc *ForkChoice) (fc *ForkChoice, diff map[[32]byte]uint64, diffNodes map[uint64]*Node) {
	diffNodes = make(map[uint64]*Node)
	tstart := time.Now()

	log.WithFields(logrus.Fields{
		"nodesIndices":      len(currFc.store.nodesIndices),
		"nodesRootIndexMap": len(nodesRootIndexMap),
	}).Info("FC: getCompatibleFc 000 start")

	// if current fc is equivalent target fc
	//if cacheKeyByRootIndexMap(currFc.store.nodesIndices) == cacheKeyByRootIndexMap(nodesRootIndexMap) {
	if len(currFc.store.nodesIndices) == len(nodesRootIndexMap) {
		fc = currFc //.Copy()
		diff = map[[32]byte]uint64{}
		cacheForkChoice.incrInactivity([32]byte{})

		log.WithFields(logrus.Fields{
			"elapsed":           time.Since(tstart),
			"nodesIndices":      len(currFc.store.nodesIndices),
			"nodesRootIndexMap": len(nodesRootIndexMap),
		}).Info("FC: getCompatibleFc 111 end")

		return fc, diff, diffNodes
	}

	log.WithFields(logrus.Fields{
		"elapsed":           time.Since(tstart),
		"nodesIndices":      len(currFc.store.nodesIndices),
		"nodesRootIndexMap": len(nodesRootIndexMap),
	}).Info("FC: getCompatibleFc 111")
	tstart = time.Now()

	// search cached fc
	fc, diff = cacheForkChoice.SearchCompatibleFc(nodesRootIndexMap)
	if fc != nil {
		log.WithFields(logrus.Fields{
			"elapsed":           time.Since(tstart),
			"nodesIndices":      len(currFc.store.nodesIndices),
			"nodesRootIndexMap": len(nodesRootIndexMap),
		}).Info("FC: getCompatibleFc 222 end")

		for _, idx := range diff {
			diffNodes[idx] = copyNode(currFc.store.nodes[idx])
		}
		return fc, diff, diffNodes
	}

	log.WithFields(logrus.Fields{
		"elapsed":           time.Since(tstart),
		"nodesIndices":      len(currFc.store.nodesIndices),
		"nodesRootIndexMap": len(nodesRootIndexMap),
	}).Info("FC: getCompatibleFc 222")
	tstart = time.Now()

	// create new ForkChoice instance
	fc = New(currFc.store.justifiedEpoch, currFc.store.finalizedEpoch)
	diff = nodesRootIndexMap
	for _, idx := range nodesRootIndexMap {
		diffNodes[idx] = copyNode(currFc.store.nodes[idx])
	}
	cacheForkChoice.incrInactivity([32]byte{})

	log.WithFields(logrus.Fields{
		"elapsed":           time.Since(tstart),
		"nodesIndices":      len(currFc.store.nodesIndices),
		"nodesRootIndexMap": len(nodesRootIndexMap),
	}).Info("FC: getCompatibleFc 333 end")

	return fc, diff, diffNodes
}

// updateCache
// 1. removes inactive items
// 2. checks diff len of nodesRootIndexMap and add item to cache
func updateCache(fc *ForkChoice, diffLen int) {
	cacheForkChoice.removeInactiveItems()
	if diffLen > diffLenToCache {
		cacheForkChoice.Add(fc)
	}
}

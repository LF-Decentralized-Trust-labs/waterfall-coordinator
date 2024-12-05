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

package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
	gwatTypes "gitlab.waterfall.network/waterfall/protocol/gwat/core/types"
)

type spineData struct {
	optSpines   *lru.Cache
	optSpinesMu sync.RWMutex

	lastValidRoot  []byte
	lastValidSlot  types.Slot
	gwatCheckpoint *gwatTypes.Checkpoint //cache for current finalization request checkpoint param
	coordState     *gwatTypes.Checkpoint //cache for current gwat coordinated state
	sync.RWMutex
}

func (s *Service) GetOptimisticSpines(ctx context.Context, baseSpine gwatCommon.Hash) ([]gwatCommon.HashArray, error) {
	if s.isSynchronizing() {
		log.WithError(fmt.Errorf("Node syncing to latest head, not ready to respond")).WithFields(logrus.Fields{
			"Syncing": s.isSynchronizing(),
		}).Warn("Get Optimistic Spines: skipped (synchronizing)")
		optSpines := s.GetCacheOptimisticSpines(baseSpine)
		if len(optSpines) == 0 {
			return optSpines, errNoOptSpines
		}
		return optSpines, nil
	}
	if s.IsGwatSynchronizing() {
		log.WithError(fmt.Errorf("GWAT synchronization process is running, not ready to respond")).WithFields(logrus.Fields{
			"Syncing": s.IsGwatSynchronizing(),
		}).Warn("Get Optimistic Spines: skipped (gwat synchronizing)")
		optSpines := s.GetCacheOptimisticSpines(baseSpine)
		if len(optSpines) == 0 {
			return optSpines, errNoOptSpines
		}
		return optSpines, nil
	}

	tout := time.Duration((params.BeaconConfig().SecondsPerSlot*1000)/4) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, tout)
	defer cancel()
	optSpines, err := s.cfg.ExecutionEngineCaller.ExecutionDagGetOptimisticSpines(reqCtx, baseSpine)
	if err != nil {
		errWrap := fmt.Errorf("could not get gwat optSpines: %v", err)
		log.WithError(errWrap).WithFields(logrus.Fields{
			"baseSpine": baseSpine,
		}).Error("Get Optimistic Spines: retrieving opt spine failed")
		return nil, errWrap
		//return s.GetCacheOptimisticSpines(baseSpine), nil
	}
	s.setCacheOptimisticSpines(baseSpine, optSpines)

	log.WithFields(logrus.Fields{
		"baseSpine": fmt.Sprintf("%#x", baseSpine),
		"opSpines":  optSpines,
	}).Debug("Get Optimistic Spines: success")

	return s.GetCacheOptimisticSpines(baseSpine), nil
}

// setCacheOptimisticSpines cashes current optSpines.
func (s *Service) setCacheOptimisticSpines(baseSpine gwatCommon.Hash, optSpines []gwatCommon.HashArray) {
	s.spineData.optSpinesMu.RLock()
	defer s.spineData.optSpinesMu.RUnlock()
	if s.spineData.optSpines == nil {
		var err error
		s.spineData.optSpines, err = lru.New(8)
		if err != nil {
			log.WithError(err).Error("create optSpines failed")
		}
	}
	s.spineData.optSpines.Remove(baseSpine)
	s.spineData.optSpines.Add(baseSpine, optSpines)
}

// GetCacheOptimisticSpines returns current optSpines.
func (s *Service) GetCacheOptimisticSpines(baseSpine gwatCommon.Hash) []gwatCommon.HashArray {
	s.spineData.optSpinesMu.RLock()
	defer s.spineData.optSpinesMu.RUnlock()

	if s.spineData.optSpines == nil {
		return []gwatCommon.HashArray{}
	}

	data, ok := s.spineData.optSpines.Get(baseSpine)
	if !ok {
		return []gwatCommon.HashArray{}
	}
	optSpines, ok := data.([]gwatCommon.HashArray)
	if !ok {
		return []gwatCommon.HashArray{}
	}
	if optSpines != nil {
		return optSpines
	}
	return []gwatCommon.HashArray{}
}

// SetValidatedBlockInfo caches info of the latest success validated block.
func (s *Service) SetValidatedBlockInfo(lastValidRoot []byte, lastValidSlot types.Slot) {
	s.spineData.RLock()
	defer s.spineData.RUnlock()

	s.spineData.lastValidRoot = lastValidRoot
	s.spineData.lastValidSlot = lastValidSlot
}

// GetValidatedBlockInfo returns info of the latest success validated block.
func (s *Service) GetValidatedBlockInfo() ([]byte, types.Slot) {
	s.spineData.RLock()
	defer s.spineData.RUnlock()

	return s.spineData.lastValidRoot, s.spineData.lastValidSlot
}

// CacheGwatCheckpoint caches the current gwat checkpoint.
func (s *Service) CacheGwatCheckpoint(gwatCheckpoint *gwatTypes.Checkpoint) {
	s.spineData.RLock()
	defer s.spineData.RUnlock()

	s.spineData.gwatCheckpoint = gwatCheckpoint
}

// GetCachedGwatCheckpoint returns the currently cached gwat checkpoint.
func (s *Service) GetCachedGwatCheckpoint(cpRoot []byte) *gwatTypes.Checkpoint {
	s.spineData.RLock()
	defer s.spineData.RUnlock()

	cp := s.spineData.gwatCheckpoint
	if cp != nil && bytes.Equal(cp.Root.Bytes(), cpRoot) {
		return s.spineData.gwatCheckpoint
	}
	return nil
}

// CacheGwatCoordinatedState caches the current gwat coordinated state.
func (s *Service) CacheGwatCoordinatedState(coordState *gwatTypes.Checkpoint) {
	s.spineData.RLock()
	defer s.spineData.RUnlock()

	s.spineData.coordState = coordState
}

// GetCachedGwatCoordinatedState returns the currently cached gwat coordinated state.
func (s *Service) GetCachedGwatCoordinatedState() *gwatTypes.Checkpoint {
	s.spineData.RLock()
	defer s.spineData.RUnlock()

	return s.spineData.coordState
}

// ResetCachedGwatCoordinatedState set the cached gwat coordinated state to nil.
func (s *Service) ResetCachedGwatCoordinatedState() {
	s.spineData.RLock()
	defer s.spineData.RUnlock()

	s.spineData.coordState = nil
}

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

package prevote

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/hash"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

var hashFn = hash.HashProto

// PrevoteCache defines the caches used to satisfy prevote pool interface.
// These caches are KV store for prevotes
type PrevoteCache struct {
	prevoteCacheLock sync.RWMutex
	prevoteCache     map[types.Slot][]*ethpb.PreVote
	seenPrevote      *cache.Cache
}

func NewPrevoteCache() *PrevoteCache {
	secsInEpoch := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	c := cache.New(secsInEpoch*time.Second, 2*secsInEpoch*time.Second)
	pool := &PrevoteCache{
		prevoteCache: make(map[types.Slot][]*ethpb.PreVote),
		seenPrevote:  c,
	}

	return pool
}

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

package cache

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"k8s.io/client-go/tools/cache"
)

var (
	// Delay parameters
	minDelayLs     = float64(100)      // 100 nanoseconds
	maxDelayLs     = float64(10000000) // 0.01 second
	delayFactorLs  = 1.1
	maxCacheSizeLs = uint64(16)
)

// LoadStateCache is used to store the cached results of an AttestationData request.
type LoadStateCache struct {
	cache      *cache.FIFO
	lock       sync.RWMutex
	inProgress map[string]bool
}

// NewLoadStateCache initializes the map and underlying cache.
func NewLoadStateCache() *LoadStateCache {
	return &LoadStateCache{
		cache:      cache.NewFIFO(wrapperToKeyLs),
		inProgress: make(map[string]bool),
	}
}

func (c *LoadStateCache) Get(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error) {
	k := blRootToKey(blockRoot)
	item, exists, err := c.cache.GetByKey(k)
	if err != nil {
		return nil, err
	}
	if exists && item != nil && item.(*loadStateResWrapper).res != nil {
		attestationCacheHit.Inc()
		return item.(*loadStateResWrapper).res.Copy(), nil
	}
	return nil, nil
}

// Get waits for any in progress calculation to complete before returning a
// cached response, if any.
func (c *LoadStateCache) GetWhenReady(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error) {
	k := blRootToKey(blockRoot)
	delay := minDelayLs

	// Another identical request may be in progress already. Let'blockRoot wait until
	// any in progress request resolves or our timeout is exceeded.
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		c.lock.RLock()
		if !c.inProgress[k] {
			c.lock.RUnlock()
			break
		}
		c.lock.RUnlock()

		// This increasing backoff is to decrease the CPU cycles while waiting
		// for the in progress boolean to flip to false.
		time.Sleep(time.Duration(delay) * time.Nanosecond)
		delay *= delayFactorLs
		delay = math.Min(delay, maxDelayLs)
	}
	return c.Get(ctx, blockRoot)
}

// MarkInProgress a request so that any other similar requests will block on
// Get until MarkNotInProgress is called.
func (c *LoadStateCache) MarkInProgress(blockRoot [32]byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	k := blRootToKey(blockRoot)
	if c.inProgress[k] {
		return ErrAlreadyInProgress
	}
	c.inProgress[k] = true
	return nil
}

// MarkNotInProgress will release the lock on a given request. This should be
// called after put.
func (c *LoadStateCache) MarkNotInProgress(blockRoot [32]byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	k := blRootToKey(blockRoot)
	delete(c.inProgress, k)
	return nil
}

// Put the response in the cache.
func (c *LoadStateCache) Put(_ context.Context, blockRoot [32]byte, res state.BeaconState) error {
	data := &loadStateResWrapper{
		req: blockRoot,
		res: res,
	}
	if err := c.cache.AddIfNotPresent(data); err != nil {
		return err
	}
	trim(c.cache, maxCacheSizeLs)
	return nil
}

func wrapperToKeyLs(i interface{}) (string, error) {
	w, ok := i.(*loadStateResWrapper)
	if !ok {
		return "", errors.New("key is not of type *loadStateResWrapper")
	}
	if w == nil {
		return "", errors.New("nil wrapper")
	}
	if w.req == ([32]byte{}) {
		return "", errors.New("nil wrapper.request")
	}
	return blRootToKey(w.req), nil
}

func blRootToKey(blockRoot [32]byte) string {
	return fmt.Sprintf("%#x", blockRoot)
}

type loadStateResWrapper struct {
	req [32]byte
	res state.BeaconState
}

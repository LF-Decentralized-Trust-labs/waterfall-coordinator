package cache

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"k8s.io/client-go/tools/cache"
	"math"
	"sync"
	"time"
)

var (
	// Metrics
	prevoteCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "prevote_cache_miss",
		Help: "The number of prevote data requests that aren't present in the cache.",
	})
	prevoteCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "prevote_cache_hit",
		Help: "The number of prevote data requests that are present in the cache.",
	})
	prevoteCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "prevote_cache_size",
		Help: "The number of prevote data in the prevote cache",
	})
)

// PrevoteCache is used to store the cached results of an PreVoteData request.
type PrevoteCache struct {
	cache      *cache.FIFO
	lock       sync.RWMutex
	inProgress map[string]bool
}

// NewPrevoteCache initializes the map and underlying cache.
func NewPrevoteCache() *PrevoteCache {
	return &PrevoteCache{
		cache:      cache.NewFIFO(wrapperToKey),
		inProgress: make(map[string]bool),
	}
}

// Get waits for any in progress calculation to complete before returning a
// cached response, if any.
func (c *PrevoteCache) Get(ctx context.Context, req *ethpb.PreVoteRequest) (*ethpb.PreVoteData, error) {
	if req == nil {
		return nil, errors.New("nil prevote data request")
	}

	s, e := reqToKeyPrevote(req)
	if e != nil {
		return nil, e
	}

	delay := minDelay

	// Another identical request may be in progress already. Let's wait until
	// any in progress request resolves or our timeout is exceeded.
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		c.lock.RLock()
		if !c.inProgress[s] {
			c.lock.RUnlock()
			break
		}
		c.lock.RUnlock()

		// This increasing backoff is to decrease the CPU cycles while waiting
		// for the in progress boolean to flip to false.
		time.Sleep(time.Duration(delay) * time.Nanosecond)
		delay *= delayFactor
		delay = math.Min(delay, maxDelay)
	}

	item, exists, err := c.cache.GetByKey(s)
	if err != nil {
		return nil, err
	}

	if exists && item != nil && item.(*prevoteReqResWrapper).res != nil {
		prevoteCacheHit.Inc()
		return ethpb.CopyPrevoteData(item.(*prevoteReqResWrapper).res), nil
	}
	prevoteCacheMiss.Inc()
	return nil, nil
}

func (c *PrevoteCache) MarkInProgress(req *ethpb.PreVoteRequest) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s, e := reqToKeyPrevote(req)
	if e != nil {
		return e
	}
	if c.inProgress[s] {
		return ErrAlreadyInProgress
	}
	c.inProgress[s] = true
	return nil
}

func (c *PrevoteCache) MarkNotInProgress(req *ethpb.PreVoteRequest) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s, e := reqToKeyPrevote(req)
	if e != nil {
		return e
	}
	delete(c.inProgress, s)
	return nil
}

func (c *PrevoteCache) Put(_ context.Context, req *ethpb.PreVoteRequest, res *ethpb.PreVoteData) error {
	data := &prevoteReqResWrapper{
		req,
		res,
	}
	if err := c.cache.AddIfNotPresent(data); err != nil {
		return err
	}
	trim(c.cache, maxCacheSize)

	prevoteCacheSize.Set(float64(len(c.cache.List())))
	return nil
}

func reqToKeyPrevote(req *ethpb.PreVoteRequest) (string, error) {
	return fmt.Sprintf("%d", req.Slot), nil
}

type prevoteReqResWrapper struct {
	req *ethpb.PreVoteRequest
	res *ethpb.PreVoteData
}

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
	"context"
	"fmt"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"go.opencensus.io/trace"
)

func (c *PrevoteCache) HasPrevote(pv *ethpb.PreVote) (bool, error) {
	if pv == nil || pv.Data == nil {
		return false, errors.New("Prevote data cannot be nil")
	}

	c.prevoteCacheLock.RLock()
	defer c.prevoteCacheLock.RUnlock()
	if p, ok := c.prevoteCache[pv.Data.Slot]; ok {
		for _, v := range p {
			if pv.Data.Index == v.Data.Index {
				if c, err := v.AggregationBits.Contains(pv.AggregationBits); err != nil {
					return false, err
				} else if c {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (c *PrevoteCache) SavePrevote(pv *ethpb.PreVote) error {
	if pv == nil {
		return nil
	}

	c.prevoteCacheLock.Lock()
	defer c.prevoteCacheLock.Unlock()

	logrus.WithFields(logrus.Fields{
		"pv.slot":    pv.Data.Slot,
		"pv.index":   pv.Data.Index,
		"agrBits":    fmt.Sprintf("%b", pv.AggregationBits.Bytes()),
		"len(cache)": len(c.prevoteCache),
	}).Debug("Prevote: SavePrevote start")

	seen, err := c.hasSeenBit(pv)
	if err != nil {
		return err
	}

	if seen {
		logrus.Infof("Has seen bits in prevote fro slot %d", pv.Data.Slot)
		return nil
	}

	copiedPv := ethpb.CopyPrevote(pv) // Copied.

	if val, exists := c.prevoteCache[pv.Data.Slot]; exists {
		newVal := append(val, copiedPv)
		c.prevoteCache[pv.Data.Slot] = newVal
	} else {
		c.prevoteCache[pv.Data.Slot] = append(c.prevoteCache[pv.Data.Slot], copiedPv)
	}

	logrus.WithFields(logrus.Fields{
		"pv.slot":    pv.Data.Slot,
		"pv.index":   pv.Data.Index,
		"len(cache)": len(c.prevoteCache),
	}).Debug("Prevote: SavePrevote done")

	return nil
}

func (c *PrevoteCache) hasSeenBit(pv *ethpb.PreVote) (bool, error) {
	r, err := hashFn(pv.Data)
	if err != nil {
		return false, err
	}

	v, ok := c.seenPrevote.Get(string(r[:]))
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return false, errors.New("could not convert to bitlist type")
		}
		for _, bit := range seenBits {
			if c, err := bit.Contains(pv.AggregationBits); err != nil {
				return false, err
			} else if c {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *PrevoteCache) GetPrevoteBySlot(ctx context.Context, slot types.Slot) []*ethpb.PreVote {
	_, span := trace.StartSpan(ctx, "operations.prevote.GetPrevoteBySlot")
	defer span.End()

	c.prevoteCacheLock.RLock()
	defer c.prevoteCacheLock.RUnlock()

	logrus.WithFields(logrus.Fields{
		"slot":       slot,
		"len(cache)": len(c.prevoteCache),
	}).Info("Prevote: GetPrevoteBySlot")

	pv := c.prevoteCache[slot]
	if len(pv) > 0 {
		return pv
	}

	return []*ethpb.PreVote{}
}

func (c *PrevoteCache) PurgeOutdatedPrevote(curSlot types.Slot) error {
	c.prevoteCacheLock.RLock()
	defer c.prevoteCacheLock.RUnlock()

	logrus.WithFields(logrus.Fields{
		"len(cache)": len(c.prevoteCache),
	}).Debug("Prevote: PurgeOutdatedPrevote start")

	for k, v := range c.prevoteCache {
		if k < curSlot {
			for _, p := range v {
				err := c.insertSeenBit(p)
				if err != nil {
					return err
				}
			}
			delete(c.prevoteCache, k)
		}
	}

	logrus.WithFields(logrus.Fields{
		"len(cache)": len(c.prevoteCache),
	}).Debug("Prevote: PurgeOutdatedPrevote done")

	return nil
}

func (c *PrevoteCache) insertSeenBit(pv *ethpb.PreVote) error {
	r, err := hashFn(pv.Data)
	if err != nil {
		return err
	}

	v, ok := c.seenPrevote.Get(string(r[:]))
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return errors.New("could not convert to bitlist type")
		}
		alreadyExists := false
		for _, bit := range seenBits {
			if c, err := bit.Contains(pv.AggregationBits); err != nil {
				return err
			} else if c {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			seenBits = append(seenBits, pv.AggregationBits)
		}
		c.seenPrevote.Set(string(r[:]), seenBits, cache.DefaultExpiration /* one epoch */)
		return nil
	}

	c.seenPrevote.Set(string(r[:]), []bitfield.Bitlist{pv.AggregationBits}, cache.DefaultExpiration /* one epoch */)
	return nil
}

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

package transition

import (
	"bytes"
	"context"
	"fmt"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"sync"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
)

type nextEpochCache struct {
	sync.RWMutex
	root  []byte
	state state.BeaconState
}

var (
	nec nextEpochCache
)

// GetNextEpochStateByRoot returns the saved state if the input root matches the root in `nextEpochCache`. Returns nil otherwise.
// This is useful to check before processing slots. With a cache hit, it will return last processed state with slot plus
// one advancement.
func GetNextEpochStateByState(ctx context.Context, st state.BeaconState) (state.BeaconState, error) {
	if st == nil || st.IsNil() {
		log.Error("Get epoch cache: empty state")
		return nil, fmt.Errorf("empty state")
	}
	blHeader := st.LatestBlockHeader()
	if blHeader == nil {
		log.WithFields(logrus.Fields{
			" slot": st.Slot(),
		}).Error("Get epoch cache: bad latest header")
		return nil, fmt.Errorf("bad latest header")
	}
	blRoot, err := blHeader.HashTreeRoot()
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			" slot": st.Slot(),
			"root":  fmt.Sprintf("%#x", blRoot[:]),
		}).Error("Get epoch cache: calc root failed")
		return nil, err
	}
	if blRoot == params.BeaconConfig().ZeroHash {
		log.WithFields(logrus.Fields{
			" slot": st.Slot(),
			"root":  fmt.Sprintf("%#x", blRoot[:]),
		}).Error("Get epoch cache: zero root")
		return nil, fmt.Errorf("zero root")
	}
	nscState := GetNextEpochStateByRoot(ctx, blRoot[:], st.Slot())
	if nscState == nil || nscState.IsNil() {
		log.WithFields(logrus.Fields{
			" slot": st.Slot(),
			"root":  fmt.Sprintf("%#x", blRoot[:]),
		}).Error("Get epoch cache: not found")
		return nil, fmt.Errorf("not found")
	}

	log.WithFields(logrus.Fields{
		" slot":    st.Slot(),
		"root":     fmt.Sprintf("%#x", blRoot[:]),
		"nextSlot": nscState.Slot(),
	}).Info("Get epoch cache: success")

	return nscState, nil
}

// GetNextEpochStateByRoot returns the saved state if the input root matches the root in `nextEpochCache`. Returns nil otherwise.
// This is useful to check before processing slots. With a cache hit, it will return last processed state with slot plus
// one advancement.
func GetNextEpochStateByRoot(_ context.Context, root []byte, slot types.Slot) state.BeaconState {
	nec.RLock()
	defer nec.RUnlock()
	if !bytes.Equal(root, nec.root) || bytes.Equal(root, []byte{}) {
		return nil
	}
	if slot != nec.state.Slot() {
		return nil
	}
	// Returning copied state.
	return nec.state.Copy()
}

// SetNextEpochCache updates the `nextEpochCache`.
func SetNextEpochCache(ctx context.Context, st state.BeaconState) error {
	if st == nil || st.IsNil() {
		log.Error("Set epoch cache: empty state")
		return fmt.Errorf("empty state")
	}
	blHeader := st.LatestBlockHeader()
	if blHeader == nil {
		log.WithFields(logrus.Fields{
			" slot": st.Slot(),
		}).Error("Set epoch cache: bad latest header")
		return fmt.Errorf("bad latest header")
	}
	blRoot, err := blHeader.HashTreeRoot()
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			" slot": st.Slot(),
			"root":  fmt.Sprintf("%#x", blRoot[:]),
		}).Error("Set epoch cache: calc root failed")
		return err
	}
	if blRoot == params.BeaconConfig().ZeroHash {
		log.WithFields(logrus.Fields{
			" slot": st.Slot(),
			"root":  fmt.Sprintf("%#x", blRoot[:]),
		}).Error("Set epoch cache: zero root")
		return fmt.Errorf("zero root")
	}

	nec.Lock()
	defer nec.Unlock()

	nec.root = blRoot[:]
	nec.state = st.Copy()
	return nil
}

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

package db

import (
	"context"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
)

func BlockInfoFetcherFunc(db ReadOnlyDatabase) params.CtxBlockFetcher {
	return func(ctx context.Context, blockRoot [32]byte) (types.ValidatorIndex, types.Slot, uint64, error) {
		block, err := db.Block(ctx, blockRoot)
		if err != nil {
			return 0, 0, 0, err
		}
		if block == nil {
			return 0, 0, 0, ErrNotFound
		}
		votesIncluded := uint64(0)
		for _, att := range block.Block().Body().Attestations() {
			votesIncluded += att.AggregationBits.Count()
		}

		return block.Block().ProposerIndex(), block.Block().Slot(), votesIncluded, nil
	}
}

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

	types "github.com/prysmaticlabs/eth2-types"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

type Pool interface {
	HasPrevote(att *ethpb.PreVote) (bool, error)
	SavePrevote(att *ethpb.PreVote) error
	GetPrevoteBySlot(ctx context.Context, slot types.Slot) []*ethpb.PreVote
	PurgeOutdatedPrevote(curSlot types.Slot) error
}

func NewPool() *PrevoteCache {
	return NewPrevoteCache()
}

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

package beacon

import (
	"context"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/rpc/eth/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpbv "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/eth/v1"
	"go.opencensus.io/trace"
)

// ListBlockVotings retrieves the block votings for the given state.
// If the epoch is not passed in, then the block votings for the epoch of the state will be obtained.
func (bs *Server) ListBlockVotings(ctx context.Context, req *ethpbv.StateBlockVotingsRequest) (*ethpbv.StateBlockVotingsResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.ListBlockVotings")
	defer span.End()
	st, err := bs.StateFetcher.State(ctx, req.StateId)
	if err != nil {
		return nil, helpers.PrepareStateFetchGRPCError(err)
	}

	isOptimistic, err := helpers.IsOptimistic(ctx, st, bs.HeadFetcher)
	if err != nil {
		isOptimistic = false
		//return nil, status.Errorf(codes.Internal, "Could not check if slot's block is optimistic: %v", err)
	}

	blockVotings := st.BlockVoting()
	data := make([]*ethpbv.BlockVoting, len(blockVotings))
	for i, blockVoting := range blockVotings {
		committeeVotes := make([]*ethpbv.CommitteeVote, len(blockVoting.Votes))
		for j, vote := range blockVoting.Votes {
			committeeVotes[j] = &ethpbv.CommitteeVote{
				AggregationBits: bytesutil.SafeCopyBytes(vote.AggregationBits),
				Slot:            vote.Slot,
				Index:           vote.Index,
			}
		}
		data[i] = &ethpbv.BlockVoting{
			Root:       blockVoting.Root,
			Slot:       blockVoting.Slot,
			Candidates: blockVoting.Candidates,
			Votes:      committeeVotes,
		}
	}

	return &ethpbv.StateBlockVotingsResponse{
		Data:                data,
		ExecutionOptimistic: isOptimistic,
	}, nil
}

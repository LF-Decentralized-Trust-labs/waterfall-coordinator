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
	ethpbv "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/eth/v1"
	"go.opencensus.io/trace"
)

// GetEth1Data retrieves the eth1 data for the given state.
// If the epoch is not passed in, then the eth1 data for the epoch of the state will be obtained.
func (bs *Server) GetEth1Data(ctx context.Context, req *ethpbv.StateEth1DataRequest) (*ethpbv.StateEth1DataResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.ListEth1Data")
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

	eth1Data := st.Eth1Data()

	return &ethpbv.StateEth1DataResponse{
		Data: &ethpbv.Eth1Data{
			DepositRoot:  eth1Data.DepositRoot,
			DepositCount: eth1Data.DepositCount,
			BlockHash:    eth1Data.BlockHash,
			Candidates:   eth1Data.Candidates,
		},
		ExecutionOptimistic: isOptimistic,
	}, nil
}

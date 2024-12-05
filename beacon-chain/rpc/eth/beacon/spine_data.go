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

// GetSpineData retrieves the spine data for the given state.
// If the epoch is not passed in, then the spine data for the epoch of the state will be obtained.
func (bs *Server) GetSpineData(ctx context.Context, req *ethpbv.StateSpineDataRequest) (*ethpbv.StateSpineDataResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.ListSpineData")
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

	spineData := st.SpineData()
	parentSpines := make([]*ethpbv.SpinesSeq, len(spineData.ParentSpines))
	for _, spine := range spineData.ParentSpines {
		parentSpines = append(parentSpines, &ethpbv.SpinesSeq{
			Spines: spine.Spines,
		})
	}
	data := &ethpbv.SpineData{
		Spines:       spineData.Spines,
		Prefix:       spineData.Prefix,
		Finalization: spineData.Finalization,
		CpFinalized:  spineData.CpFinalized,
		ParentSpines: parentSpines,
	}

	return &ethpbv.StateSpineDataResponse{
		Data:                data,
		ExecutionOptimistic: isOptimistic,
	}, nil
}

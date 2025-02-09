package debug

import (
	"context"
	"fmt"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	pbrpc "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var toHash = func(x []byte) [32]byte {
	return bytesutil.ToBytes32(x)
}

// GetBeaconState retrieves an ssz-encoded beacon state
// from the beacon node by either a slot or block root.
func (ds *Server) GetBeaconState(
	ctx context.Context,
	req *pbrpc.BeaconStateRequest,
) (*pbrpc.SSZResponse, error) {
	switch q := req.QueryFilter.(type) {
	case *pbrpc.BeaconStateRequest_Slot:
		currentSlot := ds.GenesisTimeFetcher.CurrentSlot()
		requestedSlot := q.Slot
		if requestedSlot > currentSlot {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"Cannot retrieve information about a slot in the future, current slot %d, requested slot %d",
				currentSlot,
				requestedSlot,
			)
		}

		ctx = context.WithValue(ctx, params.BeaconConfig().CtxBlockFetcherKey, db.BlockInfoFetcherFunc(ds.BeaconDB))
		st, err := ds.ReplayerBuilder.ReplayerForSlot(q.Slot).ReplayBlocks(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error replaying blocks for state at slot %d: %v", q.Slot, err))
		}

		encoded, err := st.MarshalSSZ()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not ssz encode beacon state: %v", err)
		}
		return &pbrpc.SSZResponse{
			Encoded: encoded,
		}, nil
	case *pbrpc.BeaconStateRequest_BlockRoot:
		st, err := ds.StateGen.StateByRoot(ctx, toHash(q.BlockRoot))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not compute state by block root: %v", err)
		}
		encoded, err := st.MarshalSSZ()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not ssz encode beacon state: %v", err)
		}
		return &pbrpc.SSZResponse{
			Encoded: encoded,
		}, nil
	}
	return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("%s", "Need to specify either a block root or slot to request state"))
}

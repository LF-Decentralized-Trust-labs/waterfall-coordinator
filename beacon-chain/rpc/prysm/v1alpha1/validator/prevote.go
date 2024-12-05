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

package validator

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetPrevoteData requests that the beacon node produce an prevote data object,
// which the validator acting as an attester will then sign.
func (vs *Server) GetPrevoteData(ctx context.Context, req *ethpb.PreVoteRequest) (*ethpb.PreVoteData, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.RequestPrevote")
	defer span.End()
	span.AddAttributes(
		trace.Int64Attribute("slot", int64(req.Slot)),
		trace.Int64Attribute("committeeIndex", int64(req.CommitteeIndex)),
	)

	if params.BeaconConfig().PrevotingDisabled {
		return nil, status.Errorf(codes.Unavailable, "Prevoting process is disabled")
	}

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	// result is not depending on CommitteeIndex
	res, err := vs.PrevoteCache.Get(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve data from prevote cache: %v", err)
	}
	if res != nil {
		res.Index = req.CommitteeIndex
		return res, nil
	}

	if err := vs.PrevoteCache.MarkInProgress(req); err != nil {
		if errors.Is(err, cache.ErrAlreadyInProgress) {
			res, err := vs.PrevoteCache.Get(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not retrieve data from prevote cache: %v", err)
			}
			if res == nil {
				return nil, status.Error(codes.DataLoss, "A request was in progress and resolved to nil")
			}
			res.Index = req.CommitteeIndex
			return res, nil
		}
		return nil, status.Errorf(codes.Internal, "Could not mark prevote as in-progress: %v", err)
	}
	defer func() {
		if err := vs.PrevoteCache.MarkNotInProgress(req); err != nil {
			log.WithError(err).Error("Could not mark cache not in progress")
		}
	}()

	optSpines, err := vs.getOptimisticSpine(ctx)
	if err != nil {
		errWrap := fmt.Errorf("could not get gwat optimistic spines: %v", err)
		log.WithError(errWrap).WithFields(logrus.Fields{
			"slot": req.Slot,
		}).Error("Collect prevote data: Could not retrieve of gwat optimistic spines")
		return nil, errWrap
	}
	if len(optSpines) == 0 {
		log.Errorf("Collect prevote data: no optimistic spines was retrieved for slot: %v", req.Slot)
	}

	candidates := gwatCommon.HashArray{}
	if len(optSpines) > 0 {

		currHead, err := vs.HeadFetcher.HeadState(ctx)
		if err != nil {
			log.WithError(err).Error("Collect prevote data:  Could not retrieve head state")
			return nil, status.Errorf(codes.Internal, "Could not retrieve head state: %v", err)
		}
		jCpRoot := bytesutil.ToBytes32(currHead.CurrentJustifiedCheckpoint().Root)
		if currHead.CurrentJustifiedCheckpoint().Epoch == 0 {
			jCpRoot, err = vs.BeaconDB.GenesisBlockRoot(ctx)
			if err != nil {
				log.WithError(err).Error("Collect prevote data:  retrieving of genesis root")
				return nil, status.Errorf(codes.Internal, "Could not retrieve of genesis root: %v", err)
			}
		}

		//calculate optimistic parent root
		parentRoot, err := vs.HeadFetcher.ForkChoicer().GetParentByOptimisticSpines(ctx, optSpines, jCpRoot)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"extOptSpines": optSpines,
			}).Error("Collect prevote data: Could not retrieve of supported root")
			return nil, status.Errorf(codes.Internal, "retrieve of supported root error: %v", err)
		}

		log.WithFields(logrus.Fields{
			"1:parentRoot":   fmt.Sprintf("%#x", parentRoot),
			"2:extOptSpines": len(optSpines),
		}).Info("Collect prevote data: retrieved gwat optimistic spines")

		parentState, err := vs.StateGen.StateByRoot(ctx, parentRoot)
		if err != nil {
			return nil, fmt.Errorf("could not get parent state %v", err)
		}
		candidates = helpers.CalculateCandidates(parentState, optSpines)
	}

	res = &ethpb.PreVoteData{
		Slot:       req.Slot,
		Index:      req.CommitteeIndex,
		Candidates: candidates.ToBytes(),
	}

	if err = vs.PrevoteCache.Put(ctx, req, res); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not store prevote data in cache: %v", err)
	}

	log.WithFields(logrus.Fields{
		"1:req.Slot":           req.Slot,
		"2:req.CommitteeIndex": req.CommitteeIndex,
		"3:candidates":         candidates,
	}).Info("Collect prevote data: success")

	return res, nil
}

func (vs *Server) ProposePrevote(ctx context.Context, pv *ethpb.PreVote) (*ethpb.PrevoteResponse, error) {
	ctx, span := trace.StartSpan(ctx, "PrevoteServer.ProposePrevote")
	defer span.End()

	if params.BeaconConfig().PrevotingDisabled {
		return nil, status.Errorf(codes.Unavailable, "Prevoting process is disabled")
	}

	if _, err := bls.SignatureFromBytes(pv.Signature); err != nil {
		return nil, status.Error(codes.InvalidArgument, "Incorrect prevote signature")
	}

	root, err := pv.Data.HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not tree hash prevote: %v", err)
	}

	// Determine subnet to broadcast prevote to
	wantedEpoch := slots.ToEpoch(pv.Data.Slot)
	vals, err := vs.HeadFetcher.HeadValidatorsIndices(ctx, wantedEpoch)
	if err != nil {
		return nil, err
	}
	subnet := helpers.ComputeSubnetPrevotingBySlot(uint64(len(vals)), pv.Data.Slot)

	// Broadcast the new prevote to the network.
	if err := vs.P2P.BroadcastPrevoting(ctx, subnet, pv); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast prevote: %v", err)
	}

	return &ethpb.PrevoteResponse{
		PrevoteDataRoot: root[:],
	}, nil
}

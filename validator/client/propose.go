package client

// Validator client proposer functions.
import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/async"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/rand"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	validatorpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/validator-client"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
	prysmTime "gitlab.waterfall.network/waterfall/protocol/coordinator/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/validator/client/iface"
	"go.opencensus.io/trace"
	"google.golang.org/protobuf/types/known/emptypb"
)

type signingFunc func(context.Context, *validatorpb.SignRequest) (bls.Signature, error)

const domainDataErr = "could not get domain data"
const signingRootErr = "could not get signing root"

// ProposeBlock proposes a new beacon block for a given slot. This method collects the
// previous beacon block, any pending deposits, and ETH1 data from the beacon
// chain node to construct the new block. The new block is then processed with
// the state root computation, and finally signed by the validator before being
// sent back to the beacon node for broadcasting.
func (v *validator) ProposeBlock(ctx context.Context, slot types.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	if slot == 0 {
		log.Debug("Assigned to genesis slot, skipping proposal")
		return
	}
	ctx, span := trace.StartSpan(ctx, "validator.ProposeBlock")
	defer span.End()

	lock := async.NewMultilock(fmt.Sprint(iface.RoleProposer), string(pubKey[:]))
	lock.Lock()
	defer lock.Unlock()

	fmtKey := fmt.Sprintf("%#x", pubKey[:])
	span.AddAttributes(trace.StringAttribute("validator", fmtKey))
	log := log.WithField("pubKey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])))

	// Sign randao reveal, it's used to request block from beacon node
	epoch := types.Epoch(slot / params.BeaconConfig().SlotsPerEpoch)
	randaoReveal, err := v.signRandaoReveal(ctx, pubKey, epoch, slot)
	if err != nil {
		log.WithError(err).Error("Failed to sign randao reveal")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	g, err := v.getGraffiti(ctx, pubKey)
	if err != nil {
		// Graffiti is not a critical enough to fail block production and cause
		// validator to miss block reward. When failed, validator should continue
		// to produce the block.
		log.WithError(err).Warn("Could not get graffiti")
	}

	// Request block from beacon node
	b, err := v.validatorClient.GetBeaconBlock(ctx, &ethpb.BlockRequest{
		Slot:         slot,
		RandaoReveal: randaoReveal,
		Graffiti:     g,
	})
	if err != nil {
		log.WithField("blockSlot", slot).WithError(err).Error("Failed to request block from beacon node")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	// Sign returned block from beacon node
	wb, err := wrapper.WrappedBeaconBlock(b.Block)
	if err != nil {
		log.WithError(err).Error("Failed to wrap block")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	sig, signingRoot, err := v.signBlock(ctx, pubKey, epoch, slot, wb)
	if err != nil {
		log.WithError(err).Error("Failed to sign block")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	blk, err := wrapper.BuildSignedBeaconBlock(wb, sig)
	if err != nil {
		log.WithError(err).Error("Failed to build signed beacon block")
		return
	}

	if err := v.slashableProposalCheck(ctx, pubKey, blk, signingRoot); err != nil {
		log.WithFields(
			blockLogFields(pubKey, wb, nil),
		).WithError(err).Error("Failed block slashing protection check")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	// Propose and broadcast block via beacon node
	proposal, err := blk.PbGenericBlock()
	if err != nil {
		log.WithError(err).Error("Failed to create proposal request")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}
	blkResp, err := v.validatorClient.ProposeBeaconBlock(ctx, proposal)
	if err != nil {
		log.WithError(err).Error("Failed to propose block")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	span.AddAttributes(
		trace.StringAttribute("blockRoot", fmt.Sprintf("%#x", blkResp.BlockRoot)),
		trace.Int64Attribute("numDeposits", int64(len(blk.Block().Body().Deposits()))),
		trace.Int64Attribute("numAttestations", int64(len(blk.Block().Body().Attestations()))),
	)

	blkRoot := fmt.Sprintf("%#x", bytesutil.Trunc(blkResp.BlockRoot))
	log.WithFields(logrus.Fields{
		"slot":            blk.Block().Slot(),
		"blockRoot":       blkRoot,
		"numAttestations": len(blk.Block().Body().Attestations()),
		"numDeposits":     len(blk.Block().Body().Deposits()),
		"graffiti":        string(blk.Block().Body().Graffiti()),
		"fork":            version.String(blk.Block().Version()),
	}).Info("Submitted new block")

	if v.emitAccountMetrics {
		ValidatorProposeSuccessVec.WithLabelValues(fmtKey).Inc()
	}
}

// ProposeExit performs a voluntary exit on a validator.
// The exit is signed by the validator before being sent to the beacon node for broadcasting.
func ProposeExit(
	ctx context.Context,
	validatorClient ethpb.BeaconNodeValidatorClient,
	nodeClient ethpb.NodeClient,
	signer signingFunc,
	pubKey []byte,
) error {
	ctx, span := trace.StartSpan(ctx, "validator.ProposeExit")
	defer span.End()

	indexResponse, err := validatorClient.ValidatorIndex(ctx, &ethpb.ValidatorIndexRequest{PublicKey: pubKey})
	if err != nil {
		return errors.Wrap(err, "gRPC call to get validator index failed")
	}
	genesisResponse, err := nodeClient.GetGenesis(ctx, &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "gRPC call to get genesis time failed")
	}
	totalSecondsPassed := prysmTime.Now().Unix() - genesisResponse.GenesisTime.Seconds
	currentEpoch := types.Epoch(uint64(totalSecondsPassed) / uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot)))

	exit := &ethpb.VoluntaryExit{Epoch: currentEpoch, ValidatorIndex: indexResponse.Index}
	exitResp, err := validatorClient.ProposeExit(ctx, exit)
	if err != nil {
		return errors.Wrap(err, "failed to propose voluntary exit")
	}

	span.AddAttributes(
		trace.StringAttribute("exitRoot", fmt.Sprintf("%#x", exitResp.ExitRoot)),
	)

	return nil
}

// Sign randao reveal with randao domain and private key.
func (v *validator) signRandaoReveal(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, epoch types.Epoch, slot types.Slot) ([]byte, error) {
	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainRandao[:])
	if err != nil {
		return nil, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, errors.New(domainDataErr)
	}

	var randaoReveal bls.Signature
	sszUint := types.SSZUint64(epoch)
	root, err := signing.ComputeSigningRoot(&sszUint, domain.SignatureDomain)
	if err != nil {
		return nil, err
	}
	randaoReveal, err = v.keyManager.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_Epoch{Epoch: epoch},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, err
	}
	return randaoReveal.Marshal(), nil
}

// Sign block with proposer domain and private key.
// Returns the signature, block signing root, and any error.
func (v *validator) signBlock(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, epoch types.Epoch, slot types.Slot, b block.BeaconBlock) ([]byte, [32]byte, error) {
	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainBeaconProposer[:])
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, [32]byte{}, errors.New(domainDataErr)
	}

	blockRoot, err := signing.ComputeSigningRoot(b, domain.SignatureDomain)
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, signingRootErr)
	}
	sig, err := v.keyManager.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     blockRoot[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          b.AsSignRequestObject(),
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, "could not sign block proposal")
	}
	return sig.Marshal(), blockRoot, nil
}

// Gets the graffiti from cli or file for the validator public key.
func (v *validator) getGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	// When specified, default graffiti from the command line takes the first priority.
	if len(v.graffiti) != 0 {
		return v.graffiti, nil
	}

	if v.graffitiStruct == nil {
		return nil, errors.New("graffitiStruct can't be nil")
	}

	// When specified, individual validator specified graffiti takes the second priority.
	idx, err := v.validatorClient.ValidatorIndex(ctx, &ethpb.ValidatorIndexRequest{PublicKey: pubKey[:]})
	if err != nil {
		return []byte{}, err
	}
	g, ok := v.graffitiStruct.Specific[idx.Index]
	if ok {
		return []byte(g), nil
	}

	// When specified, a graffiti from the ordered list in the file take third priority.
	if v.graffitiOrderedIndex < uint64(len(v.graffitiStruct.Ordered)) {
		graffiti := v.graffitiStruct.Ordered[v.graffitiOrderedIndex]
		v.graffitiOrderedIndex = v.graffitiOrderedIndex + 1
		err := v.db.SaveGraffitiOrderedIndex(ctx, v.graffitiOrderedIndex)
		if err != nil {
			return nil, errors.Wrap(err, "failed to update graffiti ordered index")
		}
		return []byte(graffiti), nil
	}

	// When specified, a graffiti from the random list in the file take fourth priority.
	if len(v.graffitiStruct.Random) != 0 {
		r := rand.NewGenerator()
		r.Seed(time.Now().Unix())
		i := r.Uint64() % uint64(len(v.graffitiStruct.Random))
		return []byte(v.graffitiStruct.Random[i]), nil
	}

	// Finally, default graffiti if specified in the file will be used.
	if v.graffitiStruct.Default != "" {
		return []byte(v.graffitiStruct.Default), nil
	}

	return []byte{}, nil
}

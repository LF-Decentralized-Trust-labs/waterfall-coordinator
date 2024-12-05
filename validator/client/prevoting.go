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

package client

import (
	"context"
	"fmt"
	"strings"

	types "github.com/prysmaticlabs/eth2-types"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/async"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/tracing"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	validatorpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/validator-client"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/validator/client/iface"
	"go.opencensus.io/trace"
)

// SubmitPrevote completes the validator client's prevote responsibility at a given slot.
// It fetches the latest beacon block head along with the latest canonical beacon state
// information in order to sign the block and include information about the validator's
// participation in voting on the block.
func (v *validator) SubmitPrevote(ctx context.Context, slot types.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitPrevote")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))

	var b strings.Builder
	if err := b.WriteByte(byte(iface.RoleAttester)); err != nil {
		log.WithError(err).Error("Could not write role byte for lock key while prevote")
		tracing.AnnotateError(span, err)
		return
	}
	_, err := b.Write(pubKey[:])
	if err != nil {
		log.WithError(err).Error("Could not write pubkey bytes for lock key while prevote")
		tracing.AnnotateError(span, err)
		return
	}
	lock := async.NewMultilock(b.String())
	lock.Lock()
	defer lock.Unlock()

	fmtKey := fmt.Sprintf("%#x", pubKey[:])
	log := log.WithField("pubKey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:]))).WithField("slot", slot)
	duty, err := v.dutyForPrevote(pubKey, slot)
	if err != nil {
		log.WithError(err).Error("Could not fetch validator assignment while prevote")
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}
	if len(duty.Committee) == 0 {
		log.Warnf("Empty committee for validator prevote")
		return
	}

	// Pass slot-1 because this parameter enters SubmitPrevote method as current slot+1
	v.waitToSlotTwoThirds(ctx, slot-1)

	req := &ethpb.PreVoteRequest{
		Slot:           slot,
		CommitteeIndex: duty.CommitteeIndex,
	}
	data, err := v.validatorClient.GetPrevoteData(ctx, req)
	if err != nil {
		log.WithError(err).Errorf("Could not request prevote data to sign at slot %v", req.Slot)
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	log.WithFields(logrus.Fields{
		"req.Slot":           req.Slot,
		"req.CommitteeIndex": req.CommitteeIndex,
		"data.Slot":          data.Slot,
		"data.Index":         data.Index,
		"data.Candidates":    fmt.Sprintf("%#x", data.Candidates),
	}).Info("Prevote: SubmitPrevote")

	indexedPrevote := &ethpb.IndexedPreVote{
		AttestingIndices: []uint64{uint64(duty.ValidatorIndex)},
		Data:             data,
	}

	domain, signingRoot, err := v.getDomainAndSigningRootPrevote(ctx, indexedPrevote)
	if err != nil {
		log.WithError(err).Errorf("Could not get domain and signing root from prevote at slot %v", req.Slot)
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	sig, _, err := v.signPrevote(ctx, pubKey, data, slot, domain, signingRoot)
	if err != nil {
		log.WithError(err).Errorf("Could not sign prevote at slot %v", req.Slot)
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	var indexInCommittee uint64
	var found bool
	for i, vID := range duty.Committee {
		if vID == duty.ValidatorIndex {
			indexInCommittee = uint64(i)
			found = true
			break
		}
	}
	if !found {
		log.Errorf("Validator ID %d not found in committee of %v", duty.ValidatorIndex, duty.Committee)
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	aggregationBitfield := bitfield.NewBitlist(uint64(len(duty.Committee)))
	aggregationBitfield.SetBitAt(indexInCommittee, true)
	prevote := &ethpb.PreVote{
		Data:            data,
		AggregationBits: aggregationBitfield,
		Signature:       sig,
	}

	// Set the signature of the attestation and send it out to the beacon node.
	indexedPrevote.Signature = sig

	pvResp, err := v.validatorClient.ProposePrevote(ctx, prevote)
	if err != nil {
		log.WithError(err).Errorf("Could not submit prevote to beacon node at slot %v", req.Slot)
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	span.AddAttributes(
		trace.Int64Attribute("slot", int64(slot)), // lint:ignore uintcast -- This conversion is OK for tracing.
		trace.StringAttribute("prevoteHash", fmt.Sprintf("%#x", pvResp.PrevoteDataRoot)),
		trace.Int64Attribute("committeeIndex", int64(data.Index)),
		trace.StringAttribute("bitfield", fmt.Sprintf("%#x", aggregationBitfield)),
	)

	if v.emitAccountMetrics {
		ValidatorAttestSuccessVec.WithLabelValues(fmtKey).Inc()
		ValidatorAttestedSlotsGaugeVec.WithLabelValues(fmtKey).Set(float64(slot))
	}
}

func (v *validator) getDomainAndSigningRootPrevote(ctx context.Context, data *ethpb.IndexedPreVote) (*ethpb.DomainResponse, [32]byte, error) {
	domain, err := v.domainData(ctx, slots.ToEpoch(data.Data.Slot), params.BeaconConfig().DomainBeaconAttester[:])
	if err != nil {
		return nil, [32]byte{}, err
	}
	root, err := signing.ComputeSigningRoot(data.Data, domain.SignatureDomain)
	if err != nil {
		return nil, [32]byte{}, err
	}
	return domain, root, nil
}

// Given validator's public key, this function returns the signature of a prevote data and its signing root.
func (v *validator) signPrevote(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, data *ethpb.PreVoteData,
	slot types.Slot, domain *ethpb.DomainResponse, root [32]byte) ([]byte, [32]byte, error) {
	sig, err := v.keyManager.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_PrevoteData{PrevoteData: data},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, [32]byte{}, err
	}

	return sig.Marshal(), root, nil
}

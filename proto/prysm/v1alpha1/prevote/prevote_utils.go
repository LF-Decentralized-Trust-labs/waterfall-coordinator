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
	"fmt"
	"sort"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/attestation"
	"go.opencensus.io/trace"
)

func ConvertToIndexed(ctx context.Context, prevote *ethpb.PreVote, committee []types.ValidatorIndex) (*ethpb.IndexedPreVote, error) {
	_, span := trace.StartSpan(ctx, "prevoteutil.ConvertToIndexed")
	defer span.End()

	prevoteIndices, err := attestation.AttestingIndices(prevote.AggregationBits, committee)
	if err != nil {
		return nil, err
	}

	sort.Slice(prevoteIndices, func(i, j int) bool {
		return prevoteIndices[i] < prevoteIndices[j]
	})
	inPrevote := &ethpb.IndexedPreVote{
		Data:             prevote.Data,
		Signature:        prevote.Signature,
		AttestingIndices: prevoteIndices,
	}
	return inPrevote, err
}

func IsValidPrevoteIndices(ctx context.Context, indexedPrevote *ethpb.IndexedPreVote) error {
	_, span := trace.StartSpan(ctx, "prevoteutil.IsValidPrevoteIndices")
	defer span.End()

	if indexedPrevote == nil || indexedPrevote.Data == nil || indexedPrevote.AttestingIndices == nil {
		return errors.New("nil or missing indexed prevote data")
	}
	indices := indexedPrevote.AttestingIndices
	if len(indices) == 0 {
		return errors.New("expected non-empty attesting indices")
	}
	if uint64(len(indices)) > params.BeaconConfig().MaxValidatorsPerCommittee {
		return fmt.Errorf("validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE, %d > %d", len(indices), params.BeaconConfig().MaxValidatorsPerCommittee)
	}
	for i := 1; i < len(indices); i++ {
		if indices[i-1] >= indices[i] {
			return errors.New("attesting indices is not uniquely sorted")
		}
	}
	return nil
}

func VerifyIndexedPrevoteSig(ctx context.Context, indextedPrevote *ethpb.IndexedPreVote, pubKeys []bls.PublicKey, domain []byte) error {
	_, span := trace.StartSpan(ctx, "attestationutil.VerifyIndexedAttestationSig")
	defer span.End()
	indices := indextedPrevote.AttestingIndices
	messageHash, err := signing.ComputeSigningRoot(indextedPrevote.Data, domain)
	if err != nil {
		return errors.Wrap(err, "could not get signing root of object")
	}

	sig, err := bls.SignatureFromBytes(indextedPrevote.Signature)
	if err != nil {
		return errors.Wrap(err, "could not convert bytes to signature")
	}

	voted := len(indices) > 0
	if voted && !sig.FastAggregateVerify(pubKeys, messageHash) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}

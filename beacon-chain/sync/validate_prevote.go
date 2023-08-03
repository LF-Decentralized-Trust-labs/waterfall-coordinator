package sync

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/blocks"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/features"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/tracing"
	eth "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/prevote"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	"go.opencensus.io/trace"
)

func (s *Service) validateCommitteeIndexPrevote(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Prevote processing requires the target block to be present in the database, so we'll skip
	// validating or processing prevote until fully synced.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	// We should not attempt to process this message if the node is running in optimistic mode.
	// We just ignore in p2p so that the peer is not penalized.
	optimistic, err := s.cfg.chain.IsOptimistic(ctx)
	if err == nil {
		return pubsub.ValidationReject, err
	}
	if optimistic {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validatePrevote")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	prevote, ok := m.(*eth.PreVote)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	if prevote == nil {
		return pubsub.ValidationReject, errNilMessage
	}

	// Do not process slot 0 prevote.
	if prevote.Data.Slot == 0 {
		return pubsub.ValidationIgnore, nil
	}

	bState, err := s.cfg.chain.HeadState(ctx)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	validationRes, err := s.validateUnaggregatedPrevoteTopic(ctx, prevote, bState, *msg.Topic)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, err
	}

	validationRes, err = s.validateUnaggregatedPrevoteWithState(ctx, prevote, bState)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, err
	}

	s.setSeenCommitteeIndicesSlot(prevote.Data.Slot, prevote.Data.Index, prevote.AggregationBits)

	msg.ValidatorData = prevote
	return pubsub.ValidationAccept, nil
}

// This validates beacon unaggregated prevote has correct topic string.
func (s *Service) validateUnaggregatedPrevoteTopic(ctx context.Context, p *eth.PreVote, bs state.ReadOnlyBeaconState, t string) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateUnaggregatedPrevoteTopic")
	defer span.End()

	valCount, err := helpers.ActiveValidatorCount(ctx, bs, slots.ToEpoch(p.Data.Slot))
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}
	count := helpers.SlotCommitteeCount(valCount)
	if uint64(p.Data.Index) > count {
		return pubsub.ValidationReject, errors.Errorf("committee index %d > %d", p.Data.Index, count)
	}
	subnet := helpers.ComputeSubnetForPrevote(valCount, p)
	format := p2p.GossipTypeMapping[reflect.TypeOf(&eth.PreVote{})]
	digest, err := s.currentForkDigest()
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}
	if !strings.HasPrefix(t, fmt.Sprintf(format, digest, subnet)) {
		return pubsub.ValidationReject, errors.New("prevotes's subnet does not match with pubsub topic")
	}

	return pubsub.ValidationAccept, nil
}

func (s *Service) validateUnaggregatedPrevoteWithState(ctx context.Context, p *eth.PreVote, bs state.ReadOnlyBeaconState) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateUnaggregatedPrevoteWithState")
	defer span.End()

	committee, err := helpers.BeaconCommitteeFromState(ctx, bs, p.Data.Slot, p.Data.Index)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	// Verify number of aggregation bits matches the committee size.
	if err := helpers.VerifyBitfieldLength(p.AggregationBits, uint64(len(committee))); err != nil {
		return pubsub.ValidationReject, err
	}

	// Prevote must be unaggregated and the bit index must exist in the range of committee indices.
	if p.AggregationBits.Count() != 1 || p.AggregationBits.BitIndices()[0] >= len(committee) {
		return pubsub.ValidationReject, errors.New("prevote bitfield is invalid")
	}

	if features.Get().EnableBatchVerification {
		set, err := blocks.PrevoteSignatureBatch(ctx, bs, []*eth.PreVote{p})
		if err != nil {
			tracing.AnnotateError(span, err)
			return pubsub.ValidationReject, err
		}
		return s.validateWithBatchVerifier(ctx, "prevote", set)
	}
	if err := verifyPrevoteSignature(ctx, bs, p); err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}
	return pubsub.ValidationAccept, nil
}

func verifyPrevoteSignature(ctx context.Context, beaconState state.ReadOnlyBeaconState, pv *eth.PreVote) error {
	committee, err := helpers.BeaconCommitteeFromState(ctx, beaconState, pv.Data.Slot, pv.Data.Index)
	if err != nil {
		return err
	}
	indexedPrevote, err := prevote.ConvertToIndexed(ctx, pv, committee)
	if err != nil {
		return err
	}
	return VerifyIndexedPrevote(ctx, beaconState, indexedPrevote)
}

func VerifyIndexedPrevote(ctx context.Context, beaconState state.ReadOnlyBeaconState, ipv *eth.IndexedPreVote) error {
	ctx, span := trace.StartSpan(ctx, "VerifyIndexedPrevote")
	defer span.End()

	if err := prevote.IsValidPrevoteIndices(ctx, ipv); err != nil {
		return err
	}
	domain, err := signing.Domain(
		beaconState.Fork(),
		slots.ToEpoch(ipv.Data.Slot),
		params.BeaconConfig().DomainBeaconAttester,
		beaconState.GenesisValidatorsRoot(),
	)
	if err != nil {
		return err
	}
	indices := ipv.AttestingIndices
	var pubkeys []bls.PublicKey
	for i := 0; i < len(indices); i++ {
		pubkeyAtIdx := beaconState.PubkeyAtIndex(types.ValidatorIndex(indices[i]))
		pk, err := bls.PublicKeyFromBytes(pubkeyAtIdx[:])
		if err != nil {
			return errors.Wrap(err, "could not deserialize validator public key")
		}
		pubkeys = append(pubkeys, pk)
	}
	return prevote.VerifyIndexedPrevoteSig(ctx, ipv, pubkeys, domain)
}

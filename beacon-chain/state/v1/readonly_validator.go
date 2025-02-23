package v1

import (
	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

var (
	// ErrNilWrappedValidator returns when caller attempts to wrap a nil pointer validator.
	ErrNilWrappedValidator = errors.New("nil validator cannot be wrapped as readonly")
)

// readOnlyValidator returns a wrapper that only allows fields from a validator
// to be read, and prevents any modification of internal validator fields.
type readOnlyValidator struct {
	validator *ethpb.Validator
}

var _ = state.ReadOnlyValidator(readOnlyValidator{})

// NewValidator initializes the read only wrapper for validator.
func NewValidator(v *ethpb.Validator) (state.ReadOnlyValidator, error) {
	rov := readOnlyValidator{
		validator: v,
	}
	if rov.IsNil() {
		return nil, ErrNilWrappedValidator
	}
	return rov, nil
}

// EffectiveBalance returns the effective balance of the
// read only validator.
func (v readOnlyValidator) EffectiveBalance() uint64 {
	return v.validator.EffectiveBalance
}

// ActivationEligibilityEpoch returns the activation eligibility epoch of the
// read only validator.
func (v readOnlyValidator) ActivationEligibilityEpoch() types.Epoch {
	return v.validator.ActivationEligibilityEpoch
}

// ActivationEpoch returns the activation epoch of the
// read only validator.
func (v readOnlyValidator) ActivationEpoch() types.Epoch {
	return v.validator.ActivationEpoch
}

// WithdrawableEpoch returns the withdrawable epoch of the
// read only validator.
func (v readOnlyValidator) WithdrawableEpoch() types.Epoch {
	return v.validator.WithdrawableEpoch
}

// WithdrawalOps returns the array of withdrawal operation from finalized checkpoint of the
// read only validator.
func (v readOnlyValidator) WithdrawalOps() []*ethpb.WithdrawalOp {
	srcVal := v.validator.WithdrawalOps
	cpy := make([]*ethpb.WithdrawalOp, len(srcVal))

	for i, sv := range srcVal {
		if sv == nil {
			continue
		}
		h := make([]byte, len(sv.Hash))
		copy(h, sv.Hash)
		cpy[i] = &ethpb.WithdrawalOp{
			Amount: sv.Amount,
			Hash:   h,
			Slot:   sv.Slot,
		}
	}
	return cpy
}

// ActivationHash returns the tx hash of activation of
// read only validator.
func (v readOnlyValidator) ActivationHash() []byte {
	srcVal := v.validator.ActivationHash
	cpy := make([]byte, len(srcVal))
	copy(cpy, srcVal)
	return cpy
}

// ExitHash returns the tx hash of deactivation of
// read only validator.
func (v readOnlyValidator) ExitHash() []byte {
	srcVal := v.validator.ExitHash
	cpy := make([]byte, len(srcVal))
	copy(cpy, srcVal)
	return cpy
}

// ExitEpoch returns the exit epoch of the
// read only validator.
func (v readOnlyValidator) ExitEpoch() types.Epoch {
	return v.validator.ExitEpoch
}

// PublicKey returns the public key of the
// read only validator.
func (v readOnlyValidator) PublicKey() [fieldparams.BLSPubkeyLength]byte {
	var pubkey [fieldparams.BLSPubkeyLength]byte
	copy(pubkey[:], v.validator.PublicKey)
	return pubkey
}

// CreatorAddress returns the Creator Address of the
// read only validator.
func (v readOnlyValidator) CreatorAddress() []byte {
	bytes := make([]byte, len(v.validator.CreatorAddress))
	copy(bytes, v.validator.CreatorAddress)
	return bytes
}

// WithdrawalCredentials returns the withdrawal credentials of the
// read only validator.
func (v readOnlyValidator) WithdrawalCredentials() []byte {
	creds := make([]byte, len(v.validator.WithdrawalCredentials))
	copy(creds, v.validator.WithdrawalCredentials)
	return creds
}

// Slashed returns the read only validator is slashed.
func (v readOnlyValidator) Slashed() bool {
	return v.validator.Slashed
}

// IsNil returns true if the validator is nil.
func (v readOnlyValidator) IsNil() bool {
	return v.validator == nil
}

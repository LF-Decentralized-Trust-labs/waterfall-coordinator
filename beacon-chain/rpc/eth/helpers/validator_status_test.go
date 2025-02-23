package helpers

import (
	"strconv"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/eth/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/migration"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func Test_ValidatorStatus(t *testing.T) {
	farFutureEpoch := params.BeaconConfig().FarFutureEpoch

	type args struct {
		validator *ethpb.Validator
		epoch     types.Epoch
	}
	tests := []struct {
		name    string
		args    args
		want    ethpb.ValidatorStatus
		wantErr bool
	}{
		{
			name: "pending initialized",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:            farFutureEpoch,
					ActivationEligibilityEpoch: farFutureEpoch,
					ActivationHash:             make([]byte, 32),
					ExitHash:                   make([]byte, 32),
					WithdrawalOps:              make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_PENDING,
		},
		{
			name: "pending queued",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:            10,
					ActivationEligibilityEpoch: 2,
					ActivationHash:             make([]byte, 32),
					ExitHash:                   make([]byte, 32),
					WithdrawalOps:              make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_PENDING,
		},
		{
			name: "active ongoing",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch: 3,
					ExitEpoch:       farFutureEpoch,
					ActivationHash:  make([]byte, 32),
					ExitHash:        make([]byte, 32),
					WithdrawalOps:   make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_ACTIVE,
		},
		{
			name: "active slashed",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch: 3,
					ExitEpoch:       30,
					Slashed:         true,
					ActivationHash:  make([]byte, 32),
					ExitHash:        make([]byte, 32),
					WithdrawalOps:   make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_ACTIVE,
		},
		{
			name: "active exiting",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch: 3,
					ExitEpoch:       30,
					Slashed:         false,
					ActivationHash:  make([]byte, 32),
					ExitHash:        make([]byte, 32),
					WithdrawalOps:   make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_ACTIVE,
		},
		{
			name: "exited slashed",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					Slashed:           true,
					ActivationHash:    make([]byte, 32),
					ExitHash:          make([]byte, 32),
					WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(35),
			},
			want: ethpb.ValidatorStatus_EXITED,
		},
		{
			name: "exited unslashed",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					Slashed:           false,
					ActivationHash:    make([]byte, 32),
					ExitHash:          make([]byte, 32),
					WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(35),
			},
			want: ethpb.ValidatorStatus_EXITED,
		},
		{
			name: "withdrawal possible",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
					Slashed:           false,
					ActivationHash:    make([]byte, 32),
					ExitHash:          make([]byte, 32),
					WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(45),
			},
			want: ethpb.ValidatorStatus_WITHDRAWAL,
		},
		{
			name: "withdrawal done",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					EffectiveBalance:  0,
					Slashed:           false,
					ActivationHash:    make([]byte, 32),
					ExitHash:          make([]byte, 32),
					WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(45),
			},
			want: ethpb.ValidatorStatus_WITHDRAWAL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readOnlyVal, err := v1.NewValidator(migration.V1ValidatorToV1Alpha1(tt.args.validator))
			require.NoError(t, err)
			got, err := ValidatorStatus(readOnlyVal, tt.args.epoch)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("validatorStatus() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ValidatorSubStatus(t *testing.T) {
	farFutureEpoch := params.BeaconConfig().FarFutureEpoch

	type args struct {
		validator *ethpb.Validator
		epoch     types.Epoch
	}
	tests := []struct {
		name    string
		args    args
		want    ethpb.ValidatorStatus
		wantErr bool
	}{
		{
			name: "pending initialized",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:            farFutureEpoch,
					ActivationEligibilityEpoch: farFutureEpoch,
					ActivationHash:             make([]byte, 32),
					ExitHash:                   make([]byte, 32),
					WithdrawalOps:              make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_PENDING_INITIALIZED,
		},
		{
			name: "pending queued",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:            10,
					ActivationEligibilityEpoch: 2,
					ActivationHash:             make([]byte, 32),
					ExitHash:                   make([]byte, 32),
					WithdrawalOps:              make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_PENDING_QUEUED,
		},
		{
			name: "active ongoing",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch: 3,
					ExitEpoch:       farFutureEpoch,
					ActivationHash:  make([]byte, 32),
					ExitHash:        make([]byte, 32),
					WithdrawalOps:   make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_ACTIVE_ONGOING,
		},
		{
			name: "active slashed",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch: 3,
					ExitEpoch:       30,
					Slashed:         true,
					ActivationHash:  make([]byte, 32),
					ExitHash:        make([]byte, 32),
					WithdrawalOps:   make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_ACTIVE_SLASHED,
		},
		{
			name: "active exiting",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch: 3,
					ExitEpoch:       30,
					Slashed:         false,
					ActivationHash:  make([]byte, 32),
					ExitHash:        make([]byte, 32),
					WithdrawalOps:   make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(5),
			},
			want: ethpb.ValidatorStatus_ACTIVE_EXITING,
		},
		{
			name: "exited slashed",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					Slashed:           true,
					ActivationHash:    make([]byte, 32),
					ExitHash:          make([]byte, 32),
					WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(35),
			},
			want: ethpb.ValidatorStatus_EXITED_SLASHED,
		},
		{
			name: "exited unslashed",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					Slashed:           false,
					ActivationHash:    make([]byte, 32),
					ExitHash:          make([]byte, 32),
					WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(35),
			},
			want: ethpb.ValidatorStatus_EXITED_UNSLASHED,
		},
		{
			name: "withdrawal possible",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
					Slashed:           false,
					ActivationHash:    make([]byte, 32),
					ExitHash:          make([]byte, 32),
					WithdrawalOps:     make([]*ethpb.WithdrawalOp, 0),
				},
				epoch: types.Epoch(45),
			},
			want: ethpb.ValidatorStatus_WITHDRAWAL_POSSIBLE,
		},
		{
			name: "withdrawal done",
			args: args{
				validator: &ethpb.Validator{
					ActivationEpoch:   3,
					ExitEpoch:         30,
					WithdrawableEpoch: 40,
					EffectiveBalance:  0,
					Slashed:           false,
					ActivationHash:    (params.BeaconConfig().ZeroHash)[:],
					ExitHash:          (params.BeaconConfig().ZeroHash)[:],
					WithdrawalOps:     []*ethpb.WithdrawalOp{},
				},
				epoch: types.Epoch(45),
			},
			want: ethpb.ValidatorStatus_WITHDRAWAL_DONE,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readOnlyVal, err := v1.NewValidator(migration.V1ValidatorToV1Alpha1(tt.args.validator))
			require.NoError(t, err)
			got, err := ValidatorSubStatus(readOnlyVal, tt.args.epoch)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("validatorSubStatus() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// This test verifies how many validator statuses have meaningful values.
// The first expected non-meaningful value will have x.String() equal to its numeric representation.
// This test assumes we start numbering from 0 and do not skip any values.
// Having a test like this allows us to use e.g. `if value < 10` for validity checks.
func TestNumberOfStatuses(t *testing.T) {
	lastValidEnumValue := 12
	x := ethpb.ValidatorStatus(lastValidEnumValue)
	assert.NotEqual(t, strconv.Itoa(lastValidEnumValue), x.String())
	x = ethpb.ValidatorStatus(lastValidEnumValue + 1)
	assert.Equal(t, strconv.Itoa(lastValidEnumValue+1), x.String())
}

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

package withdrawals

import (
	"context"
	"reflect"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"google.golang.org/protobuf/proto"
)

func TestPool_InsertWithdrawal(t *testing.T) {
	type fields struct {
		pending []*ethpb.Withdrawal
	}
	type args struct {
		withdrawal *ethpb.Withdrawal
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []*ethpb.Withdrawal
	}{
		{
			name: "Prevent inserting nil withdrawal",
			fields: fields{
				pending: make([]*ethpb.Withdrawal, 0),
			},
			args: args{
				withdrawal: nil,
			},
			want: []*ethpb.Withdrawal{},
		},
		{
			name: "Prevent inserting malformed withdrawal",
			fields: fields{
				pending: make([]*ethpb.Withdrawal, 0),
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{},
			},
			want: []*ethpb.Withdrawal{},
		},
		{
			name: "Empty list",
			fields: fields{
				pending: make([]*ethpb.Withdrawal, 0),
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
			want: []*ethpb.Withdrawal{
				{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
		},
		{
			name: "Duplicate identical withdrawal",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{
						Epoch:          12,
						ValidatorIndex: 1,
						Amount:         45000,
						InitTxHash:     []byte{0, 1, 2},
					},
				},
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
			want: []*ethpb.Withdrawal{
				{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
		},
		{
			name: "Duplicate withdrawal in pending list",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{
						Epoch:          12,
						ValidatorIndex: 1,
						Amount:         45000,
						InitTxHash:     []byte{0, 1, 2},
					},
				},
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
			want: []*ethpb.Withdrawal{
				{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
		},
		{
			name: "Duplicate validator index",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{
						Epoch:          12,
						ValidatorIndex: 1,
						Amount:         45000,
						InitTxHash:     []byte{0, 1, 2},
					},
				},
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{
					Epoch:          20,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
			want: []*ethpb.Withdrawal{
				{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
		},
		{
			name: "Duplicate received with more favorable withdrawal epoch",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{
						Epoch:          12,
						ValidatorIndex: 1,
						Amount:         45000,
						InitTxHash:     []byte{0, 1, 2, 3},
					},
				},
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{
					Epoch:          4,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
			},
			want: []*ethpb.Withdrawal{
				{
					Epoch:          4,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2},
				},
				{
					Epoch:          12,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2, 3},
				},
			},
		},
		{
			name: "WithdrawalPool for already withdrawaled validator",
			fields: fields{
				pending: []*ethpb.Withdrawal{},
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{
					Epoch:          12,
					ValidatorIndex: 2,
				},
			},
			want: []*ethpb.Withdrawal{},
		},
		{
			name: "Maintains sorted order",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{
						Epoch:          12,
						ValidatorIndex: 0,
						Amount:         45000,
						InitTxHash:     []byte{0, 1, 2, 3},
					},
					{
						Epoch:          15,
						ValidatorIndex: 2,
						Amount:         45000,
						InitTxHash:     []byte{0, 1, 2, 4},
					},
				},
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{
					Epoch:          10,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2, 5},
				},
			},
			want: []*ethpb.Withdrawal{
				{
					Epoch:          10,
					ValidatorIndex: 1,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2, 5},
				},
				{
					Epoch:          12,
					ValidatorIndex: 0,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2, 3},
				},
				{
					Epoch:          15,
					ValidatorIndex: 2,
					Amount:         45000,
					InitTxHash:     []byte{0, 1, 2, 4},
				},
			},
		},
	}
	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pool{
				pending: tt.fields.pending,
			}
			p.InsertWithdrawal(ctx, tt.args.withdrawal)
			if len(p.pending) != len(tt.want) {
				t.Fatalf("Mismatched lengths of pending list. Got %d, wanted %d.", len(p.pending), len(tt.want))
			}
			for i := range p.pending {
				if !proto.Equal(p.pending[i], tt.want[i]) {
					t.Errorf("Pending withdrawal at index %d does not match expected. Got=%v wanted=%v", i, p.pending[i], tt.want[i])
				}
			}
		})
	}
}

func TestPool_MarkIncluded(t *testing.T) {
	type fields struct {
		pending []*ethpb.Withdrawal
	}
	type args struct {
		withdrawal *ethpb.Withdrawal
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   fields
	}{
		{
			name: "Removes from pending list",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{InitTxHash: []byte{1}},
					{InitTxHash: []byte{2}},
					{InitTxHash: []byte{3}},
				},
			},
			args: args{
				withdrawal: &ethpb.Withdrawal{InitTxHash: []byte{2}},
			},
			want: fields{
				pending: []*ethpb.Withdrawal{{InitTxHash: []byte{1}}, {InitTxHash: []byte{3}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pool{
				pending: tt.fields.pending,
			}
			p.MarkIncluded(tt.args.withdrawal)
			if len(p.pending) != len(tt.want.pending) {
				t.Fatalf("Mismatched lengths of pending list. Got %d, wanted %d.", len(p.pending), len(tt.want.pending))
			}
			for i := range p.pending {
				if !proto.Equal(p.pending[i], tt.want.pending[i]) {
					t.Errorf("Pending withdrawal at index %d does not match expected. Got=%v wanted=%v", i, p.pending[i], tt.want.pending[i])
				}
			}
		})
	}
}

func TestPool_PendingWithdrawals(t *testing.T) {
	type fields struct {
		pending []*ethpb.Withdrawal
		noLimit bool
	}
	type args struct {
		slot types.Slot
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []*ethpb.Withdrawal
	}{
		{
			name: "Empty list",
			fields: fields{
				pending: []*ethpb.Withdrawal{},
			},
			args: args{
				slot: 100000,
			},
			want: []*ethpb.Withdrawal{},
		},
		{
			name: "All eligible",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{Epoch: 0},
					{Epoch: 1},
					{Epoch: 2},
					{Epoch: 3},
					{Epoch: 4},
				},
			},
			args: args{
				slot: 1000000,
			},
			want: []*ethpb.Withdrawal{
				{Epoch: 0},
				{Epoch: 1},
				{Epoch: 2},
				{Epoch: 3},
				{Epoch: 4},
			},
		},
		{
			name: "All eligible, above max",
			fields: fields{
				noLimit: true,
				pending: []*ethpb.Withdrawal{
					{Epoch: 0},
					{Epoch: 1},
					{Epoch: 2},
					{Epoch: 3},
					{Epoch: 4},
					{Epoch: 5},
					{Epoch: 6},
					{Epoch: 7},
					{Epoch: 8},
					{Epoch: 9},
					{Epoch: 10},
					{Epoch: 11},
					{Epoch: 12},
					{Epoch: 13},
					{Epoch: 14},
					{Epoch: 15},
					{Epoch: 16},
					{Epoch: 17},
					{Epoch: 18},
					{Epoch: 19},
				},
			},
			args: args{
				slot: 1000000,
			},
			want: []*ethpb.Withdrawal{
				{Epoch: 0},
				{Epoch: 1},
				{Epoch: 2},
				{Epoch: 3},
				{Epoch: 4},
				{Epoch: 5},
				{Epoch: 6},
				{Epoch: 7},
				{Epoch: 8},
				{Epoch: 9},
				{Epoch: 10},
				{Epoch: 11},
				{Epoch: 12},
				{Epoch: 13},
				{Epoch: 14},
				{Epoch: 15},
				{Epoch: 16},
				{Epoch: 17},
				{Epoch: 18},
				{Epoch: 19},
			},
		},
		{
			name: "Some eligible",
			fields: fields{
				pending: []*ethpb.Withdrawal{
					{Epoch: 0},
					{Epoch: 3},
					{Epoch: 4},
					{Epoch: 2},
					{Epoch: 1},
				},
			},
			args: args{
				slot: 2 * params.BeaconConfig().SlotsPerEpoch,
			},
			want: []*ethpb.Withdrawal{
				{Epoch: 0},
				{Epoch: 2},
				{Epoch: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pool{
				pending: tt.fields.pending,
			}
			s, err := v1.InitializeFromProtoUnsafe(&ethpb.BeaconState{Validators: []*ethpb.Validator{{WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch}}})
			require.NoError(t, err)
			if got := p.PendingWithdrawals(tt.args.slot, s, tt.fields.noLimit); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PendingWithdrawals() = %v, want %v", got, tt.want)
			}
		})
	}
}

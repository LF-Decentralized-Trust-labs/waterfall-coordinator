package sync

import (
	"reflect"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain"
	mock "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestExtractBlockDataType(t *testing.T) {
	// Precompute digests
	genDigest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	altairDigest, err := signing.ComputeForkDigest(params.BeaconConfig().AltairForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	bellatrixDigest, err := signing.ComputeForkDigest(params.BeaconConfig().BellatrixForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)

	type args struct {
		digest []byte
		chain  blockchain.ChainInfoFetcher
	}
	tests := []struct {
		name    string
		args    args
		want    block.SignedBeaconBlock
		wantErr bool
	}{
		{
			name: "no digest",
			args: args{
				digest: []byte{},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},

			want: func() block.SignedBeaconBlock {
				wsb, err := wrapper.WrappedSignedBeaconBlock(&ethpb.SignedBeaconBlock{})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
		{
			name: "invalid digest",
			args: args{
				digest: []byte{0x00, 0x01},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "non existent digest",
			args: args{
				digest: []byte{0x00, 0x01, 0x02, 0x03},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "genesis fork version",
			args: args{
				digest: genDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want: func() block.SignedBeaconBlock {
				wsb, err := wrapper.WrappedSignedBeaconBlock(&ethpb.SignedBeaconBlock{})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
		{
			name: "altair fork version",
			args: args{
				digest: altairDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want: func() block.SignedBeaconBlock {
				wsb, err := wrapper.WrappedSignedBeaconBlock(&ethpb.SignedBeaconBlockAltair{Block: &ethpb.BeaconBlockAltair{}})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
		{
			name: "bellatrix fork version",
			args: args{
				digest: bellatrixDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want: func() block.SignedBeaconBlock {
				wsb, err := wrapper.WrappedSignedBeaconBlock(&ethpb.SignedBeaconBlockBellatrix{Block: &ethpb.BeaconBlockBellatrix{}})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractBlockDataType(tt.args.digest, tt.args.chain)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractBlockDataType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractBlockDataType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

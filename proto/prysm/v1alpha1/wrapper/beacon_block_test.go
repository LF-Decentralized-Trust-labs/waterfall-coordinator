package wrapper_test

import (
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestWrappedSignedBeaconBlock(t *testing.T) {
	tests := []struct {
		name    string
		blk     interface{}
		wantErr bool
	}{
		{
			name:    "unsupported type",
			blk:     "not a beacon block",
			wantErr: true,
		},
		{
			name: "phase0",
			blk:  util.NewBeaconBlock(),
		},
		{
			name: "altair",
			blk:  util.NewBeaconBlockAltair(),
		},
		{
			name: "bellatrix",
			blk:  util.NewBeaconBlockBellatrix(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := wrapper.WrappedSignedBeaconBlock(tt.blk)
			if tt.wantErr {
				require.ErrorIs(t, err, wrapper.ErrUnsupportedSignedBeaconBlock)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

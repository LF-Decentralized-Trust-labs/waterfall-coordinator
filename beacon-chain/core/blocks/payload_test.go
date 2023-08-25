package blocks_test

import (
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/blocks"
	fieldparams "gitlab.waterfall.network/waterfall/protocol/coordinator/config/fieldparams"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func Test_IsMergeComplete(t *testing.T) {
	tests := []struct {
		name    string
		payload *ethpb.ExecutionPayloadHeader
		want    bool
	}{
		{
			name:    "empty payload header",
			payload: emptyPayloadHeader(),
			want:    false,
		},
		{
			name: "has parent hash",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.ParentHash = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has fee recipient",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.FeeRecipient = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has state root",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.StateRoot = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has receipt root",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.ReceiptRoot = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has logs bloom",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.LogsBloom = bytesutil.PadTo([]byte{'a'}, fieldparams.LogsBloomLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has random",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.PrevRandao = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has base fee",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.BaseFeePerGas = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has block hash",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.BlockHash = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has tx root",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.TransactionsRoot = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has extra data",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.ExtraData = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "has block number",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.BlockNumber = 1
				return h
			}(),
			want: true,
		},
		{
			name: "has gas limit",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.GasLimit = 1
				return h
			}(),
			want: true,
		},
		{
			name: "has gas used",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.GasUsed = 1
				return h
			}(),
			want: true,
		},
		{
			name: "has time stamp",
			payload: func() *ethpb.ExecutionPayloadHeader {
				h := emptyPayloadHeader()
				h.Timestamp = 1
				return h
			}(),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
			require.NoError(t, st.SetLatestExecutionPayloadHeader(tt.payload))
			got, err := blocks.IsMergeTransitionComplete(st)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("mergeComplete() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkBellatrixComplete(b *testing.B) {
	st, _ := util.DeterministicGenesisStateBellatrix(b, 1)
	require.NoError(b, st.SetLatestExecutionPayloadHeader(emptyPayloadHeader()))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := blocks.IsMergeTransitionComplete(st)
		require.NoError(b, err)
	}
}

func emptyPayloadHeader() *ethpb.ExecutionPayloadHeader {
	return &ethpb.ExecutionPayloadHeader{
		ParentHash:       make([]byte, fieldparams.RootLength),
		FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:        make([]byte, fieldparams.RootLength),
		ReceiptRoot:      make([]byte, fieldparams.RootLength),
		LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:       make([]byte, fieldparams.RootLength),
		BaseFeePerGas:    make([]byte, fieldparams.RootLength),
		BlockHash:        make([]byte, fieldparams.RootLength),
		TransactionsRoot: make([]byte, fieldparams.RootLength),
		ExtraData:        make([]byte, 0),
	}
}

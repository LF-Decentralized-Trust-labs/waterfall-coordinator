package types

import (
	"math/big"

	"github.com/pkg/errors"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common"
	gethTypes "gitlab.waterfall.network/waterfall/protocol/gwat/core/types"
)

// HeaderInfo specifies the block header information in the ETH 1.0 chain.
type HeaderInfo struct {
	Number *big.Int
	Hash   common.Hash
	Time   uint64
	Slot   uint64
}

// HeaderToHeaderInfo converts an eth1 header to a header metadata type.
func HeaderToHeaderInfo(hdr *gethTypes.Header) (*HeaderInfo, error) {
	if hdr.Nr() == 0 && hdr.Height != 0 {
		return nil, errors.Errorf("Not finalized block hash=%s height=%d", hdr.Hash().Hex(), hdr.Height)
	}

	return &HeaderInfo{
		Hash:   hdr.Hash(),
		Number: new(big.Int).SetUint64(hdr.Nr()),
		Time:   hdr.Time,
		Slot:   hdr.Slot,
	}, nil
}

// Copy sends out a copy of the current header info.
func (h *HeaderInfo) Copy() *HeaderInfo {
	return &HeaderInfo{
		Hash:   bytesutil.ToBytes32(h.Hash[:]),
		Number: new(big.Int).Set(h.Number),
		Time:   h.Time,
		Slot:   h.Slot,
	}
}

package sync

import (
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p/encoder"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p/types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/network/forks"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
)

// chunkBlockWriter writes the given message as a chunked response to the given network
// stream.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func (s *Service) chunkBlockWriter(stream libp2pcore.Stream, blk block.SignedBeaconBlock) error {
	SetStreamWriteDeadline(stream, defaultWriteDuration)
	return WriteBlockChunk(stream, s.cfg.chain, s.cfg.p2p.Encoding(), blk)
}

// WriteBlockChunk writes block chunk object to stream.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func WriteBlockChunk(stream libp2pcore.Stream, chain blockchain.ChainInfoFetcher, encoding encoder.NetworkEncoding, blk block.SignedBeaconBlock) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}
	var obtainedCtx []byte
	switch blk.Version() {
	case version.Phase0:
		valRoot := chain.GenesisValidatorsRoot()
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().GenesisEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	case version.Altair:
		valRoot := chain.GenesisValidatorsRoot()
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().AltairForkEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	case version.Bellatrix:
		valRoot := chain.GenesisValidatorsRoot()
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().BellatrixForkEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	}

	if err := writeContextToStream(obtainedCtx, stream, chain); err != nil {
		return err
	}
	_, err := encoding.EncodeWithMaxLength(stream, blk)
	return err
}

// ReadChunkedBlock handles each response chunk that is sent by the
// peer and converts it into a beacon block.
func ReadChunkedBlock(stream libp2pcore.Stream, chain blockchain.ChainInfoFetcher, p2p p2p.P2P, isFirstChunk bool) (block.SignedBeaconBlock, error) {
	// Handle deadlines differently for first chunk
	if isFirstChunk {
		return readFirstChunkedBlock(stream, chain, p2p)
	}

	return readResponseChunk(stream, chain, p2p)
}

// readFirstChunkedBlock reads the first chunked block and applies the appropriate deadlines to
// it.
func readFirstChunkedBlock(stream libp2pcore.Stream, chain blockchain.ChainInfoFetcher, p2p p2p.P2P) (block.SignedBeaconBlock, error) {
	code, errMsg, err := ReadStatusCode(stream, p2p.Encoding())
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}
	rpcCtx, err := readContextFromStream(stream, chain)
	if err != nil {
		return nil, err
	}
	blk, err := extractBlockDataType(rpcCtx, chain)
	if err != nil {
		return nil, err
	}
	err = p2p.Encoding().DecodeWithMaxLength(stream, blk)
	return blk, err
}

// readResponseChunk reads the response from the stream and decodes it into the
// provided message type.
func readResponseChunk(stream libp2pcore.Stream, chain blockchain.ChainInfoFetcher, p2p p2p.P2P) (block.SignedBeaconBlock, error) {
	SetStreamReadDeadline(stream, respTimeout)
	code, errMsg, err := readStatusCodeNoDeadline(stream, p2p.Encoding())
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}
	// No-op for now with the rpc context.
	rpcCtx, err := readContextFromStream(stream, chain)
	if err != nil {
		return nil, err
	}
	blk, err := extractBlockDataType(rpcCtx, chain)
	if err != nil {
		return nil, err
	}
	err = p2p.Encoding().DecodeWithMaxLength(stream, blk)
	return blk, err
}

func extractBlockDataType(digest []byte, chain blockchain.ChainInfoFetcher) (block.SignedBeaconBlock, error) {
	if len(digest) == 0 {
		bFunc, ok := types.BlockMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
		if !ok {
			return nil, errors.New("no block type exists for the genesis fork version.")
		}
		return bFunc()
	}
	if len(digest) != forkDigestLength {
		return nil, errors.Errorf("invalid digest returned, wanted a length of %d but received %d", forkDigestLength, len(digest))
	}
	vRoot := chain.GenesisValidatorsRoot()
	for k, blkFunc := range types.BlockMap {
		rDigest, err := signing.ComputeForkDigest(k[:], vRoot[:])
		if err != nil {
			return nil, err
		}
		if rDigest == bytesutil.ToBytes4(digest) {
			return blkFunc()
		}
	}
	return nil, errors.New("no valid digest matched")
}

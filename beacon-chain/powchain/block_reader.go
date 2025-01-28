package powchain

import (
	"context"
	"fmt"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/tracing"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"go.opencensus.io/trace"
)

// BlockExists returns true if the block exists, its height and any possible error encountered.
func (s *Service) BlockExists(ctx context.Context, hash common.Hash) (bool, *big.Int, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.web3service.BlockExists")
	defer span.End()

	if exists, hdrInfo, err := s.headerCache.HeaderInfoByHash(hash); exists || err != nil {
		if err != nil {
			return false, nil, err
		}
		if hdrInfo.Number.Uint64() > 0 {
			span.AddAttributes(trace.BoolAttribute("blockCacheHit", true))
			return true, hdrInfo.Number, nil
		}
	}
	span.AddAttributes(trace.BoolAttribute("blockCacheHit", false))
	header, err := s.eth1DataFetcher.HeaderByHash(ctx, hash)
	if err != nil {
		return false, big.NewInt(0), errors.Wrap(err, "could not query block with given hash")
	}

	if err := s.headerCache.AddHeader(header); err != nil {
		return false, big.NewInt(0), err
	}

	return true, new(big.Int).SetUint64(header.Nr()), nil
}

// BlockExistsWithCache returns true if the block exists in cache, its height and any possible error encountered.
func (s *Service) BlockExistsWithCache(ctx context.Context, hash common.Hash) (bool, *big.Int, error) {
	_, span := trace.StartSpan(ctx, "beacon-chain.web3service.BlockExistsWithCache")
	defer span.End()
	if exists, hdrInfo, err := s.headerCache.HeaderInfoByHash(hash); exists || err != nil {
		if err != nil {
			return false, nil, err
		}
		span.AddAttributes(trace.BoolAttribute("blockCacheHit", true))
		return true, hdrInfo.Number, nil
	}
	span.AddAttributes(trace.BoolAttribute("blockCacheHit", false))
	return false, nil, nil
}

// BlockHashByHeight returns the block hash of the block at the given height.
func (s *Service) BlockHashByHeight(ctx context.Context, height *big.Int) (common.Hash, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.web3service.BlockHashByHeight")
	defer span.End()

	if exists, hInfo, err := s.headerCache.HeaderInfoByHeight(height); exists || err != nil {
		if err != nil {
			return [32]byte{}, err
		}
		span.AddAttributes(trace.BoolAttribute("headerCacheHit", true))
		return hInfo.Hash, nil
	}
	span.AddAttributes(trace.BoolAttribute("headerCacheHit", false))

	if s.eth1DataFetcher == nil {
		err := errors.New("nil eth1DataFetcher")
		tracing.AnnotateError(span, err)
		return [32]byte{}, err
	}

	header, err := s.eth1DataFetcher.HeaderByNumber(ctx, height)
	if err != nil {
		if height == nil {
			return [32]byte{}, errors.Wrap(err, "could not query last finalized header")
		}
		return [32]byte{}, errors.Wrap(err, fmt.Sprintf("could not query header with nr=%d", height.Uint64()))
	}
	if header == nil {
		log.Errorf("could not query header with height %d", height.Uint64())
		return [32]byte{}, errors.Wrap(err, fmt.Sprintf("could not query header with nr=%d", height.Uint64()))
	}
	if header.Nr() == 0 && header.Height != 0 {
		log.WithFields(logrus.Fields{
			"header.Nr":     header.Nr(),
			"header.Height": header.Height,
			"blockHash":     fmt.Sprintf("%#x", s.latestEth1Data.BlockHash),
		}).Error("Latest shard1 block is not finalized")
		return [32]byte{}, errors.Wrap(err, fmt.Sprintf("could not query header with height %d", height.Uint64()))
	}
	if err := s.headerCache.AddHeader(header); err != nil {
		return [32]byte{}, err
	}
	return header.Hash(), nil
}

// BlockSlotByHeight returns the block hash of the block at the given height.
func (s *Service) BlockSlotByHeight(ctx context.Context, height *big.Int) (uint64, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.web3service.BlockSlotByHeight")
	defer span.End()

	if exists, hInfo, err := s.headerCache.HeaderInfoByHeight(height); exists || err != nil {
		if err != nil {
			return 0, err
		}
		span.AddAttributes(trace.BoolAttribute("headerCacheHit", true))
		return hInfo.Slot, nil
	}
	span.AddAttributes(trace.BoolAttribute("headerCacheHit", false))

	if s.eth1DataFetcher == nil {
		err := errors.New("nil eth1DataFetcher")
		tracing.AnnotateError(span, err)
		return 0, err
	}

	header, err := s.eth1DataFetcher.HeaderByNumber(ctx, height)
	if err != nil {
		if height == nil {
			return 0, errors.Wrap(err, "could not query last finalized header")
		}
		return 0, errors.Wrap(err, fmt.Sprintf("could not query header with nr=%d", height.Uint64()))
	}
	if header == nil {
		log.Errorf("could not query header with height %d", height.Uint64())
		return 0, errors.Wrap(err, fmt.Sprintf("could not query header with nr=%d", height.Uint64()))
	}
	if header.Nr() == 0 && header.Height != 0 {
		log.WithFields(logrus.Fields{
			"header.Nr":     header.Nr(),
			"header.Height": header.Height,
			"blockHash":     fmt.Sprintf("%#x", s.latestEth1Data.BlockHash),
		}).Error("Latest shard1 block is not finalized")
		return 0, errors.Wrap(err, fmt.Sprintf("could not query header with height %d", height.Uint64()))
	}
	if err := s.headerCache.AddHeader(header); err != nil {
		return 0, err
	}
	return header.Slot, nil
}

// BlockTimeByHeight fetches an eth1.0 block timestamp by its height.
func (s *Service) BlockTimeByHeight(ctx context.Context, height *big.Int) (uint64, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.web3service.BlockTimeByHeight")
	defer span.End()
	if s.eth1DataFetcher == nil {
		err := errors.New("nil eth1DataFetcher")
		tracing.AnnotateError(span, err)
		return 0, err
	}

	header, err := s.eth1DataFetcher.HeaderByNumber(ctx, height)
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("could not query block with height %d", height.Uint64()))
	}
	return header.Time, nil
}

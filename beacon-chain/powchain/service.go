// Package powchain defines a runtime service which is tasked with
// communicating with an eth1 endpoint, processing logs from a deposit
// contract, and the latest eth1 data headers for usage in the beacon node.
package powchain

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"reflect"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ethTypes "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache/depositcache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed"
	statefeed "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/transition"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/voluntaryexits"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/withdrawals"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	nativev1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/state-native/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/stategen"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/features"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/container/trie"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/monitoring/clientstats"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/network"
	ethpbv1 "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/eth/v1"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	prysmTime "gitlab.waterfall.network/waterfall/protocol/coordinator/time"
	ethereum "gitlab.waterfall.network/waterfall/protocol/gwat"
	"gitlab.waterfall.network/waterfall/protocol/gwat/accounts/abi/bind"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common/hexutil"
	gwatTypes "gitlab.waterfall.network/waterfall/protocol/gwat/core/types"
	"gitlab.waterfall.network/waterfall/protocol/gwat/ethclient"
	gethRPC "gitlab.waterfall.network/waterfall/protocol/gwat/rpc"
)

var (
	validDepositsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "powchain_valid_deposits_received",
		Help: "The number of valid deposits received in the deposit contract",
	})
	blockNumberGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "powchain_block_number",
		Help: "The current block number in the proof-of-work chain",
	})
	missedDepositLogsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "powchain_missed_deposit_logs",
		Help: "The number of times a missed deposit log is detected",
	})
)

var (
	// time to wait before trying to reconnect with the eth1 node.
	backOffPeriod = 15 * time.Second
	// amount of times before we log the status of the eth1 dial attempt.
	logThreshold = 8
	// period to log chainstart related information
	logPeriod = 1 * time.Minute
	// threshold of how old we will accept an eth1 node's head to be.
	eth1Threshold = 20 * time.Minute
)

// ChainStartFetcher retrieves information pertaining to the chain start event
// of the beacon chain for usage across various services.
type ChainStartFetcher interface {
	ChainStartEth1Data() *ethpb.Eth1Data
	PreGenesisState() state.BeaconState
	ClearPreGenesisData()
}

// ChainInfoFetcher retrieves information about eth1 metadata at the Ethereum consensus genesis time.
type ChainInfoFetcher interface {
	Eth2GenesisPowchainInfo() (uint64, *big.Int)
	IsConnectedToETH1() bool
	CurrentETH1Endpoint() string
	CurrentETH1ConnectionError() error
	ETH1Endpoints() []string
	ETH1ConnectionErrors() []error
}

// POWBlockFetcher defines a struct that can retrieve mainchain blocks.
type POWBlockFetcher interface {
	BlockTimeByHeight(ctx context.Context, height *big.Int) (uint64, error)
	BlockHashByHeight(ctx context.Context, height *big.Int) (gwatCommon.Hash, error)
	BlockExists(ctx context.Context, hash gwatCommon.Hash) (bool, *big.Int, error)
	BlockExistsWithCache(ctx context.Context, hash gwatCommon.Hash) (bool, *big.Int, error)

	ExecutionDagGetOptimisticSpines(ctx context.Context, fromSpine gwatCommon.Hash) ([]gwatCommon.HashArray, error)
	ExecutionDagGetCandidates(ctx context.Context, slot ethTypes.Slot) (gwatCommon.HashArray, error)
	ExecutionDagFinalize(ctx context.Context, finParams *gwatTypes.FinalizationParams) (*gwatTypes.FinalizationResult, error)
	ExecutionDagCoordinatedState(ctx context.Context) (*gwatTypes.FinalizationResult, error)

	IsTxLogValid() bool
}

// Chain defines a standard interface for the powchain service in Prysm.
type Chain interface {
	ChainStartFetcher
	ChainInfoFetcher
	POWBlockFetcher
}

// RPCDataFetcher defines a subset of methods conformed to by ETH1.0 RPC clients for
// fetching eth1 data from the clients.
type RPCDataFetcher interface {
	Close()
	HeaderByNumber(ctx context.Context, number *big.Int) (*gwatTypes.Header, error)
	HeaderByHash(ctx context.Context, hash gwatCommon.Hash) (*gwatTypes.Header, error)
	SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error)
}

// RPCClient defines the rpc methods required to interact with the eth1 node.
type RPCClient interface {
	Close()
	BatchCall(b []gethRPC.BatchElem) error
	CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error
}

// config defines a config struct for dependencies into the service.
type config struct {
	depositContractAddr     gwatCommon.Address
	beaconDB                db.HeadAccessDatabase
	depositCache            *depositcache.DepositCache
	exitPool                voluntaryexits.PoolManager
	withdrawalPool          withdrawals.PoolManager
	stateNotifier           statefeed.Notifier
	stateGen                *stategen.State
	eth1HeaderReqLimit      uint64
	beaconNodeStatsUpdater  BeaconNodeStatsUpdater
	httpEndpoints           []network.Endpoint
	currHttpEndpoint        network.Endpoint
	finalizedStateAtStartup state.BeaconState
}

// Service fetches important information about the canonical
// Ethereum ETH1.0 chain via a web3 endpoint using an ethclient. The Random
// Beacon Chain requires synchronization with the ETH1.0 chain's current
// blockhash, block number, and access to logs within the
// Validator Registration Contract on the ETH1.0 chain to kick off the beacon
// chain's validator registration process.
type Service struct {
	connectedETH1           bool
	isRunning               bool
	pollConnActive          bool //if reconnecting to gwat
	processingLock          sync.RWMutex
	cfg                     *config
	ctx                     context.Context
	cancel                  context.CancelFunc
	headTicker              *time.Ticker
	httpLogger              bind.ContractFilterer
	eth1DataFetcher         RPCDataFetcher
	rpcClient               RPCClient
	headerCache             *headerCache // cache to store block hash/block height.
	latestEth1Data          *ethpb.LatestETH1Data
	depositTrie             *trie.SparseMerkleTrie
	chainStartData          *ethpb.ChainStartData
	lastReceivedMerkleIndex int64 // Keeps track of the last received index to prevent log spam.
	runError                error
	preGenesisState         state.BeaconState
	//tracking handled beacon state
	lastHandledBlock [32]byte          // last handled beacon block root
	lastHandledSlot  ethTypes.Slot     // last handled beacon block slot
	lastHandledState state.BeaconState // last handled beacon state
}

// NewService sets up a new instance with an ethclient when given a web3 endpoint as a string in the config.
func NewService(ctx context.Context, opts ...Option) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel // govet fix for lost cancel. Cancel is handled in service.Stop()
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "could not setup deposit trie")
	}
	genState, err := transition.EmptyGenesisState()
	if err != nil {
		return nil, errors.Wrap(err, "could not setup genesis state")
	}

	s := &Service{
		ctx:    ctx,
		cancel: cancel,
		cfg: &config{
			beaconNodeStatsUpdater: &NopBeaconNodeStatsUpdater{},
			eth1HeaderReqLimit:     defaultEth1HeaderReqLimit,
		},
		latestEth1Data: &ethpb.LatestETH1Data{
			BlockHeight:        0,
			BlockTime:          0,
			BlockHash:          []byte{},
			LastRequestedBlock: 0,
		},
		headerCache: newHeaderCache(),
		depositTrie: depositTrie,
		chainStartData: &ethpb.ChainStartData{
			Eth1Data:           &ethpb.Eth1Data{},
			ChainstartDeposits: make([]*ethpb.Deposit, 0),
		},
		lastReceivedMerkleIndex: -1,
		preGenesisState:         genState,
		//headTicker:              time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerETH1Block) * time.Second),
		headTicker: time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	if err := s.ensureValidPowchainData(ctx); err != nil {
		return nil, errors.Wrap(err, "unable to validate powchain data")
	}

	eth1Data, err := s.cfg.beaconDB.PowchainData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve shard1 data")
	}

	if err := s.initializeEth1Data(ctx, eth1Data); err != nil {
		return nil, err
	}

	//// load saved withdrawal pool
	//witdrawalPoolData, err := s.cfg.beaconDB.ReadWithdrawalPool(ctx)
	//if err != nil {
	//	return nil, errors.Wrap(err, "unable to retrieve withdrawal pool data")
	//}
	//for _, op := range witdrawalPoolData {
	//	log.WithFields(logrus.Fields{
	//		" InitTxHash":     fmt.Sprintf("%#x", op.InitTxHash),
	//		"Epoch":           fmt.Sprintf("%d", op.Epoch),
	//		"Amount":          fmt.Sprintf("%d", op.Amount),
	//		" ValidatorIndex": fmt.Sprintf("%d", op.ValidatorIndex),
	//	}).Info("Witdrawal pool: load saved op")
	//	s.cfg.withdrawalPool.InsertWithdrawal(ctx, op)
	//}

	//// load saved exit pool
	//exitPoolData, err := s.cfg.beaconDB.ReadExitPool(ctx)
	//if err != nil {
	//	return nil, errors.Wrap(err, "unable to retrieve exit pool data")
	//}
	//for _, op := range exitPoolData {
	//	log.WithFields(logrus.Fields{
	//		" InitTxHash":     fmt.Sprintf("%#x", op.InitTxHash),
	//		"Epoch":           fmt.Sprintf("%d", op.Epoch),
	//		" ValidatorIndex": fmt.Sprintf("%d", op.ValidatorIndex),
	//	}).Info("Exit pool: load saved op")
	//	s.cfg.exitPool.InsertVoluntaryExitByGwat(ctx, op)
	//}

	go s.StateTracker()

	return s, nil
}

// Start a web3 service's main event loop.
func (s *Service) StateTracker() {
	chainEvtCh := make(chan *feed.Event, 0)
	sub := s.cfg.stateNotifier.StateFeed().Subscribe(chainEvtCh)
	defer sub.Unsubscribe()

	isBadDepositRoot := false

	for {
		select {
		case <-s.ctx.Done():
			log.Info("Context closed, exiting goroutine")
			return
		//case err := <-sub.Err():
		//	log.WithError(err).Warn("Exit by error")
		//	return
		case ev := <-chainEvtCh:
			//work while syncing only, check condition
			if ev.Type == statefeed.BlockProcessed {
				data, ok := ev.Data.(*statefeed.BlockProcessedData)
				if !ok {
					continue
				}

				log.WithFields(logrus.Fields{
					" isInitialSync":            data.InitialSync,
					"s.lastHandledSlot":         s.lastHandledSlot,
					"s.lastReceivedMerkleIndex": s.lastReceivedMerkleIndex,
					"isBadDepositRoot":          isBadDepositRoot,
					"isChainstarted":            s.chainStartData.Chainstarted,
				}).Info("=== LogProcessing: StateTracker: EvtBlockProcessed: 000")

				if err := s.mainSyncTxLogsByBlockEvt(data, isBadDepositRoot); err != nil {
					if errors.Is(err, errInvalidDepositRoot) {
						isBadDepositRoot = true
					}
					log.WithError(err).WithFields(logrus.Fields{
						"errInvalidDepositRoot": errors.Is(err, errInvalidDepositRoot),
						"isBadDepositRoot":      isBadDepositRoot,
					}).Error("=== LogProcessing: StateTracker: EvtBlockProcessed: error 1")
					continue
				}
				if err := s.headSyncTxLogsByBlockEvt(data, isBadDepositRoot); err != nil {
					if errors.Is(err, errInvalidDepositRoot) {
						isBadDepositRoot = true
					}
					log.WithError(err).WithFields(logrus.Fields{
						"errInvalidDepositRoot": errors.Is(err, errInvalidDepositRoot),
						"isBadDepositRoot":      isBadDepositRoot,
					}).Error("=== LogProcessing: StateTracker: EvtBlockProcessed: error 2")
					continue
				}
			}

			// first event when last finalized checkpoint reached
			if ev.Type == statefeed.FinalizedCheckpoint {
				data, ok := ev.Data.(*ethpbv1.EventFinalizedCheckpoint)
				if !ok {
					continue
				}

				log.WithFields(logrus.Fields{
					"s.lastHandledSlot":         s.lastHandledSlot,
					"data.FinalizationSlot":     data.FinalizationSlot,
					"s.lastReceivedMerkleIndex": s.lastReceivedMerkleIndex,
					"IsDelegate":                params.BeaconConfig().IsDelegatingStakeSlot(s.lastHandledSlot),
					"Chainstarted":              s.chainStartData.Chainstarted,
				}).Info("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint: 0000")

				if err := s.handlerFinalizedCheckpointEvt(data); err != nil {
					if errors.Is(err, errInvalidDepositRoot) {
						isBadDepositRoot = true
					}
					s.lastHandledState = nil
					log.WithError(err).WithFields(logrus.Fields{
						"errInvalidDepositRoot": errors.Is(err, errInvalidDepositRoot),
						"isBadDepositRoot":      isBadDepositRoot,
					}).Error("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint: error")
					continue
				}
			}
		}
	}
}

func (s *Service) IsTxLogValid() bool {
	return s.lastHandledState != nil
}

func (s *Service) mainSyncTxLogsByBlockEvt(data *statefeed.BlockProcessedData, isBadDepositRoot bool) error {
	if !data.InitialSync {
		return nil
	}
	if isBadDepositRoot {
		return nil
	}
	s.lastHandledSlot = data.Slot
	s.lastHandledBlock = data.BlockRoot
	depLen := len(data.SignedBlock.Block().Body().Deposits())
	if depLen == 0 {
		return nil
	}
	st, err := s.cfg.stateGen.StateByRoot(s.ctx, data.BlockRoot)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			" slot":    data.Slot,
			"deposits": len(data.SignedBlock.Block().Body().Deposits()),
			"block":    fmt.Sprintf("%#x", data.BlockRoot),
		}).Error("=== LogProcessing: StateTracker: main sync: get state failed")
		return nil
	}

	log.WithFields(logrus.Fields{
		" slot":        data.Slot,
		"deposits":     len(data.SignedBlock.Block().Body().Deposits()),
		"depositIndex": st.Eth1DepositIndex(),
		"depositCount": st.Eth1Data().DepositCount,
		"block":        fmt.Sprintf("%#x", data.BlockRoot),
	}).Info("=== LogProcessing: StateTracker: main sync: run")

	baseDepIndex := st.Eth1Data().DepositCount - uint64(depLen)
	if s.lastReceivedMerkleIndex+1 < new(big.Int).SetUint64(baseDepIndex).Int64() {
		handledCount, err := s.handleFinalizedDeposits(bytesutil.ToBytes32(data.SignedBlock.Block().ParentRoot()))
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				" slot":        data.Slot,
				"deposits":     len(data.SignedBlock.Block().Body().Deposits()),
				"block":        fmt.Sprintf("%#x", data.BlockRoot),
				"handledCount": handledCount,
			}).Error("=== LogProcessing: StateTracker: main sync: rebuild merkle trie failed")
			return fmt.Errorf("%w: %w", errInvalidDepositRoot, err)
		}
	}

	for i, deposit := range data.SignedBlock.Block().Body().Deposits() {
		err = s.ProcessDepositBlock(deposit, baseDepIndex+uint64(i))
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				" slot":    data.Slot,
				"deposits": len(data.SignedBlock.Block().Body().Deposits()),
				"block":    fmt.Sprintf("%#x", data.BlockRoot),
			}).Error("=== LogProcessing: StateTracker: main sync: process deposit failed")
			return fmt.Errorf("%w: %w", errInvalidDepositRoot, err)
		}
	}

	baseSpine := helpers.GetTerminalFinalizedSpine(st)
	log.WithFields(logrus.Fields{
		" slot":     data.Slot,
		"baseSpine": fmt.Sprintf("%#x", baseSpine),
		"deposits":  len(data.SignedBlock.Block().Body().Deposits()),
		"block":     fmt.Sprintf("%#x", data.BlockRoot),
	}).Info("=== LogProcessing: StateTracker: main sync: baseSpine")

	return nil
}

func (s *Service) headSyncTxLogsByBlockEvt(data *statefeed.BlockProcessedData, isBadDepositRoot bool) error {
	if data.InitialSync {
		return nil
	}
	// if the first event recieved - reinit required props
	if s.lastHandledState != nil {
		return nil
	}
	//check gwat connection is established
	if s.eth1DataFetcher == nil {
		return nil
	}

	evtSt, err := s.cfg.stateGen.StateByRoot(s.ctx, data.BlockRoot)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			" slot": data.Slot,
			"block": fmt.Sprintf("%#x", data.BlockRoot),
		}).Error("=== LogProcessing: StateTracker: head sync: get state failed")
		return nil
	}
	evtFcpRoot := bytesutil.ToBytes32(evtSt.FinalizedCheckpoint().Root)
	cpSt, err := s.cfg.stateGen.StateByRoot(s.ctx, evtFcpRoot)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			" slot":            data.Slot,
			"block":            fmt.Sprintf("%#x", data.BlockRoot),
			"cpRoot":           fmt.Sprintf("%#x", evtFcpRoot),
			"isBadDepositRoot": isBadDepositRoot,
		}).Error("=== LogProcessing: StateTracker: head sync: get cp state failed")
		return err
	}

	s.lastHandledSlot = cpSt.Slot()

	if isBadDepositRoot {
		log.Warn("=== LogProcessing: StateTracker: head sync: reset by bad deposit root")
		// reset to recalculate all deposits
		genesisSt, err := s.cfg.beaconDB.GenesisState(s.ctx)
		if err != nil {
			log.WithError(err).Error("=== LogProcessing: StateTracker: head sync: retrieve state failed")
			return err
		}
		err = s.resetEth1Data(s.ctx)
		if err != nil {
			log.WithError(err).Error("Init state tracker work mode: reset eth data failed")
			return err
		}
		baseSpine := helpers.GetTerminalFinalizedSpine(genesisSt)
		s.lastHandledState = genesisSt
		blockNumberGauge.Set(0)
		s.latestEth1Data.BlockHash = baseSpine.Bytes()
		s.latestEth1Data.BlockTime = genesisSt.GenesisTime()
		s.latestEth1Data.CpHash = baseSpine.Bytes()
		s.latestEth1Data.LastRequestedBlock = 0
	} else {
		prevRoot := bytesutil.ToBytes32(cpSt.FinalizedCheckpoint().Root)
		prevSt, err := s.cfg.stateGen.StateByRoot(s.ctx, prevRoot)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				" slot":      data.Slot,
				"block":      fmt.Sprintf("%#x", data.BlockRoot),
				"prevCpRoot": fmt.Sprintf("%#x", prevRoot),
			}).Error("=== LogProcessing: StateTracker: head sync: get prev cp state failed")
			return err
		}

		if uint64(s.lastReceivedMerkleIndex) < prevSt.Eth1Data().DepositCount-1 {
			handledCount, err := s.handleFinalizedDeposits(bytesutil.ToBytes32(data.SignedBlock.Block().ParentRoot()))
			if err != nil {
				log.WithError(err).WithFields(logrus.Fields{
					" slot":        data.Slot,
					"deposits":     len(data.SignedBlock.Block().Body().Deposits()),
					"block":        fmt.Sprintf("%#x", data.BlockRoot),
					"handledCount": handledCount,
				}).Error("=== LogProcessing: StateTracker: head sync: rebuild merkle trie failed")
				return fmt.Errorf("%w: %w", errInvalidDepositRoot, err)
			}
		}

		baseSpine := helpers.GetTerminalFinalizedSpine(prevSt)
		prevHeader, err := s.eth1DataFetcher.HeaderByHash(s.ctx, baseSpine)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				" slot":      data.Slot,
				"block":      fmt.Sprintf("%#x", data.BlockRoot),
				"prevCpRoot": fmt.Sprintf("%#x", prevRoot),
				"baseSpine":  fmt.Sprintf("%#x", baseSpine),
			}).Error("=== LogProcessing: StateTracker: head sync: fetch shard1 header failed")
			if !s.pollConnActive {
				// check gwat connection
				head, err1 := s.eth1DataFetcher.HeaderByNumber(s.ctx, nil)
				log.WithError(err1).WithFields(logrus.Fields{
					"head": fmt.Sprintf("%v", head),
				}).Info("=== LogProcessing: StateTracker: head sync: update shard con status 1")
				if err1 != nil {
					go s.pollConnectionStatus(s.ctx)
				}
			}
			return err
		}
		s.lastHandledState = prevSt
		blockNumberGauge.Set(float64(prevHeader.Nr()))
		s.latestEth1Data.BlockHeight = prevHeader.Nr()
		s.latestEth1Data.BlockHash = baseSpine.Bytes()
		s.latestEth1Data.BlockTime = prevHeader.Time
		s.latestEth1Data.CpHash = baseSpine.Bytes()
		s.latestEth1Data.CpNr = prevHeader.Nr()
		s.latestEth1Data.LastRequestedBlock = s.followBlockHeight(s.ctx)
	}

	baseSpine := helpers.GetTerminalFinalizedSpine(cpSt)
	log.WithFields(logrus.Fields{
		" slot":                      data.Slot,
		"block":                      fmt.Sprintf("%#x", data.BlockRoot),
		"lState.DepositCount":        fmt.Sprintf("%d", s.lastHandledState.Eth1Data().DepositCount),
		"lastReceivedMerkleIndex":    fmt.Sprintf("%d", s.lastReceivedMerkleIndex),
		"baseSpine":                  fmt.Sprintf("%#x", baseSpine),
		"stSlot":                     cpSt.Slot(),
		"stFinalizedCheckpointEpoch": cpSt.FinalizedCheckpointEpoch(),
	}).Info("=== LogProcessing: StateTracker: head sync: request past logs")

	header, err := s.eth1DataFetcher.HeaderByHash(s.ctx, baseSpine)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			" slot":     data.Slot,
			"block":     fmt.Sprintf("%#x", data.BlockRoot),
			"baseSpine": fmt.Sprintf("%#x", baseSpine),
			"stSlot":    cpSt.Slot(),
		}).Error("=== LogProcessing: StateTracker: head sync: FinalizedCheckpoint: could not fetch latest shard1 header")

		if !s.pollConnActive {
			// check gwat connection
			head, err1 := s.eth1DataFetcher.HeaderByNumber(s.ctx, nil)
			log.WithError(err1).WithFields(logrus.Fields{
				"head": fmt.Sprintf("%v", head),
			}).Info("=== LogProcessing: StateTracker: head sync: update shard con status 2")
			if err1 != nil {
				go s.pollConnectionStatus(s.ctx)
			}
		}
		s.lastHandledState = nil
		return err
	}
	s.processBlockHeader(header, &baseSpine)
	s.handleETH1FollowDistance()

	err = s.rmOutdatedWithdrawalsFromPool(cpSt.Slot())
	if err != nil {
		// reset to retry next iteration
		s.cfg.withdrawalPool.Reset()
		s.lastHandledState = nil
		return err
	}
	s.checkDefaultEndpoint(s.ctx)

	s.lastHandledBlock = evtFcpRoot
	s.lastHandledSlot = cpSt.Slot()
	s.lastHandledState = cpSt

	//check depositRoot in cache
	cpDepRoot := cpSt.Eth1Data().DepositRoot
	cpDepCount := cpSt.Eth1Data().DepositCount
	deps := s.cfg.depositCache.AllDepositContainers(s.ctx)

	if uint64(len(deps)) < cpDepCount || !bytes.Equal(deps[cpDepCount-1].DepositRoot, cpDepRoot) {
		// init all log processing from genesis
		s.lastHandledState = nil
		log.WithError(err).WithFields(logrus.Fields{
			"deps":                    len(deps),
			" cpDepRoot":              fmt.Sprintf("%#x", cpDepRoot),
			" cacheDepositRoot":       fmt.Sprintf("%#x", deps[cpDepCount-1].DepositRoot),
			" slot":                   data.Slot,
			"block":                   fmt.Sprintf("%#x", data.BlockRoot),
			"cpSlot":                  cpSt.Slot(),
			"cpDepCount":              fmt.Sprintf("%d", cpSt.Eth1Data().DepositCount),
			"lastReceivedMerkleIndex": fmt.Sprintf("%d", s.lastReceivedMerkleIndex),
		}).Error("=== LogProcessing: StateTracker: head sync: FinalizedCheckpoint: deposit root validation failed")
		return fmt.Errorf("%w: %w", errInvalidDepositRoot, err)
	}

	return nil
}

func (s *Service) handlerFinalizedCheckpointEvt(data *ethpbv1.EventFinalizedCheckpoint) error {
	// if the first event recieved - reinit required props
	if s.lastHandledState == nil {
		return nil
	}
	//check gwat connection is established
	if s.eth1DataFetcher == nil {
		return nil
	}

	s.lastHandledSlot = data.FinalizationSlot

	// check delegating stake fork active
	if !params.BeaconConfig().IsDelegatingStakeSlot(s.lastHandledSlot) {
		handledCount, err := s.handleFinalizedDeposits(bytesutil.ToBytes32(data.Block))
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"lState.Count":            fmt.Sprintf("%d", s.lastHandledState.Eth1Data().DepositCount),
				"lastReceivedMerkleIndex": fmt.Sprintf("%d", s.lastReceivedMerkleIndex),
				"evtEpoch":                data.Epoch,
				"evtBlock":                fmt.Sprintf("%#x", data.Block),
				"evtState":                fmt.Sprintf("%#x", data.State),
				"evtIsOptimistic":         data.ExecutionOptimistic,
				"handledCount":            handledCount,
			}).Error("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint: handle finalized deposits before delegate fork")
			return fmt.Errorf("%w: %w", errInvalidDepositRoot, err)
		}
		log.WithFields(logrus.Fields{
			"lState.DepositCount":     fmt.Sprintf("%d", s.lastHandledState.Eth1Data().DepositCount),
			"lastReceivedMerkleIndex": fmt.Sprintf("%d", s.lastReceivedMerkleIndex),
			"evtEpoch":                data.Epoch,
			"evtBlock":                fmt.Sprintf("%#x", data.Block),
			"evtState":                fmt.Sprintf("%#x", data.State),
			"evtIsOptimistic":         data.ExecutionOptimistic,
			"handledCount":            handledCount,
		}).Info("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint: handle finalized deposits before delegate fork")
		return nil
	}

	st, err := s.cfg.stateGen.StateByRoot(s.ctx, bytesutil.ToBytes32(data.Block))
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"evtEpoch":        data.Epoch,
			"evtBlock":        fmt.Sprintf("%#x", data.Block),
			"evtState":        fmt.Sprintf("%#x", data.State),
			"evtIsOptimistic": data.ExecutionOptimistic,
		}).Error("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint: get state failed")
		return err
	}

	baseSpine := helpers.GetTerminalFinalizedSpine(st)
	log.WithFields(logrus.Fields{
		"lState.DepositCount":        fmt.Sprintf("%d", s.lastHandledState.Eth1Data().DepositCount),
		"lastReceivedMerkleIndex":    fmt.Sprintf("%d", s.lastReceivedMerkleIndex),
		"evtEpoch":                   data.Epoch,
		"evtBlock":                   fmt.Sprintf("%#x", data.Block),
		"evtState":                   fmt.Sprintf("%#x", data.State),
		"evtIsOptimistic":            data.ExecutionOptimistic,
		"baseSpine":                  fmt.Sprintf("%#x", baseSpine),
		"stSlot":                     st.Slot(),
		"stFinalizedCheckpointEpoch": st.FinalizedCheckpointEpoch(),
	}).Info("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint")

	header, err := s.eth1DataFetcher.HeaderByHash(s.ctx, baseSpine)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"baseSpine": fmt.Sprintf("%#x", baseSpine),
			"stSlot":    st.Slot(),
		}).Error("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint: could not fetch latest shard1 header")
		if !s.pollConnActive {
			// check gwat connection
			head, err1 := s.eth1DataFetcher.HeaderByNumber(s.ctx, nil)
			log.WithError(err1).WithFields(logrus.Fields{
				"head": fmt.Sprintf("%v", head),
			}).Info("=== LogProcessing: StateTracker: EvtFinalizedCheckpoint: update shard con status 3")
			if err1 != nil {
				go s.pollConnectionStatus(s.ctx)
			}
		}
		return err
	}
	s.processBlockHeader(header, &baseSpine)
	s.handleETH1FollowDistance()
	s.checkDefaultEndpoint(s.ctx)
	s.lastHandledBlock = bytesutil.ToBytes32(data.Block)
	s.lastHandledState = st

	return nil
}

// Start a web3 service's main event loop.
func (s *Service) Start() {
	if err := s.setupExecutionClientConnections(s.ctx, s.cfg.currHttpEndpoint); err != nil {
		log.WithError(err).Error("Could not connect to execution endpoint")
	}
	// If the chain has not started already and we don't have access to eth1 nodes, we will not be
	// able to generate the genesis state.
	if !s.chainStartData.Chainstarted && s.cfg.currHttpEndpoint.Url == "" {
		// check for genesis state before shutting down the node,
		// if a genesis state exists, we can continue on.
		genState, err := s.cfg.beaconDB.GenesisState(s.ctx)
		if err != nil {
			log.Fatal(err)
		}
		if genState == nil || genState.IsNil() {
			log.Fatal("cannot create genesis state: no shard1 http endpoint defined")
		}
	}

	s.isRunning = true

	// Poll the execution client connection and fallback if errors occur.
	s.pollConnectionStatus(s.ctx)

	// old logic (before delegating stake fork)
	go s.run(s.ctx.Done())
}

// Stop the web3 service's main event loop and associated goroutines.
func (s *Service) Stop() error {
	if s.cancel != nil {
		defer s.cancel()
	}
	if s.rpcClient != nil {
		s.rpcClient.Close()
	}
	if s.eth1DataFetcher != nil {
		s.eth1DataFetcher.Close()
	}
	return nil
}

// ClearPreGenesisData clears out the stored chainstart deposits and beacon state.
func (s *Service) ClearPreGenesisData() {
	s.chainStartData.ChainstartDeposits = []*ethpb.Deposit{}
	if features.Get().EnableNativeState {
		s.preGenesisState = &nativev1.BeaconState{}
	} else {
		s.preGenesisState = &v1.BeaconState{}
	}
}

// ChainStartEth1Data returns the eth1 data at chainstart.
func (s *Service) ChainStartEth1Data() *ethpb.Eth1Data {
	return s.chainStartData.Eth1Data
}

// PreGenesisState returns a state that contains
// pre-chainstart deposits.
func (s *Service) PreGenesisState() state.BeaconState {
	return s.preGenesisState
}

// Status is service health checks. Return nil or error.
func (s *Service) Status() error {
	// Service don't start
	if !s.isRunning {
		return nil
	}
	// get error from run function
	return s.runError
}

func (s *Service) updateBeaconNodeStats() {
	bs := clientstats.BeaconNodeStats{}
	if len(s.cfg.httpEndpoints) > 1 {
		bs.SyncEth1FallbackConfigured = true
	}
	if s.IsConnectedToETH1() {
		if s.primaryConnected() {
			bs.SyncEth1Connected = true
		} else {
			bs.SyncEth1FallbackConnected = true
		}
	}
	s.cfg.beaconNodeStatsUpdater.Update(bs)
}

func (s *Service) updateCurrHttpEndpoint(endpoint network.Endpoint) {
	s.cfg.currHttpEndpoint = endpoint
	s.updateBeaconNodeStats()
}

func (s *Service) updateConnectedETH1(state bool) {
	s.connectedETH1 = state
	s.updateBeaconNodeStats()
}

// IsConnectedToETH1 checks if the beacon node is connected to a ETH1 Node.
func (s *Service) IsConnectedToETH1() bool {
	return s.connectedETH1
}

// CurrentETH1Endpoint returns the URL of the current ETH1 endpoint.
func (s *Service) CurrentETH1Endpoint() string {
	return s.cfg.currHttpEndpoint.Url
}

// CurrentETH1ConnectionError returns the error (if any) of the current connection.
func (s *Service) CurrentETH1ConnectionError() error {
	return s.runError
}

// ETH1Endpoints returns the slice of HTTP endpoint URLs (default is 0th element).
func (s *Service) ETH1Endpoints() []string {
	var eps []string
	for _, ep := range s.cfg.httpEndpoints {
		eps = append(eps, ep.Url)
	}
	return eps
}

// ETH1ConnectionErrors returns a slice of errors for each HTTP endpoint. An error
// of nil means the connection was successful.
func (s *Service) ETH1ConnectionErrors() []error {
	var errs []error
	for _, ep := range s.cfg.httpEndpoints {
		client, err := s.newRPCClientWithAuth(s.ctx, ep)
		if err != nil {
			client.Close()
			errs = append(errs, err)
			continue
		}
		if err := ensureCorrectExecutionChain(s.ctx, ethclient.NewClient(client)); err != nil {
			client.Close()
			errs = append(errs, err)
			continue
		}
		client.Close()
	}
	return errs
}

// refers to the latest eth1 block which follows the condition: eth1_timestamp +
// SECONDS_PER_ETH1_BLOCK * ETH1_FOLLOW_DISTANCE <= current_unix_time
func (s *Service) followBlockHeight(_ context.Context) uint64 {
	latestValidBlock := uint64(0)
	if params.BeaconConfig().IsDelegatingStakeSlot(s.lastHandledSlot) {
		return s.latestEth1Data.BlockHeight
	}
	if s.latestEth1Data.CpNr > params.BeaconConfig().Eth1FollowDistance {
		latestValidBlock = s.latestEth1Data.CpNr - params.BeaconConfig().Eth1FollowDistance
	}
	return latestValidBlock
}

func (s *Service) initDepositCaches(ctx context.Context, ctrs []*ethpb.DepositContainer) error {
	if len(ctrs) == 0 {
		return nil
	}
	s.cfg.depositCache.InsertDepositContainers(ctx, ctrs)
	if !s.chainStartData.Chainstarted {
		// do not add to pending cache
		// if no genesis state exists.
		validDepositsCount.Add(float64(s.preGenesisState.Eth1DepositIndex()))
		return nil
	}
	genesisState, err := s.cfg.beaconDB.GenesisState(ctx)
	if err != nil {
		return err
	}
	// Default to all deposits post-genesis deposits in
	// the event we cannot find a finalized state.
	currIndex := genesisState.Eth1DepositIndex()
	chkPt, err := s.cfg.beaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return err
	}
	rt := bytesutil.ToBytes32(chkPt.Root)
	if rt != [32]byte{} {
		fState := s.cfg.finalizedStateAtStartup
		if fState == nil || fState.IsNil() {
			return errors.Errorf("finalized state with root %#x is nil", rt)
		}
		// Set deposit index to the one in the current archived state.
		currIndex = fState.Eth1DepositIndex()

		// when a node pauses for some time and starts again, the deposits to finalize
		// accumulates. we finalize them here before we are ready to receive a block.
		// Otherwise, the first few blocks will be slower to compute as we will
		// hold the lock and be busy finalizing the deposits.
		// The deposit index in the state is always the index of the next deposit
		// to be included(rather than the last one to be processed). This was most likely
		// done as the state cannot represent signed integers.
		actualIndex := int64(currIndex) - 1 // lint:ignore uintcast -- deposit index will not exceed int64 in your lifetime.
		s.cfg.depositCache.InsertFinalizedDeposits(ctx, actualIndex)
		// Deposit proofs are only used during state transition and can be safely removed to save space.

		if err = s.cfg.depositCache.PruneProofs(ctx, actualIndex); err != nil {
			return errors.Wrap(err, "could not prune deposit proofs")
		}
	}
	validDepositsCount.Add(float64(currIndex))
	// Only add pending deposits if the container slice length
	// is more than the current index in state.
	if uint64(len(ctrs)) > currIndex {
		for _, c := range ctrs[currIndex:] {
			s.cfg.depositCache.InsertPendingDeposit(ctx, c.Deposit, c.Eth1BlockHeight, c.Index, bytesutil.ToBytes32(c.DepositRoot))
		}
	}
	return nil
}

// processBlockHeader adds a newly observed eth1 block to the block cache and
// updates the latest blockHeight, blockHash, and blockTime properties of the service.
func (s *Service) processBlockHeader(header *gwatTypes.Header, hash *gwatCommon.Hash) {
	defer safelyHandlePanic()
	if header.Nr() == 0 && header.Height != 0 {
		log.WithFields(logrus.Fields{
			"header.Nr":     header.Nr(),
			"header.Height": header.Height,
			"blockHash":     hexutil.Encode(s.latestEth1Data.BlockHash),
		}).Warn("Latest shard1 chain event: skipping not finalized block")
		return
	}
	if s.latestEth1Data.BlockHeight > header.Nr() {
		return
	}

	//note: something wrong with calculating hash from header
	s.latestEth1Data.BlockHash = header.Hash().Bytes()
	if hash != nil {
		s.latestEth1Data.BlockHash = hash.Bytes()
	}
	s.latestEth1Data.BlockHeight = header.Nr()
	s.latestEth1Data.BlockTime = header.Time
	s.latestEth1Data.CpHash = header.CpHash.Bytes()
	s.latestEth1Data.CpNr = header.CpNumber

	blockNumberGauge.Set(float64(header.Nr()))

	log.WithFields(logrus.Fields{
		"slot":        header.Slot,
		"height":      header.Height,
		"blockNumber": s.latestEth1Data.BlockHeight,
		"blockHash":   fmt.Sprintf("%#x", s.latestEth1Data.BlockHash),
	}).Info("Latest shard1 chain event")
	if err := s.headerCache.AddHeader(header); err != nil {
		return
	}
}

// batchRequestHeaders requests the block range specified in the arguments. Instead of requesting
// each block in one call, it batches all requests into a single rpc call.
func (s *Service) batchRequestHeaders(startBlock, endBlock uint64) ([]*gwatTypes.Header, error) {
	if startBlock > endBlock {
		return nil, fmt.Errorf("start block height %d cannot be > end block height %d", startBlock, endBlock)
	}
	requestRange := (endBlock - startBlock) + 1
	elems := make([]gethRPC.BatchElem, 0, requestRange)
	headers := make([]*gwatTypes.Header, 0, requestRange)
	errs := make([]error, 0, requestRange)
	if requestRange == 0 {
		return headers, nil
	}
	for i := startBlock; i <= endBlock; i++ {
		header := &gwatTypes.Header{}
		err := error(nil)
		elems = append(elems, gethRPC.BatchElem{
			Method: "eth_getBlockByNumber",
			Args:   []interface{}{hexutil.EncodeBig(big.NewInt(0).SetUint64(i)), false},
			Result: header,
			Error:  err,
		})
		headers = append(headers, header)
		errs = append(errs, err)
	}
	ioErr := s.rpcClient.BatchCall(elems)
	if ioErr != nil {
		return nil, ioErr
	}
	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}
	for _, h := range headers {
		if h != nil {
			if err := s.headerCache.AddHeader(h); err != nil {
				return nil, err
			}
		}
	}
	return headers, nil
}

// safelyHandleHeader will recover and log any panic that occurs from the
// block
func safelyHandlePanic() {
	if r := recover(); r != nil {
		log.WithFields(logrus.Fields{
			"r": r,
		}).Error("Panicked when handling data from shard1 Chain! Recovering...")

		debug.PrintStack()
	}
}

func (s *Service) handleETH1FollowDistance() {
	defer safelyHandlePanic()
	ctx := s.ctx

	if !s.chainStartData.Chainstarted {
		if err := s.checkBlockNumberForChainStart(ctx, s.latestEth1Data.LastRequestedBlock); err != nil {
			s.runError = err
			log.Error(err)
			return
		}
	}

	if err := s.requestBatchedHeadersAndLogs(ctx); err != nil {
		s.runError = err
		log.Error(err)
		return
	}
	// Reset the Status.
	if s.runError != nil {
		s.runError = nil
	}
}

func (s *Service) initPOWService() {
	// Use a custom logger to only log errors
	logCounter := 0
	errorLogger := func(err error, msg string) {
		if logCounter > logThreshold {
			log.Errorf("%s: %v", msg, err)
			logCounter = 0
		}
		logCounter++
	}

	// Run in a select loop to retry in the event of any failures.
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			ctx := s.ctx
			header, err := s.eth1DataFetcher.HeaderByNumber(ctx, nil)
			if err != nil {
				s.retryExecutionClientConnection(ctx, err)
				errorLogger(err, "Unable to retrieve latest execution client header")
				continue
			}

			if header.Nr() == 0 && header.Height != 0 {
				log.WithFields(logrus.Fields{
					"header.Nr":     header.Nr(),
					"header.Height": header.Height,
					"blockHash":     hexutil.Encode(s.latestEth1Data.BlockHash),
				}).Fatal("Latest shard1 block is not finalized")
			}

			//s.latestEth1Data.BlockHeight = header.CpNumber
			//s.latestEth1Data.BlockHash = header.CpHash.Bytes()
			//s.latestEth1Data.BlockTime = header.Time
			//s.latestEth1Data.CpHash = header.CpHash.Bytes()
			//s.latestEth1Data.CpNr = header.CpNumber

			log.WithFields(logrus.Fields{
				"EthLFinNr":                              header.Nr(),
				"s.preGenesisState.Eth1Data().BlockHash": fmt.Sprintf("%#x", s.preGenesisState.Eth1Data().BlockHash),
				//"EthLFinHash":          fmt.Sprintf("%#x", header.Hash()),
				"lastEth.LastReqBlock":    s.latestEth1Data.LastRequestedBlock,
				"lastEth.CpNr":            s.latestEth1Data.CpNr,
				"lastEth.CpHash":          fmt.Sprintf("%#x", s.latestEth1Data.CpHash),
				"lastEth.BlockHeight":     s.latestEth1Data.BlockHeight,
				"depositTrie.NumOfItems":  s.depositTrie.NumOfItems(),
				"lastReceivedMerkleIndex": s.lastReceivedMerkleIndex,
			}).Info("=== LogProcessing: initPOWService: 00000")

			if err := s.processPastLogs(ctx); err != nil {
				s.retryExecutionClientConnection(ctx, err)
				errorLogger(err, "Unable to process past deposit contract logs")
				continue
			}
			// Handle edge case with embedded genesis state by fetching genesis header to determine
			// its height.
			if s.chainStartData.Chainstarted && s.chainStartData.GenesisBlock != 0 {
				s.chainStartData.GenesisBlock = 0
				if err := s.savePowchainData(ctx); err != nil {
					s.retryExecutionClientConnection(ctx, err)
					errorLogger(err, "Unable to save execution client data")
					continue
				}
			}
			return
		}
	}
}

// run subscribes to all the services for the ETH1.0 chain.
func (s *Service) run(done <-chan struct{}) {
	s.runError = nil

	// check delegating stake fork active
	if params.BeaconConfig().IsDelegatingStakeSlot(s.lastHandledSlot) {
		log.WithFields(logrus.Fields{
			"lastHandledSlot":             s.lastHandledSlot,
			"s.preGenesisState.BlockHash": fmt.Sprintf("%#x", s.preGenesisState.Eth1Data().BlockHash),
			//"EthLFinHash":          fmt.Sprintf("%#x", header.Hash()),
			"lastEth.LastReqBlock":    s.latestEth1Data.LastRequestedBlock,
			"lastEth.CpNr":            s.latestEth1Data.CpNr,
			"lastEth.CpHash":          fmt.Sprintf("%#x", s.latestEth1Data.CpHash),
			"lastEth.BlockHeight":     s.latestEth1Data.BlockHeight,
			"depositTrie.NumOfItems":  s.depositTrie.NumOfItems(),
			"lastReceivedMerkleIndex": s.lastReceivedMerkleIndex,
		}).Info("=== LogProcessing: run: start the delegating stake fork")

		return
	}

	s.initPOWService()

	chainstartTicker := time.NewTicker(logPeriod)
	defer chainstartTicker.Stop()

	for {
		select {
		case <-done:
			s.isRunning = false
			s.runError = nil
			if s.rpcClient != nil {
				s.rpcClient.Close()
			}
			s.updateConnectedETH1(false)
			log.Info("Context closed, exiting goroutine")
			return
		case <-s.headTicker.C:
			// check delegating stake fork active
			//s.lastHandledSlot = slots.CurrentSlot(s.preGenesisState.GenesisTime())
			if params.BeaconConfig().IsDelegatingStakeSlot(s.lastHandledSlot) {
				log.WithFields(logrus.Fields{
					"slot":                        s.lastHandledSlot,
					"s.preGenesisState.BlockHash": fmt.Sprintf("%#x", s.preGenesisState.Eth1Data().BlockHash),
					//"EthLFinHash":          fmt.Sprintf("%#x", header.Hash()),
					"lastEth.LastReqBlock":    s.latestEth1Data.LastRequestedBlock,
					"lastEth.CpNr":            s.latestEth1Data.CpNr,
					"lastEth.CpHash":          fmt.Sprintf("%#x", s.latestEth1Data.CpHash),
					"lastEth.BlockHeight":     s.latestEth1Data.BlockHeight,
					"depositTrie.NumOfItems":  s.depositTrie.NumOfItems(),
					"lastReceivedMerkleIndex": s.lastReceivedMerkleIndex,
				}).Info("=== LogProcessing: run ticker: start the delegating stake fork")
				return
			}

			log.WithFields(logrus.Fields{
				"slot":                        s.lastHandledSlot,
				"IsDelegate":                  params.BeaconConfig().IsDelegatingStakeSlot(s.lastHandledSlot),
				"s.preGenesisState.BlockHash": fmt.Sprintf("%#x", s.preGenesisState.Eth1Data().BlockHash),
				//"EthLFinHash":          fmt.Sprintf("%#x", header.Hash()),
				"lastEth.LastReqBlock":    s.latestEth1Data.LastRequestedBlock,
				"lastEth.CpNr":            s.latestEth1Data.CpNr,
				"lastEth.CpHash":          fmt.Sprintf("%#x", s.latestEth1Data.CpHash),
				"lastEth.BlockHeight":     s.latestEth1Data.BlockHeight,
				"depositTrie.NumOfItems":  s.depositTrie.NumOfItems(),
				"lastReceivedMerkleIndex": s.lastReceivedMerkleIndex,
			}).Info("=== LogProcessing: run ticker")

			head, err := s.eth1DataFetcher.HeaderByNumber(s.ctx, nil)
			if err != nil {
				s.pollConnectionStatus(s.ctx)
				log.WithError(err).Error("Could not fetch latest shard1 header")
				continue
			}
			if head.CpHash == (gwatCommon.Hash{}) {
				//if genesis
				if head.Height == 0 {
					gst, err := s.cfg.beaconDB.GenesisState(s.ctx)
					if err != nil {
						log.Fatal(err)
					}
					if gst == nil || gst.IsNil() {
						log.Fatal("cannot create genesis state: no shard1 http endpoint defined 0")
					}
					head.CpHash = gwatCommon.BytesToHash(gst.Eth1Data().BlockHash)
				} else {
					log.Error("Could not fetch latest shard1 header: bad header")
					continue
				}
			}
			s.processBlockHeader(head, nil)
			s.handleETH1FollowDistance()
			s.checkDefaultEndpoint(s.ctx)
		case <-chainstartTicker.C:
			if s.chainStartData.Chainstarted {
				chainstartTicker.Stop()
				continue
			}
			s.logTillChainStart(context.Background())
		}
	}
}

// logs the current thresholds required to hit chainstart every minute.
func (s *Service) logTillChainStart(ctx context.Context) {
	if s.chainStartData.Chainstarted {
		return
	}
	_, blockTime, err := s.retrieveBlockHashAndTime(s.ctx, big.NewInt(int64(s.latestEth1Data.LastRequestedBlock)))
	if err != nil {
		log.Error(err)
		return
	}
	valCount, genesisTime := s.currentCountAndTime(ctx, blockTime)
	valNeeded := uint64(0)
	if valCount < params.BeaconConfig().MinGenesisActiveValidatorCount {
		valNeeded = params.BeaconConfig().MinGenesisActiveValidatorCount - valCount
	}
	secondsLeft := uint64(0)
	if genesisTime < params.BeaconConfig().MinGenesisTime {
		secondsLeft = params.BeaconConfig().MinGenesisTime - genesisTime
	}

	fields := logrus.Fields{
		"Additional validators needed": valNeeded,
	}
	if secondsLeft > 0 {
		fields["Generating genesis state in"] = time.Duration(secondsLeft) * time.Second
	}

	log.WithFields(fields).Info("Currently waiting for chainstart")
}

// initializes our service from the provided eth1data object by initializing all the relevant
// fields and data.
func (s *Service) initializeEth1Data(ctx context.Context, eth1DataInDB *ethpb.ETH1ChainData) error {
	// The node has no eth1data persisted on disk, so we exit and instead
	// request from contract logs.
	if eth1DataInDB == nil {
		return nil
	}
	s.depositTrie = trie.CreateTrieFromProto(eth1DataInDB.Trie)
	s.chainStartData = eth1DataInDB.ChainstartData
	var err error
	if !reflect.ValueOf(eth1DataInDB.BeaconState).IsZero() {
		s.preGenesisState, err = v1.InitializeFromProto(eth1DataInDB.BeaconState)
		if err != nil {
			return errors.Wrap(err, "Could not initialize state trie")
		}
	}
	s.latestEth1Data = eth1DataInDB.CurrentEth1Data
	numOfItems := s.depositTrie.NumOfItems()
	s.lastReceivedMerkleIndex = int64(numOfItems - 1)
	if err := s.initDepositCaches(ctx, eth1DataInDB.DepositContainers); err != nil {
		return errors.Wrap(err, "could not initialize caches")
	}
	return nil
}

// resets eth1data properties to recalculate all tx-logs starting from genesis
func (s *Service) resetEth1Data(ctx context.Context) error {
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return errors.Wrap(err, "could not reset deposit trie")
	}
	genState, err := s.cfg.beaconDB.GenesisState(s.ctx)
	if err != nil {
		return errors.Wrap(err, "reset: could not reset genesis state")
	}
	err = s.cfg.depositCache.Reset(ctx)
	if err != nil {
		return errors.Wrap(err, "reset: could not reset deposit trie")
	}
	s.depositTrie = depositTrie
	s.preGenesisState = genState
	s.latestEth1Data = &ethpb.LatestETH1Data{
		BlockHeight:        0,
		BlockTime:          0,
		BlockHash:          []byte{},
		LastRequestedBlock: 0,
		CpHash:             []byte{},
		CpNr:               0,
	}
	numOfItems := s.depositTrie.NumOfItems()
	s.lastReceivedMerkleIndex = int64(numOfItems - 1)

	return nil
}

// validates that all deposit containers are valid and have their relevant indices
// in order.
func validateDepositContainers(ctrs []*ethpb.DepositContainer) bool {
	ctrLen := len(ctrs)
	// Exit for empty containers.
	if ctrLen == 0 {
		return true
	}
	// Sort deposits in ascending order.
	sort.Slice(ctrs, func(i, j int) bool {
		return ctrs[i].Index < ctrs[j].Index
	})
	startIndex := int64(0)
	for _, c := range ctrs {
		if c.Index != startIndex {
			log.Info("Recovering missing deposit containers, node is re-requesting missing deposit data")
			return false
		}
		startIndex++
	}
	return true
}

// validates the current powchain data saved and makes sure that any
// embedded genesis state is correctly accounted for.
func (s *Service) ensureValidPowchainData(ctx context.Context) error {
	genState, err := s.cfg.beaconDB.GenesisState(ctx)
	if err != nil {
		return err
	}
	// Exit early if no genesis state is saved.
	if genState == nil || genState.IsNil() {
		return nil
	}
	eth1Data, err := s.cfg.beaconDB.PowchainData(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve shard1 data")
	}
	if eth1Data == nil || !eth1Data.ChainstartData.Chainstarted || !validateDepositContainers(eth1Data.DepositContainers) {
		pbState, err := v1.ProtobufBeaconState(s.preGenesisState.InnerStateUnsafe())
		if err != nil {
			return err
		}
		s.chainStartData = &ethpb.ChainStartData{
			Chainstarted:       true,
			GenesisTime:        genState.GenesisTime(),
			GenesisBlock:       0,
			Eth1Data:           genState.Eth1Data(),
			ChainstartDeposits: make([]*ethpb.Deposit, 0),
		}
		eth1Data = &ethpb.ETH1ChainData{
			CurrentEth1Data:   s.latestEth1Data,
			ChainstartData:    s.chainStartData,
			BeaconState:       pbState,
			Trie:              s.depositTrie.ToProto(),
			DepositContainers: s.cfg.depositCache.AllDepositContainers(ctx),
		}
		return s.cfg.beaconDB.SavePowchainData(ctx, eth1Data)
	}
	return nil
}

func dedupEndpoints(endpoints []string) []string {
	selectionMap := make(map[string]bool)
	newEndpoints := make([]string, 0, len(endpoints))
	for _, point := range endpoints {
		if selectionMap[point] {
			continue
		}
		newEndpoints = append(newEndpoints, point)
		selectionMap[point] = true
	}
	return newEndpoints
}

// Checks if the provided timestamp is beyond the prescribed bound from
// the current wall clock time.
func eth1HeadIsBehind(timestamp uint64) bool {
	timeout := prysmTime.Now().Add(-eth1Threshold)
	// check that web3 client is syncing
	return time.Unix(int64(timestamp), 0).Before(timeout) // lint:ignore uintcast -- timestamp will not exceed int64 in your lifetime.
}

func (s *Service) primaryConnected() bool {
	return s.cfg.currHttpEndpoint.Equals(s.cfg.httpEndpoints[0])
}

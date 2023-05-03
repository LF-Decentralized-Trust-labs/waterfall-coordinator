// Package blockchain defines the life-cycle of the blockchain at the core of
// Ethereum, including processing of new blocks and attestations using proof of stake.
package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/pkg/errors"
	types "github.com/prysmaticlabs/eth2-types"
	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/async/event"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain/store"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache/depositcache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed"
	statefeed "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/transition"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db"
	f "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/forkchoice"
	doublylinkedtree "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/forkchoice/doubly-linked-tree"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/forkchoice/protoarray"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/attestations"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/slashings"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/voluntaryexits"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/powchain"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/stategen"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/cmd/beacon-chain/flags"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/features"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	prysmTime "gitlab.waterfall.network/waterfall/protocol/coordinator/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"go.opencensus.io/trace"
)

// SyncSrv interface to treat sync functionality.
type SyncSrv interface {
	SetIsSyncFn(fn func() bool)
	AddFinalizedSpines(finSpines gwatCommon.HashArray)
	ResetFinalizedSpines()
}

// headSyncMinEpochsAfterCheckpoint defines how many epochs should elapse after known finalization
// checkpoint for head sync to be triggered.
const headSyncMinEpochsAfterCheckpoint = 128

// Service represents a service that handles the internal
// logic of managing the full PoS beacon chain.
type Service struct {
	cfg         *config
	ctx         context.Context
	cancel      context.CancelFunc
	genesisTime time.Time
	spineData   spineData
	head        *head
	headLock    sync.RWMutex
	// originBlockRoot is the genesis root, or weak subjectivity checkpoint root, depending on how the node is initialized
	originBlockRoot       [32]byte
	nextEpochBoundarySlot types.Slot
	boundaryRoots         [][32]byte
	checkpointStateCache  *cache.CheckpointStateCache
	initSyncBlocks        map[[32]byte]block.SignedBeaconBlock
	initSyncBlocksLock    sync.RWMutex
	justifiedBalances     *stateBalanceCache
	wsVerifier            *WeakSubjectivityVerifier
	store                 *store.Store
	fnIsSync              func() bool
	newHeadCh             chan *head
	isGwatSyncing         bool
}

// config options for the service.
type config struct {
	BeaconBlockBuf          int
	ChainStartFetcher       powchain.ChainStartFetcher
	BeaconDB                db.HeadAccessDatabase
	DepositCache            *depositcache.DepositCache
	ProposerSlotIndexCache  *cache.ProposerPayloadIDsCache
	AttPool                 attestations.Pool
	ExitPool                voluntaryexits.PoolManager
	SlashingPool            slashings.PoolManager
	P2p                     p2p.Broadcaster
	MaxRoutines             int
	StateNotifier           statefeed.Notifier
	ForkChoiceStore         f.ForkChoicer
	AttService              *attestations.Service
	StateGen                *stategen.State
	SlasherAttestationsFeed *event.Feed
	WeakSubjectivityCheckpt *ethpb.Checkpoint
	BlockFetcher            powchain.POWBlockFetcher
	FinalizedStateAtStartUp state.BeaconState
	ExecutionEngineCaller   powchain.EngineCaller
}

// NewService instantiates a new block service instance that will
// be registered into a running beacon node.
func NewService(ctx context.Context, opts ...Option) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	srv := &Service{
		ctx:                  ctx,
		cancel:               cancel,
		boundaryRoots:        [][32]byte{},
		checkpointStateCache: cache.NewCheckpointStateCache(),
		initSyncBlocks:       make(map[[32]byte]block.SignedBeaconBlock),
		cfg:                  &config{},
		store:                &store.Store{},
		spineData:            spineData{},
		newHeadCh:            make(chan *head),
	}
	for _, opt := range opts {
		if err := opt(srv); err != nil {
			return nil, err
		}
	}
	var err error
	if srv.justifiedBalances == nil {
		srv.justifiedBalances, err = newStateBalanceCache(srv.cfg.StateGen)
		if err != nil {
			return nil, err
		}
	}
	srv.wsVerifier, err = NewWeakSubjectivityVerifier(srv.cfg.WeakSubjectivityCheckpt, srv.cfg.BeaconDB)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// Start a blockchain service's main event loop.
func (s *Service) Start() {
	saved := s.cfg.FinalizedStateAtStartUp

	if saved != nil && !saved.IsNil() {
		if err := s.StartFromSavedState(saved); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := s.startFromPOWChain(); err != nil {
			log.Fatal(err)
		}
	}
	s.spawnProcessAttestationsRoutine(s.cfg.StateNotifier.StateFeed())
	go s.initGwatSync()
}

// Stop the blockchain service's main event loop and associated goroutines.
func (s *Service) Stop() error {
	defer s.cancel()

	if s.cfg.StateGen != nil && s.head != nil && s.head.state != nil {
		if err := s.cfg.StateGen.ForceCheckpoint(s.ctx, s.head.state.FinalizedCheckpoint().Root); err != nil {
			return err
		}
	}

	// Save initial sync cached blocks to the DB before stop.
	return s.cfg.BeaconDB.SaveBlocks(s.ctx, s.getInitSyncBlocks())
}

// Status always returns nil unless there is an error condition that causes
// this service to be unhealthy.
func (s *Service) Status() error {
	if s.originBlockRoot == params.BeaconConfig().ZeroHash {
		return errors.New("genesis state has not been created")
	}
	if runtime.NumGoroutine() > s.cfg.MaxRoutines {
		return fmt.Errorf("too many goroutines %d", runtime.NumGoroutine())
	}
	return nil
}

func (s *Service) SetIsSyncFn(fn func() bool) {
	s.fnIsSync = fn
}

func (s *Service) isSynchronizing() bool {
	if s.fnIsSync == nil {
		return false
	}
	return s.fnIsSync()
}

func (s *Service) IsSynced() bool {
	return !s.isSynchronizing()
}

func (s *Service) IsGwatSynchronizing() bool {
	return s.isGwatSyncing
}

func (s *Service) StartFromSavedState(saved state.BeaconState) error {
	log.Info("Blockchain data already exists in DB, initializing...")
	s.genesisTime = time.Unix(int64(saved.GenesisTime()), 0) // lint:ignore uintcast -- Genesis time will not exceed int64 in your lifetime.
	s.cfg.AttService.SetGenesisTime(saved.GenesisTime())

	originRoot, err := s.originRootFromSavedState(s.ctx)
	if err != nil {
		return err
	}
	s.originBlockRoot = originRoot

	if err := s.initializeHeadFromDB(s.ctx); err != nil {
		return errors.Wrap(err, "could not set up chain info")
	}
	spawnCountdownIfPreGenesis(s.ctx, s.genesisTime, s.cfg.BeaconDB)

	justified, err := s.cfg.BeaconDB.JustifiedCheckpoint(s.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get justified checkpoint")
	}
	finalized, err := s.cfg.BeaconDB.FinalizedCheckpoint(s.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint")
	}
	s.store = store.New(justified, finalized)

	var fc f.ForkChoicer
	fRoot := bytesutil.ToBytes32(finalized.Root)
	if features.Get().EnableForkChoiceDoublyLinkedTree {
		fc = doublylinkedtree.New(justified.Epoch, finalized.Epoch)
	} else {
		fc = protoarray.New(justified.Epoch, finalized.Epoch, fRoot)
	}
	s.cfg.ForkChoiceStore = fc
	fb, err := s.cfg.BeaconDB.Block(s.ctx, s.ensureRootNotZeros(fRoot))
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint block")
	}
	if fb == nil {
		return errNilFinalizedInStore
	}

	calcRoot, err := fb.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get state for forkchoice")
	}
	st, err := s.cfg.StateGen.StateByRoot(s.ctx, calcRoot)
	if err != nil {
		return errors.Wrap(err, "could not get state for forkchoice")
	}

	fSlot := fb.Block().Slot()
	if err := fc.InsertOptimisticBlock(s.ctx, fSlot, fRoot, params.BeaconConfig().ZeroHash,
		justified.Epoch, finalized.Epoch,
		justified.Root, finalized.Root,
		fb.Block().Body().Attestations(),
		fb.Block().Body().Eth1Data().Candidates,
		st.SpineData().Finalization,
	); err != nil {
		return errors.Wrap(err, "could not insert finalized block to forkchoice")
	}

	lastValidatedCheckpoint, err := s.cfg.BeaconDB.LastValidatedCheckpoint(s.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get last validated checkpoint")
	}
	if bytes.Equal(finalized.Root, lastValidatedCheckpoint.Root) {
		if err := fc.SetOptimisticToValid(s.ctx, fRoot); err != nil {
			return errors.Wrap(err, "could not set finalized block as validated")
		}
	}

	h := s.headBlock().Block()
	if h.Slot() > fSlot {
		log.WithFields(logrus.Fields{
			"startSlot": fSlot,
			"endSlot":   h.Slot(),
		}).Info("Loading blocks to fork choice store, this may take a while.")
		if err := s.fillInForkChoiceMissingBlocks(s.ctx, h, finalized, justified); err != nil {
			return errors.Wrap(err, "could not fill in fork choice store missing blocks")
		}
	}

	// not attempting to save initial sync blocks here, because there shouldn't be any until
	// after the statefeed.Initialized event is fired (below)
	if err := s.wsVerifier.VerifyWeakSubjectivity(s.ctx, finalized.Epoch); err != nil {
		// Exit run time if the node failed to verify weak subjectivity checkpoint.
		return errors.Wrap(err, "could not verify initial checkpoint provided for chain sync")
	}

	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.Initialized,
		Data: &statefeed.InitializedData{
			StartTime:             s.genesisTime,
			GenesisValidatorsRoot: saved.GenesisValidatorsRoot(),
		},
	})

	return nil
}

func (s *Service) originRootFromSavedState(ctx context.Context) ([32]byte, error) {
	// first check if we have started from checkpoint sync and have a root
	originRoot, err := s.cfg.BeaconDB.OriginCheckpointBlockRoot(ctx)
	if err == nil {
		return originRoot, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return originRoot, errors.Wrap(err, "could not retrieve checkpoint sync chain origin data from db")
	}

	// we got here because OriginCheckpointBlockRoot gave us an ErrNotFound. this means the node was started from a genesis state,
	// so we should have a value for GenesisBlock
	genesisBlock, err := s.cfg.BeaconDB.GenesisBlock(ctx)
	if err != nil {
		return originRoot, errors.Wrap(err, "could not get genesis block from db")
	}
	if err := helpers.BeaconBlockIsNil(genesisBlock); err != nil {
		return originRoot, err
	}
	genesisBlkRoot, err := genesisBlock.Block().HashTreeRoot()
	if err != nil {
		return genesisBlkRoot, errors.Wrap(err, "could not get signing root of genesis block")
	}
	return genesisBlkRoot, nil
}

// initializeHeadFromDB uses the finalized checkpoint and head block found in the database to set the current head
// note that this may block until stategen replays blocks between the finalized and head blocks
// if the head sync flag was specified and the gap between the finalized and head blocks is at least 128 epochs long
func (s *Service) initializeHeadFromDB(ctx context.Context) error {
	finalized, err := s.cfg.BeaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint from db")
	}
	if finalized == nil {
		// This should never happen. At chain start, the finalized checkpoint
		// would be the genesis state and block.
		return errors.New("no finalized epoch in the database")
	}
	finalizedRoot := s.ensureRootNotZeros(bytesutil.ToBytes32(finalized.Root))
	var finalizedState state.BeaconState

	finalizedState, err = s.cfg.StateGen.Resume(ctx, s.cfg.FinalizedStateAtStartUp)
	if err != nil {
		return errors.Wrap(err, "could not get finalized state from db")
	}

	if flags.Get().HeadSync {
		headBlock, err := s.cfg.BeaconDB.HeadBlock(ctx)
		if err != nil {
			return errors.Wrap(err, "could not retrieve head block")
		}
		headEpoch := slots.ToEpoch(headBlock.Block().Slot())
		var epochsSinceFinality types.Epoch
		if headEpoch > finalized.Epoch {
			epochsSinceFinality = headEpoch - finalized.Epoch
		}
		// Head sync when node is far enough beyond known finalized epoch,
		// this becomes really useful during long period of non-finality.
		if epochsSinceFinality >= headSyncMinEpochsAfterCheckpoint {
			headRoot, err := headBlock.Block().HashTreeRoot()
			if err != nil {
				return errors.Wrap(err, "could not hash head block")
			}
			finalizedState, err := s.cfg.StateGen.Resume(ctx, s.cfg.FinalizedStateAtStartUp)
			if err != nil {
				return errors.Wrap(err, "could not get finalized state from db")
			}
			log.Infof("Regenerating state from the last checkpoint at slot %d to current head slot of %d."+
				"This process may take a while, please wait.", finalizedState.Slot(), headBlock.Block().Slot())
			headState, err := s.cfg.StateGen.StateByRoot(ctx, headRoot)
			if err != nil {
				return errors.Wrap(err, "could not retrieve head state")
			}
			s.setHead(headRoot, headBlock, headState)
			return nil
		} else {
			log.Warnf("Finalized checkpoint at slot %d is too close to the current head slot, "+
				"resetting head from the checkpoint ('--%s' flag is ignored).",
				finalizedState.Slot(), flags.HeadSync.Name)
		}
	}

	finalizedBlock, err := s.cfg.BeaconDB.Block(ctx, finalizedRoot)
	if err != nil {
		return errors.Wrap(err, "could not get finalized block from db")
	}

	if finalizedState == nil || finalizedState.IsNil() || finalizedBlock == nil || finalizedBlock.IsNil() {
		return errors.New("finalized state and block can't be nil")
	}
	s.setHead(finalizedRoot, finalizedBlock, finalizedState)

	return nil
}

func (s *Service) startFromPOWChain() error {
	log.Info("Waiting to reach the validator deposit threshold to start the beacon chain...")
	if s.cfg.ChainStartFetcher == nil {
		return errors.New("not configured web3Service for POW chain")
	}
	go func() {
		stateChannel := make(chan *feed.Event, 1)
		stateSub := s.cfg.StateNotifier.StateFeed().Subscribe(stateChannel)
		defer stateSub.Unsubscribe()
		for {
			select {
			case e := <-stateChannel:
				if e.Type == statefeed.ChainStarted {
					data, ok := e.Data.(*statefeed.ChainStartedData)
					if !ok {
						log.Error("event data is not type *statefeed.ChainStartedData")
						return
					}
					log.WithField("starttime", data.StartTime).Debug("Received chain start event")
					s.onPowchainStart(s.ctx, data.StartTime)
					return
				}
			case <-s.ctx.Done():
				log.Debug("Context closed, exiting goroutine")
				return
			case err := <-stateSub.Err():
				log.WithError(err).Error("Subscription to state notifier failed")
				return
			}
		}
	}()

	return nil
}

// onPowchainStart initializes a series of deposits from the ChainStart deposits in the eth1
// deposit contract, initializes the beacon chain's state, and kicks off the beacon chain.
func (s *Service) onPowchainStart(ctx context.Context, genesisTime time.Time) {
	preGenesisState := s.cfg.ChainStartFetcher.PreGenesisState()
	initializedState, err := s.initializeBeaconChain(ctx, genesisTime, preGenesisState, s.cfg.ChainStartFetcher.ChainStartEth1Data())
	if err != nil {
		log.Fatalf("Could not initialize beacon chain: %v", err)
	}
	// We start a counter to genesis, if needed.
	gRoot, err := initializedState.HashTreeRoot(s.ctx)
	if err != nil {
		log.Fatalf("Could not hash tree root genesis state: %v", err)
	}
	go slots.CountdownToGenesis(ctx, genesisTime, uint64(initializedState.NumValidators()), gRoot)

	// We send out a state initialized event to the rest of the services
	// running in the beacon node.
	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.Initialized,
		Data: &statefeed.InitializedData{
			StartTime:             genesisTime,
			GenesisValidatorsRoot: initializedState.GenesisValidatorsRoot(),
		},
	})
}

// initializes the state and genesis block of the beacon chain to persistent storage
// based on a genesis timestamp value obtained from the ChainStart event emitted
// by the ETH1.0 Deposit Contract and the POWChain service of the node.
func (s *Service) initializeBeaconChain(
	ctx context.Context,
	genesisTime time.Time,
	preGenesisState state.BeaconState,
	eth1data *ethpb.Eth1Data) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.Service.initializeBeaconChain")
	defer span.End()
	s.genesisTime = genesisTime
	unixTime := uint64(genesisTime.Unix())

	genesisState, err := transition.OptimizedGenesisBeaconState(unixTime, preGenesisState, eth1data)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize genesis state")
	}

	if err := s.saveGenesisData(ctx, genesisState); err != nil {
		return nil, errors.Wrap(err, "could not save genesis data")
	}

	log.Info("Initialized beacon chain genesis state")

	// Clear out all pre-genesis data now that the state is initialized.
	s.cfg.ChainStartFetcher.ClearPreGenesisData()

	// Update committee shuffled indices for genesis epoch.
	if err := helpers.UpdateCommitteeCache(genesisState, 0 /* genesis epoch */); err != nil {
		return nil, err
	}
	if err := helpers.UpdateProposerIndicesInCache(ctx, genesisState); err != nil {
		return nil, err
	}

	s.cfg.AttService.SetGenesisTime(genesisState.GenesisTime())

	return genesisState, nil
}

// This gets called when beacon chain is first initialized to save genesis data (state, block, and more) in db.
func (s *Service) saveGenesisData(ctx context.Context, genesisState state.BeaconState) error {
	if err := s.cfg.BeaconDB.SaveGenesisData(ctx, genesisState); err != nil {
		return errors.Wrap(err, "could not save genesis data")
	}
	genesisBlk, err := s.cfg.BeaconDB.GenesisBlock(ctx)
	if err != nil || genesisBlk == nil || genesisBlk.IsNil() {
		return fmt.Errorf("could not load genesis block: %v", err)
	}
	genesisBlkRoot, err := genesisBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get genesis block root")
	}

	s.originBlockRoot = genesisBlkRoot
	s.cfg.StateGen.SaveFinalizedState(0 /*slot*/, genesisBlkRoot, genesisState)

	// Finalized checkpoint at genesis is a zero hash.
	genesisCheckpoint := genesisState.FinalizedCheckpoint()
	s.store = store.New(genesisCheckpoint, genesisCheckpoint)

	if err := s.cfg.ForkChoiceStore.InsertOptimisticBlock(ctx,
		genesisBlk.Block().Slot(),
		genesisBlkRoot,
		params.BeaconConfig().ZeroHash,
		genesisCheckpoint.Epoch,
		genesisCheckpoint.Epoch,
		genesisCheckpoint.Root,
		genesisCheckpoint.Root,
		genesisBlk.Block().Body().Attestations(),
		genesisBlk.Block().Body().Eth1Data().Candidates,
		genesisState.SpineData().Finalization,
	); err != nil {
		log.Fatalf("Could not process genesis block for fork choice: %v", err)
	}
	if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, genesisBlkRoot); err != nil {
		log.Fatalf("Could not set optimistic status of genesis block to false: %v", err)
	}

	s.setHead(genesisBlkRoot, genesisBlk, genesisState)
	return nil
}

// This returns true if block has been processed before. Two ways to verify the block has been processed:
// 1.) Check fork choice store.
// 2.) Check DB.
// Checking 1.) is ten times faster than checking 2.)
func (s *Service) hasBlock(ctx context.Context, root [32]byte) bool {
	if s.cfg.ForkChoiceStore.HasNode(root) {
		return true
	}

	return s.cfg.BeaconDB.HasBlock(ctx, root)
}

func spawnCountdownIfPreGenesis(ctx context.Context, genesisTime time.Time, db db.HeadAccessDatabase) {
	currentTime := prysmTime.Now()
	if currentTime.After(genesisTime) {
		log.WithFields(logrus.Fields{
			"genesisTime": genesisTime.Local(),
			"currentTime": currentTime.Local(),
		}).Warn("⏳ The genesis time is expired")
		return
	}

	gState, err := db.GenesisState(ctx)
	if err != nil {
		log.Fatalf("Could not retrieve genesis state: %v", err)
	}
	gRoot, err := gState.HashTreeRoot(ctx)
	if err != nil {
		log.Fatalf("Could not hash tree root genesis state: %v", err)
	}
	go slots.CountdownToGenesis(ctx, genesisTime, uint64(gState.NumValidators()), gRoot)
}

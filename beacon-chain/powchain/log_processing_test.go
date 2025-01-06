package powchain

import (
	"context"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache/depositcache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db"
	testDB "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/voluntaryexits"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/withdrawals"
	mockPOW "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/powchain/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	ethereum "gitlab.waterfall.network/waterfall/protocol/gwat"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

func TestProcessETH2GenesisLog_8DuplicatePubkeys(t *testing.T) {
	hook := logTest.NewGlobal()
	testAcc, err := mockPOW.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := testDB.SetupDB(t)
	depositCache, err := depositcache.New()
	require.NoError(t, err)
	server, endpoint, err := mockPOW.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})

	web3Service, err := NewService(context.Background(),
		WithHttpEndpoints([]string{endpoint}),
		WithDatabase(beaconDB),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)

	params.SetupTestConfigCleanup(t)
	bConfig := params.MinimalSpecConfig()
	bConfig.MinGenesisTime = 0
	params.OverrideBeaconConfig(bConfig)

	testAcc.Backend.Commit()
	require.NoError(t, testAcc.Backend.AdjustTime(time.Duration(int64(time.Now().Nanosecond()))))

	testAcc.TxOpts.Value = mockPOW.Amount3200Wat()
	testAcc.TxOpts.GasLimit = 1000000

	// 64 Validators are used as size required for beacon-chain to start. This number
	// is defined in the deposit contract as the number required for the testnet. The actual number
	// is 2**14

	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			web3Service.cfg.depositContractAddr,
		},
	}

	logs, err := testAcc.Backend.FilterLogs(web3Service.ctx, query)
	require.NoError(t, err, "Unable to retrieve logs")

	for _, log := range logs {
		err = web3Service.ProcessLog(context.Background(), log)
		require.NoError(t, err)
	}
	assert.Equal(t, false, web3Service.chainStartData.Chainstarted, "Genesis has been triggered despite being 8 duplicate keys")

	require.LogsDoNotContain(t, hook, "Minimum number of validators reached for beacon-chain to start")
	hook.Reset()
}

func TestProcessETH2GenesisLog_CorrectNumOfDeposits(t *testing.T) {
	hook := logTest.NewGlobal()
	testAcc, err := mockPOW.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	kvStore := testDB.SetupDB(t)
	depositCache, err := depositcache.New()
	require.NoError(t, err)
	server, endpoint, err := mockPOW.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})

	web3Service, err := NewService(context.Background(),
		WithHttpEndpoints([]string{endpoint}),
		WithDatabase(kvStore),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.rpcClient = &mockPOW.RPCClient{Backend: testAcc.Backend}
	web3Service.httpLogger = testAcc.Backend
	web3Service.eth1DataFetcher = &goodFetcher{backend: testAcc.Backend}
	web3Service.latestEth1Data.LastRequestedBlock = 0
	web3Service.latestEth1Data.BlockHeight = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Nr()
	web3Service.latestEth1Data.BlockTime = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Time()
	params.SetupTestConfigCleanup(t)
	bConfig := params.MinimalSpecConfig()
	bConfig.MinGenesisTime = 0
	bConfig.SecondsPerETH1Block = 10
	params.OverrideBeaconConfig(bConfig)
	nConfig := params.BeaconNetworkConfig()
	params.OverrideBeaconNetworkConfig(nConfig)

	testAcc.Backend.Commit()

	totalNumOfDeposits := depositsReqForChainStart + 30

	depositOffset := 5

	// 64 Validators are used as size required for beacon-chain to start. This number
	// is defined in the deposit contract as the number required for the testnet. The actual number
	// is 2**14
	for i := 0; i < totalNumOfDeposits; i++ {
		testAcc.TxOpts.Value = mockPOW.Amount3200Wat()
		testAcc.TxOpts.GasLimit = 1000000
		if (i+1)%8 == depositOffset {
			testAcc.Backend.Commit()
		}
	}
	// Forward the chain to account for the follow distance
	for i := uint64(0); i < params.BeaconConfig().Eth1FollowDistance; i++ {
		testAcc.Backend.Commit()
	}
	web3Service.latestEth1Data.BlockHeight = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Nr()
	web3Service.latestEth1Data.BlockTime = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Time()

	// Set up our subscriber now to listen for the chain started event.
	stateChannel := make(chan *feed.Event, 1)
	stateSub := web3Service.cfg.stateNotifier.StateFeed().Subscribe(stateChannel)
	defer stateSub.Unsubscribe()

	cachedDeposits := web3Service.chainStartData.ChainstartDeposits
	require.Equal(t, 0, len(cachedDeposits), "Did not cache the chain start deposits correctly")

	// Receive the chain started event.

	require.LogsDoNotContain(t, hook, "Unable to unpack ChainStart log data")
	require.LogsDoNotContain(t, hook, "Receipt root from log doesn't match the root saved in memory")
	require.LogsDoNotContain(t, hook, "Invalid timestamp from log")

	hook.Reset()
}

func TestProcessETH2GenesisLog_LargePeriodOfNoLogs(t *testing.T) {
	hook := logTest.NewGlobal()
	testAcc, err := mockPOW.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	kvStore := testDB.SetupDB(t)
	depositCache, err := depositcache.New()
	require.NoError(t, err)
	server, endpoint, err := mockPOW.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})

	web3Service, err := NewService(context.Background(),
		WithHttpEndpoints([]string{endpoint}),
		WithDatabase(kvStore),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.rpcClient = &mockPOW.RPCClient{Backend: testAcc.Backend}
	web3Service.httpLogger = testAcc.Backend
	web3Service.eth1DataFetcher = &goodFetcher{backend: testAcc.Backend}
	web3Service.latestEth1Data.LastRequestedBlock = 0
	web3Service.latestEth1Data.BlockHeight = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Nr()
	web3Service.latestEth1Data.BlockTime = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Time()
	params.SetupTestConfigCleanup(t)
	bConfig := params.MinimalSpecConfig()
	bConfig.SecondsPerETH1Block = 10
	params.OverrideBeaconConfig(bConfig)
	nConfig := params.BeaconNetworkConfig()
	params.OverrideBeaconNetworkConfig(nConfig)

	testAcc.Backend.Commit()

	totalNumOfDeposits := depositsReqForChainStart + 30

	depositOffset := 5

	// 64 Validators are used as size required for beacon-chain to start. This number
	// is defined in the deposit contract as the number required for the testnet. The actual number
	// is 2**14
	for i := 0; i < totalNumOfDeposits; i++ {
		testAcc.TxOpts.Value = mockPOW.Amount3200Wat()
		testAcc.TxOpts.GasLimit = 1000000

		if (i+1)%8 == depositOffset {
			testAcc.Backend.Commit()
		}
	}
	// Forward the chain to 'mine' blocks without logs
	for i := uint64(0); i < 1500; i++ {
		testAcc.Backend.Commit()
	}
	wantedGenesisTime := testAcc.Backend.Blockchain().GetLastFinalizedBlock().Time()

	// Forward the chain to account for the follow distance
	for i := uint64(0); i < params.BeaconConfig().Eth1FollowDistance; i++ {
		testAcc.Backend.Commit()
	}
	web3Service.latestEth1Data.BlockHeight = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Nr()
	web3Service.latestEth1Data.BlockTime = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Time()

	// Set the genesis time 500 blocks ahead of the last
	// deposit log.
	bConfig = params.MinimalSpecConfig()
	bConfig.MinGenesisTime = wantedGenesisTime - 10
	params.OverrideBeaconConfig(bConfig)

	// Set up our subscriber now to listen for the chain started event.
	stateChannel := make(chan *feed.Event, 1)
	stateSub := web3Service.cfg.stateNotifier.StateFeed().Subscribe(stateChannel)
	defer stateSub.Unsubscribe()

	cachedDeposits := web3Service.chainStartData.ChainstartDeposits
	require.Equal(t, 0, len(cachedDeposits), "Did not cache the chain start deposits correctly")

	require.LogsDoNotContain(t, hook, "Unable to unpack ChainStart log data")
	require.LogsDoNotContain(t, hook, "Receipt root from log doesn't match the root saved in memory")
	require.LogsDoNotContain(t, hook, "Invalid timestamp from log")

	hook.Reset()
}

func TestRestoreValidatorOpPools(t *testing.T) {
	hook := logTest.NewGlobal()
	testAcc, err := mockPOW.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	kvStore := testDB.SetupDB(t)
	depositCache, err := depositcache.New()
	require.NoError(t, err)
	server, endpoint, err := mockPOW.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})

	withdravalOps := []*ethpb.Withdrawal{
		{
			PublicKey:      common.BlsPubKey{0x01, 0x01, 0x01, 0x01, 0x01}.Bytes(),
			ValidatorIndex: 100,
			Amount:         100000000,
			InitTxHash:     common.Hash{0x01, 0x01, 0x01, 0x01, 0x01}.Bytes(),
			Epoch:          1000,
		},
		{
			PublicKey:      common.BlsPubKey{0x02, 0x02, 0x02, 0x02, 0x02}.Bytes(),
			ValidatorIndex: 200,
			Amount:         200000000,
			InitTxHash:     common.Hash{0x02, 0x02, 0x02, 0x02, 0x02}.Bytes(),
			Epoch:          2000,
		},
		{
			PublicKey:      common.BlsPubKey{0x03, 0x03, 0x03, 0x03, 0x03}.Bytes(),
			ValidatorIndex: 300,
			Amount:         300000000,
			InitTxHash:     common.Hash{0x03, 0x03, 0x03, 0x03, 0x03}.Bytes(),
			Epoch:          3000,
		},
	}
	err = kvStore.WriteWithdrawalPool(context.Background(), withdravalOps)
	require.NoError(t, err)

	exitOps := []*ethpb.VoluntaryExit{
		{
			Epoch:          1000,
			ValidatorIndex: 100,
			InitTxHash:     common.Hash{0x01, 0x01, 0x01, 0x01, 0x01}.Bytes(),
		},
		{
			ValidatorIndex: 200,
			InitTxHash:     common.Hash{0x02, 0x02, 0x02, 0x02, 0x02}.Bytes(),
			Epoch:          2000,
		},
		{
			ValidatorIndex: 300,
			InitTxHash:     common.Hash{0x03, 0x03, 0x03, 0x03, 0x03}.Bytes(),
			Epoch:          3000,
		},
	}
	err = kvStore.WriteExitPool(context.Background(), exitOps)
	require.NoError(t, err)
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoints([]string{endpoint}),
		WithDatabase(kvStore),
		WithDepositCache(depositCache),
		WithWithdrawalPool(withdrawals.NewPool()),
		WithExitPool(voluntaryexits.NewPool()),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.rpcClient = &mockPOW.RPCClient{Backend: testAcc.Backend}
	web3Service.httpLogger = testAcc.Backend
	web3Service.eth1DataFetcher = &goodFetcher{backend: testAcc.Backend}
	web3Service.latestEth1Data.LastRequestedBlock = 0
	web3Service.latestEth1Data.BlockHeight = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Nr()
	web3Service.latestEth1Data.BlockTime = testAcc.Backend.Blockchain().GetLastFinalizedBlock().Time()
	params.SetupTestConfigCleanup(t)
	bConfig := params.MinimalSpecConfig()
	bConfig.SecondsPerETH1Block = 10
	params.OverrideBeaconConfig(bConfig)
	nConfig := params.BeaconNetworkConfig()
	params.OverrideBeaconNetworkConfig(nConfig)
	testAcc.Backend.Commit()

	resWithdravalOps := web3Service.cfg.withdrawalPool.CopyItems()
	require.DeepEqual(t, withdravalOps, resWithdravalOps, "Restore witdrawal pool failed")

	resExitOps := web3Service.cfg.exitPool.CopyItems()
	require.DeepEqual(t, exitOps, resExitOps, "Restore exit pool failed")

	hook.Reset()
}

func TestCheckForChainstart_NoValidator(t *testing.T) {
	hook := logTest.NewGlobal()
	testAcc, err := mockPOW.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := testDB.SetupDB(t)
	s := newPowchainService(t, testAcc, beaconDB)
	s.checkForChainstart(context.Background(), [32]byte{}, 0, 0)
	require.LogsDoNotContain(t, hook, "Could not determine active validator count from pre genesis state")
}

func newPowchainService(t *testing.T, eth1Backend *mockPOW.TestAccount, beaconDB db.Database) *Service {
	depositCache, err := depositcache.New()
	require.NoError(t, err)
	server, endpoint, err := mockPOW.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoints([]string{endpoint}),
		WithDatabase(beaconDB),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)

	web3Service.rpcClient = &mockPOW.RPCClient{Backend: eth1Backend.Backend}
	web3Service.eth1DataFetcher = &goodFetcher{backend: eth1Backend.Backend}
	web3Service.httpLogger = &goodLogger{backend: eth1Backend.Backend}
	params.SetupTestConfigCleanup(t)
	bConfig := params.MinimalSpecConfig()
	bConfig.MinGenesisTime = 0
	params.OverrideBeaconConfig(bConfig)
	return web3Service
}

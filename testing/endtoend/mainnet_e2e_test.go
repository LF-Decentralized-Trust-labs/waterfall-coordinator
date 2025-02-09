package endtoend

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ev "gitlab.waterfall.network/waterfall/protocol/coordinator/testing/endtoend/evaluators"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/endtoend/helpers"
	e2eParams "gitlab.waterfall.network/waterfall/protocol/coordinator/testing/endtoend/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/endtoend/types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestEndToEnd_MainnetConfig(t *testing.T) {
	e2eMainnet(t, false /*usePrysmSh*/)
}

func e2eMainnet(t *testing.T, usePrysmSh bool) {
	params.UseE2EMainnetConfig()
	require.NoError(t, e2eParams.InitMultiClient(e2eParams.StandardBeaconCount, e2eParams.StandardLighthouseNodeCount))

	// Run for 10 epochs if not in long-running to confirm long-running has no issues.
	var err error
	epochsToRun := 10
	epochStr, longRunning := os.LookupEnv("E2E_EPOCHS")
	if longRunning {
		epochsToRun, err = strconv.Atoi(epochStr)
		require.NoError(t, err)
	}
	_, crossClient := os.LookupEnv("RUN_CROSS_CLIENT")
	if usePrysmSh {
		// If using prysm.sh, run for only 6 epochs.
		// TODO(#9166): remove this block once v2 changes are live.
		epochsToRun = helpers.AltairE2EForkEpoch - 1
	}
	seed := 0
	seedStr, isValid := os.LookupEnv("E2E_SEED")
	if isValid {
		seed, err = strconv.Atoi(seedStr)
		require.NoError(t, err)
	}
	tracingPort := e2eParams.TestParams.Ports.JaegerTracingPort
	tracingEndpoint := fmt.Sprintf("127.0.0.1:%d", tracingPort)
	evals := []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsAreActive,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.ProposeVoluntaryExit,
		ev.ValidatorHasExited,
		ev.ColdStateCheckpoint,
		ev.AltairForkTransition,
		ev.BellatrixForkTransition,
		ev.APIMiddlewareVerifyIntegrity,
		ev.APIGatewayV1Alpha1VerifyIntegrity,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
		ev.TransactionsPresent,
	}
	testConfig := &types.E2EConfig{
		BeaconFlags: []string{
			fmt.Sprintf("--slots-per-archive-point=%d", params.BeaconConfig().SlotsPerEpoch*16),
			fmt.Sprintf("--tracing-endpoint=http://%s", tracingEndpoint),
			"--enable-tracing",
			"--trace-sample-fraction=1.0",
		},
		ValidatorFlags:          []string{},
		EpochsToRun:             uint64(epochsToRun),
		TestSync:                true,
		TestFeature:             true,
		TestDeposits:            true,
		UseFixedPeerIDs:         true,
		UseValidatorCrossClient: crossClient,
		UsePrysmShValidator:     usePrysmSh,
		UsePprof:                !longRunning,
		TracingSinkEndpoint:     tracingEndpoint,
		Evaluators:              evals,
		Seed:                    int64(seed),
	}

	newTestRunner(t, testConfig).run()
}

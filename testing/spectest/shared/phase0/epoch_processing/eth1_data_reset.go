package epoch_processing

import (
	"path"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/epoch"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/spectest/utils"
)

// RunEth1DataResetTests executes "epoch_processing/eth1_data_reset" tests.
func RunEth1DataResetTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "phase0", "epoch_processing/eth1_data_reset/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processEth1DataResetWrapper)
		})
	}
}

func processEth1DataResetWrapper(t *testing.T, state state.BeaconState) (state.BeaconState, error) {
	state, err := epoch.ProcessEth1DataReset(state)
	require.NoError(t, err, "Could not process final updates")
	return state, nil
}

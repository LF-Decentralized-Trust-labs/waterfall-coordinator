package epoch_processing

import (
	"path"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/altair"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/spectest/utils"
)

// RunParticipationFlagUpdatesTests executes "epoch_processing/participation_flag_updates" tests.
func RunParticipationFlagUpdatesTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "altair", "epoch_processing/participation_flag_updates/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processParticipationFlagUpdatesWrapper)
		})
	}
}

func processParticipationFlagUpdatesWrapper(t *testing.T, state state.BeaconState) (state.BeaconState, error) {
	state, err := altair.ProcessParticipationFlagUpdates(state)
	require.NoError(t, err, "Could not process participation flag update")
	return state, nil
}

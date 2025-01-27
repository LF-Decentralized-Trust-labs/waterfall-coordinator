// Package shuffle contains all conformity specification tests
// for validator shuffling logic according to the Ethereum Beacon Node spec (Modified for Waterfall).
package shuffle

import (
	"encoding/hex"
	"path"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/spectest/utils"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"gopkg.in/yaml.v2"
)

// RunShuffleTests executes "shuffling/core/shuffle" tests.
func RunShuffleTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "phase0", "shuffling/core/shuffle")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			testCaseFile, err := util.BazelFileBytes(path.Join(testsFolderPath, folder.Name(), "mapping.yaml"))
			require.NoError(t, err, "Could not read YAML tests directory")

			testCase := &ShuffleTestCase{}
			require.NoError(t, yaml.Unmarshal(testCaseFile, testCase), "Could not unmarshal YAML file into test struct")
			require.NoError(t, runShuffleTest(t, testCase), "Shuffle test failed")
		})
	}
}

// RunShuffleTest uses validator set specified from a YAML file, runs the validator shuffle
// algorithm, then compare the output with the expected output from the YAML file.
func runShuffleTest(t *testing.T, testCase *ShuffleTestCase) error {
	baseSeed, err := hex.DecodeString(testCase.Seed[2:])
	if err != nil {
		return err
	}

	seed := common.BytesToHash(baseSeed)
	testIndices := make([]types.ValidatorIndex, testCase.Count)
	for i := types.ValidatorIndex(0); uint64(i) < testCase.Count; i++ {
		testIndices[i] = i
	}
	shuffledList := make([]types.ValidatorIndex, testCase.Count)
	for i := types.ValidatorIndex(0); uint64(i) < testCase.Count; i++ {
		si, err := helpers.ShuffledIndex(i, testCase.Count, seed)
		if err != nil {
			return err
		}
		shuffledList[i] = si
	}
	require.DeepSSZEqual(t, shuffledList, testCase.Mapping)
	return nil
}

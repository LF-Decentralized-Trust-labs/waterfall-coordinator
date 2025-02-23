package derived

import (
	"testing"

	"github.com/tyler-smith/go-bip39"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestMnemonic_Generate_CanRecover(t *testing.T) {
	generator := &EnglishMnemonicGenerator{}
	data := make([]byte, 32)
	copy(data, "hello-world")
	phrase, err := generator.Generate(data)
	require.NoError(t, err)
	entropy, err := bip39.EntropyFromMnemonic(phrase)
	require.NoError(t, err)
	assert.DeepEqual(t, data, entropy, "Expected to recover original data")
}

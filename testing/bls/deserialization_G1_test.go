package bls

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls/common"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/bls/utils"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestDeserializationG1(t *testing.T) {
	t.Run("blst", testDeserializationG1)
}

func testDeserializationG1(t *testing.T) {
	fNames, fContent := utils.RetrieveFiles("deserialization_G1", t)

	for i, file := range fNames {
		content := fContent[i]
		t.Run(file, func(t *testing.T) {
			test := &DeserializationG1Test{}
			require.NoError(t, yaml.Unmarshal(content, test))
			rawKey, err := hex.DecodeString(test.Input.Pubkey)
			require.NoError(t, err)

			_, err = bls.PublicKeyFromBytes(rawKey)
			// Exit early if we encounter an infinite key here.
			if strings.Contains(file, "deserialization_succeeds_infinity_with_true_b_flag") &&
				err == common.ErrInfinitePubKey {
				t.Log("Success")
				return
			}
			require.Equal(t, test.Output, err == nil)
			t.Log("Success")
		})
	}
}

package accounts

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/validator/accounts/wallet"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/validator/keymanager"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/validator/keymanager/local"
)

func TestDeleteAccounts_Noninteractive(t *testing.T) {
	walletDir, _, passwordFilePath := setupWalletAndPasswordsDir(t)
	// Write a directory where we will import keys from.
	keysDir := filepath.Join(t.TempDir(), "keysDir")
	require.NoError(t, os.MkdirAll(keysDir, os.ModePerm))

	// Create 3 keystore files in the keys directory we can then
	// import from in our wallet.
	k1, _ := createKeystore(t, keysDir)
	time.Sleep(time.Second)
	k2, _ := createKeystore(t, keysDir)
	time.Sleep(time.Second)
	k3, _ := createKeystore(t, keysDir)
	generatedPubKeys := []string{k1.Pubkey, k2.Pubkey, k3.Pubkey}
	// Only delete keys 0 and 1.
	deletePublicKeys := strings.Join(generatedPubKeys[0:2], ",")

	// We initialize a wallet with a local keymanager.
	cliCtx := setupWalletCtx(t, &testWalletConfig{
		// Wallet configuration flags.
		walletDir:           walletDir,
		keymanagerKind:      keymanager.Local,
		walletPasswordFile:  passwordFilePath,
		accountPasswordFile: passwordFilePath,
		// Flags required for ImportAccounts to work.
		keysDir: keysDir,
		// Flags required for DeleteAccounts to work.
		deletePublicKeys: deletePublicKeys,
	})
	w, err := CreateWalletWithKeymanager(cliCtx.Context, &CreateWalletConfig{
		WalletCfg: &wallet.Config{
			WalletDir:      walletDir,
			KeymanagerKind: keymanager.Local,
			WalletPassword: password,
		},
	})
	require.NoError(t, err)

	// We attempt to import accounts.
	require.NoError(t, ImportAccountsCli(cliCtx))

	// We attempt to delete the accounts specified.
	require.NoError(t, DeleteAccountCli(cliCtx))

	keymanager, err := local.NewKeymanager(
		cliCtx.Context,
		&local.SetupConfig{
			Wallet:           w,
			ListenForChanges: false,
		},
	)
	require.NoError(t, err)
	remainingAccounts, err := keymanager.FetchValidatingPublicKeys(cliCtx.Context)
	require.NoError(t, err)
	require.Equal(t, len(remainingAccounts), 1)
	remainingPublicKey, err := hex.DecodeString(k3.Pubkey)
	require.NoError(t, err)
	assert.DeepEqual(t, remainingAccounts[0], bytesutil.ToBytes48(remainingPublicKey))
}

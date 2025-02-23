package db

import (
	"context"
	"flag"
	"io/ioutil"
	"os"
	"path"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db/kv"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/cmd"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
)

func TestRestore(t *testing.T) {
	logHook := logTest.NewGlobal()
	ctx := context.Background()

	backupDb, err := kv.NewKVStore(context.Background(), t.TempDir(), &kv.Config{})
	require.NoError(t, err)
	head := util.NewBeaconBlock()
	head.Block.Slot = 5000
	wsb, err := wrapper.WrappedSignedBeaconBlock(head)
	require.NoError(t, err)
	require.NoError(t, backupDb.SaveBlock(ctx, wsb))
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, backupDb.SaveState(ctx, st, root))
	require.NoError(t, backupDb.SaveHeadBlockRoot(ctx, root))
	require.NoError(t, err)
	require.NoError(t, backupDb.Close())
	// We rename the backup file so that we can later verify
	// whether the restored db has been renamed correctly.
	require.NoError(t, os.Rename(
		path.Join(backupDb.DatabasePath(), kv.DatabaseFileName),
		path.Join(backupDb.DatabasePath(), "backup.db")))

	restoreDir := t.TempDir()
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.RestoreSourceFileFlag.Name, "", "")
	set.String(cmd.RestoreTargetDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.RestoreSourceFileFlag.Name, path.Join(backupDb.DatabasePath(), "backup.db")))
	require.NoError(t, set.Set(cmd.RestoreTargetDirFlag.Name, restoreDir))
	cliCtx := cli.NewContext(&app, set, nil)

	assert.NoError(t, Restore(cliCtx))

	files, err := ioutil.ReadDir(path.Join(restoreDir, kv.BeaconNodeDbDirName))
	require.NoError(t, err)
	assert.Equal(t, 1, len(files))
	assert.Equal(t, kv.DatabaseFileName, files[0].Name())
	restoredDb, err := kv.NewKVStore(context.Background(), path.Join(restoreDir, kv.BeaconNodeDbDirName), &kv.Config{})
	defer func() {
		require.NoError(t, restoredDb.Close())
	}()
	require.NoError(t, err)
	headBlock, err := restoredDb.HeadBlock(ctx)
	require.NoError(t, err)
	assert.Equal(t, types.Slot(5000), headBlock.Block().Slot(), "Restored database has incorrect data")
	assert.LogsContain(t, logHook, "Restore completed successfully")

}

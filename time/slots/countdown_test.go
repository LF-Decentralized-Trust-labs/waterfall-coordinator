package slots

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	prysmTime "gitlab.waterfall.network/waterfall/protocol/coordinator/time"
)

func TestCountdownToGenesis(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	hook := logTest.NewGlobal()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.GenesisCountdownInterval = time.Millisecond * 500
	params.OverrideBeaconConfig(config)

	t.Run("normal countdown", func(t *testing.T) {
		defer hook.Reset()
		firstStringResult := "1s until chain genesis"
		genesisReached := "Chain genesis time reached"
		CountdownToGenesis(
			context.Background(),
			prysmTime.Now().Add(2*time.Second),
			params.BeaconConfig().MinGenesisActiveValidatorCount,
			[32]byte{},
		)
		require.LogsContain(t, hook, firstStringResult)
		require.LogsContain(t, hook, genesisReached)
	})

	t.Run("close context", func(t *testing.T) {
		defer hook.Reset()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.AfterFunc(1500*time.Millisecond, func() {
				cancel()
			})
		}()
		CountdownToGenesis(
			ctx,
			prysmTime.Now().Add(5*time.Second),
			params.BeaconConfig().MinGenesisActiveValidatorCount,
			[32]byte{},
		)
		require.LogsContain(t, hook, "4s until chain genesis")
		require.LogsContain(t, hook, "3s until chain genesis")
		require.LogsContain(t, hook, "Context closed, exiting routine")
		require.LogsDoNotContain(t, hook, "Chain genesis time reached")
	})
}

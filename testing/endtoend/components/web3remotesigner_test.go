package components_test

import (
	"context"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/endtoend/components"
	e2eparams "gitlab.waterfall.network/waterfall/protocol/coordinator/testing/endtoend/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestWeb3RemoteSigner_StartsAndReturnsPublicKeys(t *testing.T) {
	t.Skip() // WebRemoteSigner method Start is unused.
	require.NoError(t, e2eparams.Init(0))
	fp, err := bazel.Runfile("config/params/testdata/e2e_config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	wsc := components.NewWeb3RemoteSigner(fp)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := wsc.Start(ctx); err != nil {
			t.Error(err)
			panic(err)
		}
	}()

	select {
	case <-ctx.Done():
		t.Fatal("Web3RemoteSigner did not start within timeout")
	case <-wsc.Started():
		t.Log("Web3RemoteSigner started")
		break
	}

	time.Sleep(10 * time.Second)

	keys, err := wsc.PublicKeys(ctx)
	require.NoError(t, err)

	if uint64(len(keys)) != params.BeaconConfig().MinGenesisActiveValidatorCount {
		t.Fatalf("Expected %d keys, got %d", params.BeaconConfig().MinGenesisActiveValidatorCount, len(keys))
	}
}

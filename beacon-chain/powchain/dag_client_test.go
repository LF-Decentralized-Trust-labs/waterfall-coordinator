//Copyright 2024   Blue Wave Inc.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package powchain

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mocks "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/powchain/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
	gwatTypes "gitlab.waterfall.network/waterfall/protocol/gwat/core/types"
	"gitlab.waterfall.network/waterfall/protocol/gwat/rpc"
)

var (
	_ = EngineCaller(&Service{})
	_ = EngineCaller(&mocks.EngineClient{})
)

func TestDagClient_IPC(t *testing.T) {
	server := newTestDagIPCServer(t)
	defer server.Stop()
	rpcClient := rpc.DialInProc(server)
	defer rpcClient.Close()
	srv := &Service{}
	srv.rpcClient = rpcClient
	ctx := context.Background()
	fix := dagFixtures()

	t.Run(ExecutionDagGetOptimisticSpines, func(t *testing.T) {
		want, ok := fix["ExecutionOptimisticSpines"].(*gwatTypes.OptimisticSpinesResult)
		require.Equal(t, true, ok)
		resp, err := srv.ExecutionDagGetOptimisticSpines(ctx, gwatCommon.Hash{})
		require.NoError(t, err)
		require.DeepEqual(t, want.Data, resp)
	})
	t.Run(ExecutionDagGetCandidatesMethod, func(t *testing.T) {
		want, ok := fix["ExecutionCandidates"].(*gwatTypes.CandidatesResult)
		require.Equal(t, true, ok)
		resp, err := srv.ExecutionDagGetCandidates(ctx, 1000)
		require.NoError(t, err)
		require.DeepEqual(t, want.Candidates, resp)
	})
	t.Run(ExecutionDagFinalizeMethod, func(t *testing.T) {
		want, ok := fix["ExecutionFinalize"].(*gwatTypes.FinalizationResult)
		require.Equal(t, true, ok)

		baseSpine := common.HexToHash("0x351cd65f6e74ff61322d16c4a808bdce69c30410b3965fbbf188c46fa44da545")
		hash_1 := common.HexToHash("0xa659fcd4ed3f3ad9cd43ab36eb29080a4655328fe16f045962afab1d66a5da09")
		params := &gwatTypes.FinalizationParams{
			Spines:      gwatCommon.HashArray{hash_1},
			BaseSpine:   &baseSpine,
			Checkpoint:  nil,
			ValSyncData: nil,
		}
		res, err := srv.ExecutionDagFinalize(ctx, params)
		require.DeepEqual(t, hash_1.Hex(), res.LFSpine.Hex())
		require.ErrorContains(t, *want.Error, err)
	})
	t.Run(ExecutionDagCoordinatedStateMethod, func(t *testing.T) {
		want, ok := fix["ExecutionFinalize"].(*gwatTypes.FinalizationResult)
		require.Equal(t, true, ok)
		hash_1 := common.HexToHash("0xa659fcd4ed3f3ad9cd43ab36eb29080a4655328fe16f045962afab1d66a5da09")
		res, err := srv.ExecutionDagCoordinatedState(ctx)
		require.DeepEqual(t, hash_1.Hex(), res.LFSpine.Hex())
		require.ErrorContains(t, *want.Error, err)
	})
}

func TestDagClient_HTTP(t *testing.T) {
	ctx := context.Background()
	fix := dagFixtures()

	t.Run(ExecutionDagGetOptimisticSpines, func(t *testing.T) {
		want, ok := fix["ExecutionOptimisticSpines"].(*gwatTypes.OptimisticSpinesResult)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			_, err := ioutil.ReadAll(r.Body)
			require.NoError(t, err)
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := service.ExecutionDagGetOptimisticSpines(ctx, gwatCommon.Hash{})
		require.NoError(t, err)
		require.DeepEqual(t, want.Data, resp)
	})
	t.Run(ExecutionDagGetCandidatesMethod, func(t *testing.T) {
		want, ok := fix["ExecutionCandidates"].(*gwatTypes.CandidatesResult)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			_, err := ioutil.ReadAll(r.Body)
			require.NoError(t, err)
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := service.ExecutionDagGetCandidates(ctx, 1000)
		require.NoError(t, err)
		require.DeepEqual(t, want.Candidates, resp)
	})
	t.Run(ExecutionDagFinalizeMethod, func(t *testing.T) {
		baseSpine := common.HexToHash("0x351cd65f6e74ff61322d16c4a808bdce69c30410b3965fbbf188c46fa44da545")
		hash_1 := common.HexToHash("0xa659fcd4ed3f3ad9cd43ab36eb29080a4655328fe16f045962afab1d66a5da09")
		spines := gwatCommon.HashArray{hash_1}
		want, ok := fix["ExecutionFinalize"].(*gwatTypes.FinalizationResult)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := ioutil.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)
			// We expect the JSON string RPC request contains the right arguments.
			//sArgs, _ := arg.MarshalJSON()
			sArgs := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"dag_finalize\",\"params\":[" +
				"{\"spines\":[\"0xa659fcd4ed3f3ad9cd43ab36eb29080a4655328fe16f045962afab1d66a5da09\"]," +
				"\"baseSpine\":\"0x351cd65f6e74ff61322d16c4a808bdce69c30410b3965fbbf188c46fa44da545\"," +
				"\"checkpoint\":null,\"valSyncData\":null}]}"
			//t.Logf("=========== %v", jsonRequestString)
			//t.Logf("=========== %v", fmt.Sprintf("%s", sArgs))
			require.Equal(t, false, strings.Contains(
				jsonRequestString, fmt.Sprintf("%v", fmt.Sprintf("%s", sArgs)),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}

			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		params := &gwatTypes.FinalizationParams{
			Spines:      spines,
			BaseSpine:   &baseSpine,
			Checkpoint:  nil,
			ValSyncData: nil,
		}

		// We call the RPC method via HTTP and expect a proper result.
		res, err := service.ExecutionDagFinalize(ctx, params)
		require.ErrorContains(t, *want.Error, err)
		require.DeepEqual(t, hash_1.Hex(), res.LFSpine.Hex())
	})
	t.Run(ExecutionDagCoordinatedStateMethod, func(t *testing.T) {
		hash_1 := common.HexToHash("0xa659fcd4ed3f3ad9cd43ab36eb29080a4655328fe16f045962afab1d66a5da09")
		want, ok := fix["ExecutionFinalize"].(*gwatTypes.FinalizationResult)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := ioutil.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)
			// We expect the JSON string RPC request contains the right arguments.
			//sArgs, _ := arg.MarshalJSON()
			sArgs := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"dag_coordinatedState\"}"
			//t.Logf("=========== %v", jsonRequestString)
			//t.Logf("=========== %v", fmt.Sprintf("%s", sArgs))
			require.Equal(t, true, strings.Contains(
				jsonRequestString, fmt.Sprintf("%v", fmt.Sprintf("%s", sArgs)),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		res, err := service.ExecutionDagCoordinatedState(ctx)
		require.ErrorContains(t, *want.Error, err)
		require.DeepEqual(t, hash_1.Hex(), res.LFSpine.Hex())
	})
}

func newTestDagIPCServer(t *testing.T) *rpc.Server {
	server := rpc.NewServer()
	err := server.RegisterName("dag", new(testDagEngineService))
	require.NoError(t, err)
	return server
}

func dagFixtures() map[string]interface{} {
	hash_1 := common.HexToHash("0xa659fcd4ed3f3ad9cd43ab36eb29080a4655328fe16f045962afab1d66a5da09")
	executionCandidates := &gwatTypes.CandidatesResult{
		Error:      nil,
		Candidates: gwatCommon.HashArray{hash_1},
	}

	executionOptimisticSpines := &gwatTypes.OptimisticSpinesResult{
		Error: nil,
		Data:  []gwatCommon.HashArray{gwatCommon.HashArray{hash_1, hash_1}, gwatCommon.HashArray{hash_1, hash_1}},
	}

	finErr := "Post \"http://127.0.0.1:51304\": EOF"
	executionFinalize := &gwatTypes.FinalizationResult{
		Error:   &finErr,
		LFSpine: &hash_1,
		CpEpoch: nil,
		CpRoot:  nil,
	}

	return map[string]interface{}{
		"ExecutionFinalize":         executionFinalize,
		"ExecutionCandidates":       executionCandidates,
		"ExecutionOptimisticSpines": executionOptimisticSpines,
	}
}

type testDagEngineService struct{}

func (*testDagEngineService) NoArgsRets() {}

func (*testDagEngineService) GetOptimisticSpines(
	_ context.Context, fromSpine gwatCommon.Hash,
) *gwatTypes.OptimisticSpinesResult {
	fix := dagFixtures()
	item, ok := fix["ExecutionOptimisticSpines"].(*gwatTypes.OptimisticSpinesResult)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testDagEngineService) GetCandidates(
	_ context.Context, slot uint64,
) *gwatTypes.CandidatesResult {
	fix := dagFixtures()
	item, ok := fix["ExecutionCandidates"].(*gwatTypes.CandidatesResult)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testDagEngineService) Finalize(
	_ context.Context, _ *gwatTypes.FinalizationParams,
) *gwatTypes.FinalizationResult {
	fix := dagFixtures()
	item, ok := fix["ExecutionFinalize"].(*gwatTypes.FinalizationResult)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testDagEngineService) CoordinatedState(
	_ context.Context,
) *gwatTypes.FinalizationResult {
	fix := dagFixtures()
	item, ok := fix["ExecutionFinalize"].(*gwatTypes.FinalizationResult)
	if !ok {
		panic("not found")
	}
	return item
}

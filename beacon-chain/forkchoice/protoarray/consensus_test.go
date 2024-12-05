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

package protoarray

import (
	"context"
	"fmt"
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

func TestStore_GetFork(t *testing.T) {
	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		[32]byte{'A'}: 0,
		[32]byte{'B'}: 1,
		[32]byte{'C'}: 2,
		[32]byte{'D'}: 3,
		[32]byte{'E'}: 4,
		[32]byte{'F'}: 5,
		[32]byte{'G'}: 6,
		[32]byte{'H'}: 7,
		[32]byte{'I'}: 8,
		[32]byte{'J'}: 9,
		[32]byte{'K'}: 10,
	}
	f.store.nodes = []*Node{
		//fork 0
		{slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
		{slot: 2, root: [32]byte{'B'}, parent: 0},
		{slot: 3, root: [32]byte{'C'}, parent: 1},
		{slot: 4, root: [32]byte{'D'}, parent: 2},
		{slot: 5, root: [32]byte{'E'}, parent: 3},
		{slot: 6, root: [32]byte{'F'}, parent: 4},
		//fork 1
		{slot: 7, root: [32]byte{'G'}, parent: 2},
		{slot: 8, root: [32]byte{'H'}, parent: 6},
		//fork 2
		{slot: 9, root: [32]byte{'I'}, parent: 3},
		{slot: 10, root: [32]byte{'J'}, parent: 8},
		{slot: 11, root: [32]byte{'K'}, parent: 9},
	}
	want := &Fork{
		roots: [][32]byte{
			{'H'},
			{'G'},
			{'C'},
			{'B'},
			{'A'},
		},
		nodesMap: map[[32]byte]*Node{
			[32]byte{'H'}: {slot: 8, root: [32]byte{'H'}, parent: 6},
			[32]byte{'G'}: {slot: 7, root: [32]byte{'G'}, parent: 2},
			[32]byte{'C'}: {slot: 3, root: [32]byte{'C'}, parent: 1},
			[32]byte{'B'}: {slot: 2, root: [32]byte{'B'}, parent: 0},
			[32]byte{'A'}: {slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
		},
	}
	got := f.GetFork([32]byte{'H'})
	require.DeepEqual(t, want, got)
}

func TestStore_GetForks(t *testing.T) {
	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		[32]byte{'A'}: 0,
		[32]byte{'B'}: 1,
		[32]byte{'C'}: 2,
		[32]byte{'D'}: 3,
		[32]byte{'E'}: 4,
		[32]byte{'F'}: 5,
		[32]byte{'G'}: 6,
		[32]byte{'H'}: 7,
		[32]byte{'I'}: 8,
		[32]byte{'J'}: 9,
		[32]byte{'K'}: 10,
	}
	f.store.nodes = []*Node{
		//fork 0
		{slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
		{slot: 2, root: [32]byte{'B'}, parent: 0},
		{slot: 3, root: [32]byte{'C'}, parent: 1},
		{slot: 4, root: [32]byte{'D'}, parent: 2},
		{slot: 5, root: [32]byte{'E'}, parent: 3},
		{slot: 6, root: [32]byte{'F'}, parent: 4},
		//fork 1
		{slot: 7, root: [32]byte{'G'}, parent: 2},
		{slot: 8, root: [32]byte{'H'}, parent: 6},
		//fork 2
		{slot: 9, root: [32]byte{'I'}, parent: 3},
		{slot: 10, root: [32]byte{'J'}, parent: 8},
		{slot: 11, root: [32]byte{'K'}, parent: 9},
	}
	want := []*Fork{
		{
			roots: [][32]byte{
				{'K'},
				{'J'},
				{'I'},
				{'D'},
				{'C'},
				{'B'},
				{'A'},
			},
			nodesMap: map[[32]byte]*Node{
				[32]byte{'K'}: {slot: 11, root: [32]byte{'K'}, parent: 9},
				[32]byte{'J'}: {slot: 10, root: [32]byte{'J'}, parent: 8},
				[32]byte{'I'}: {slot: 9, root: [32]byte{'I'}, parent: 3},
				[32]byte{'D'}: {slot: 4, root: [32]byte{'D'}, parent: 2},
				[32]byte{'C'}: {slot: 3, root: [32]byte{'C'}, parent: 1},
				[32]byte{'B'}: {slot: 2, root: [32]byte{'B'}, parent: 0},
				[32]byte{'A'}: {slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
			},
		},
		{
			roots: [][32]byte{
				{'H'},
				{'G'},
				{'C'},
				{'B'},
				{'A'},
			},
			nodesMap: map[[32]byte]*Node{
				[32]byte{'H'}: {slot: 8, root: [32]byte{'H'}, parent: 6},
				[32]byte{'G'}: {slot: 7, root: [32]byte{'G'}, parent: 2},
				[32]byte{'C'}: {slot: 3, root: [32]byte{'C'}, parent: 1},
				[32]byte{'B'}: {slot: 2, root: [32]byte{'B'}, parent: 0},
				[32]byte{'A'}: {slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
			},
		},
		{
			roots: [][32]byte{
				{'F'},
				{'E'},
				{'D'},
				{'C'},
				{'B'},
				{'A'},
			},
			nodesMap: map[[32]byte]*Node{
				[32]byte{'F'}: {slot: 6, root: [32]byte{'F'}, parent: 4},
				[32]byte{'E'}: {slot: 5, root: [32]byte{'E'}, parent: 3},
				[32]byte{'D'}: {slot: 4, root: [32]byte{'D'}, parent: 2},
				[32]byte{'C'}: {slot: 3, root: [32]byte{'C'}, parent: 1},
				[32]byte{'B'}: {slot: 2, root: [32]byte{'B'}, parent: 0},
				[32]byte{'A'}: {slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
			},
		},
	}

	got := f.GetForks()
	require.DeepEqual(t, want, got)
}

func TestStore_GetForks_from_one(t *testing.T) {
	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		[32]byte{'A'}: 0,
	}
	f.store.nodes = []*Node{
		//fork 0
		{slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
	}
	want := []*Fork{
		{
			roots: [][32]byte{{'A'}},
			nodesMap: map[[32]byte]*Node{
				[32]byte{'A'}: {slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
			},
		},
	}

	got := f.GetForks()
	require.DeepEqual(t, want, got)
}

func TestStore_GetFirstFork(t *testing.T) {
	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		[32]byte{'A'}: 0,
		[32]byte{'B'}: 1,
		[32]byte{'C'}: 2,
		[32]byte{'D'}: 3,
		[32]byte{'E'}: 4,
		[32]byte{'F'}: 5,
		[32]byte{'G'}: 6,
		[32]byte{'H'}: 7,
		[32]byte{'I'}: 8,
		[32]byte{'J'}: 9,
		[32]byte{'K'}: 10,
	}
	f.store.nodes = []*Node{
		//fork 0
		{slot: 1, root: [32]byte{'A'}, parent: NonExistentNode},
		{slot: 2, root: [32]byte{'B'}, parent: 0},
		{slot: 3, root: [32]byte{'C'}, parent: 1},
		{slot: 4, root: [32]byte{'D'}, parent: 2},
		{slot: 5, root: [32]byte{'E'}, parent: 3},
		{slot: 6, root: [32]byte{'F'}, parent: 4},
		//fork 1
		{slot: 7, root: [32]byte{'G'}, parent: 2},
		{slot: 8, root: [32]byte{'H'}, parent: 6},
		//fork 2
		{slot: 9, root: [32]byte{'I'}, parent: 3},
		{slot: 10, root: [32]byte{'J'}, parent: 8},
		{slot: 11, root: [32]byte{'K'}, parent: 9},
	}
	want := &Node{slot: 3, root: [32]byte{'C'}, parent: 1}
	got := f.GetCommonAncestor()
	require.DeepEqual(t, want, got)
}

func nrToHash(i int) [32]byte {
	return bytesutil.ToBytes32([]byte(fmt.Sprintf("%d", i)))
}

func Test_collectTgTreeNodesByOptimisticSpines_prefix_extension(t *testing.T) {

	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
		nrToHash(5): 5,
		nrToHash(6): 6,
		nrToHash(7): 7,
		nrToHash(8): 8,
		nrToHash(9): 9,
	}
	f.store.nodes = []*Node{

		{slot: 0, root: nrToHash(0), parent: NonExistentNode, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 1, root: nrToHash(1), parent: 0, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 2, root: nrToHash(2), parent: 1, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 3, root: nrToHash(3), parent: 2, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 4, root: nrToHash(4), parent: 3, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 5, root: nrToHash(5), parent: 4, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 0
		{slot: 6, root: nrToHash(6), parent: 5, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 7, root: nrToHash(7), parent: 6, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '5'}, {'a', '6'}, {'a', '7'}},
			prefix:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 8, root: nrToHash(8), parent: 7, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '6'}, {'a', '7'}, {'a', '8'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 9, root: nrToHash(9), parent: 8, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '7'}, {'a', '8'}, {'a', '9'}, {'a', '1', '0'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}, {'a', '7'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
	}

	optSpines := []gwatCommon.HashArray{
		{{'a', '1'}},
		{nrToHash(0), nrToHash(0), {'a', '2'}, nrToHash(0)},
		{{'a', '3'}, nrToHash(0), nrToHash(0)},
		{nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), {'a', '4'}},
		{nrToHash(0), {'a', '5'}, nrToHash(0), nrToHash(0)},
		{nrToHash(0), nrToHash(0), nrToHash(0), {'a', '6'}, nrToHash(0), nrToHash(0)},
		{{'a', '7'}},
		{{'a', '8'}, nrToHash(0)},
		{nrToHash(0), nrToHash(0), {'a', '9'}},
		{{'a', '1', '0'}},
	}

	wantRootIndexMap := map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
		nrToHash(5): 5,
		nrToHash(6): 6,
		nrToHash(7): 7,
		nrToHash(8): 8,
		nrToHash(9): 9,
	}
	wantLeafs := map[[32]byte]int{nrToHash(9): 10}

	rootIndexMap, leafs := collectTgTreeNodesByOptimisticSpines(f, optSpines, nrToHash(0))
	require.DeepEqual(t, wantRootIndexMap, rootIndexMap)
	require.DeepEqual(t, wantLeafs, leafs)
}

func Test_collectTgTreeNodesByOptimisticSpines_prefix_not_extension(t *testing.T) {

	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
		nrToHash(5): 5,
		nrToHash(6): 6,
		nrToHash(7): 7,
		nrToHash(8): 8,
		nrToHash(9): 9,
	}
	f.store.nodes = []*Node{

		{slot: 0, root: nrToHash(0), parent: NonExistentNode, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 1, root: nrToHash(1), parent: 0, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 2, root: nrToHash(2), parent: 1, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 3, root: nrToHash(3), parent: 2, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 4, root: nrToHash(4), parent: 3, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 5, root: nrToHash(5), parent: 4, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 0
		{slot: 6, root: nrToHash(6), parent: 5, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 7, root: nrToHash(7), parent: 6, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '5'}, {'a', '6'}, {'a', '7'}},
			prefix:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 8, root: nrToHash(8), parent: 7, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '6'}, {'a', '7'}, {'a', '8'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 9, root: nrToHash(9), parent: 8, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '8'}, {'a', '9'}, {'a', '1', '0'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}, {'a', '7'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
	}

	optSpines := []gwatCommon.HashArray{
		{{'a', '1'}},
		{nrToHash(0), nrToHash(0), {'a', '2'}, nrToHash(0)},
		{{'a', '3'}, nrToHash(0), nrToHash(0)},
		{nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), {'a', '4'}},
		{nrToHash(0), {'a', '5'}, nrToHash(0), nrToHash(0)},
		{nrToHash(0), nrToHash(0), nrToHash(0), {'a', '6'}, nrToHash(0), nrToHash(0)},
		{{'a', '7'}},
		{{'a', '8'}, nrToHash(0)},
		{nrToHash(0), nrToHash(0), {'a', '9'}},
		{{'a', '1', '0'}},
	}

	wantRootIndexMap := map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
		nrToHash(5): 5,
		nrToHash(6): 6,
		nrToHash(7): 7,
		nrToHash(8): 8,
		nrToHash(9): 9,
	}
	wantLeafs := map[[32]byte]int{nrToHash(9): 10}

	rootIndexMap, leafs := collectTgTreeNodesByOptimisticSpines(f, optSpines, nrToHash(0))
	require.DeepEqual(t, wantRootIndexMap, rootIndexMap)
	require.DeepEqual(t, wantLeafs, leafs)
}

func Test_collectTgTreeNodesByOptimisticSpines_3_forks(t *testing.T) {

	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
		nrToHash(5): 5,
		nrToHash(6): 6,
		nrToHash(7): 7,
		nrToHash(8): 8,
		nrToHash(9): 9,
	}
	f.store.nodes = []*Node{

		{slot: 0, root: nrToHash(0), parent: NonExistentNode, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 1, root: nrToHash(1), parent: 0, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 2, root: nrToHash(2), parent: 1, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 3, root: nrToHash(3), parent: 2, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 1
		{slot: 4, root: nrToHash(4), parent: 1, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 5, root: nrToHash(5), parent: 4, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 2
		{slot: 6, root: nrToHash(6), parent: 2, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 7, root: nrToHash(7), parent: 6, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '5'}, {'a', '6'}, {'a', '7'}},
			prefix:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 3
		{slot: 8, root: nrToHash(8), parent: 5, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '6'}, {'a', '7'}, {'a', '8'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 9, root: nrToHash(9), parent: 8, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '7'}, {'a', '8'}, {'a', '9'}, {'a', '1', '0'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}, {'a', '7'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
	}

	optSpines := []gwatCommon.HashArray{
		{{'a', '1'}},
		{nrToHash(0), nrToHash(0), {'a', '2'}, nrToHash(0)},
		{{'a', '3'}, nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0)},
		{nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), {'a', '4'}},
		{nrToHash(0), {'a', '5'}, nrToHash(0), nrToHash(0)},
		{nrToHash(0), nrToHash(0), nrToHash(0), {'a', '6'}, nrToHash(0), nrToHash(0)},
		{{'a', '7'}},
		{{'a', '8'}, nrToHash(0)},
		{nrToHash(0), nrToHash(0), {'a', '9'}},
		{{'a', '1', '0'}},
	}

	wantRootIndexMap := map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
		nrToHash(5): 5,
		nrToHash(6): 6,
		nrToHash(7): 7,
		nrToHash(8): 8,
		nrToHash(9): 9,
	}
	wantLeafs := map[[32]byte]int{
		nrToHash(9): 6,
		nrToHash(7): 5,
		nrToHash(3): 4,
	}

	rootIndexMap, leafs := collectTgTreeNodesByOptimisticSpines(f, optSpines, nrToHash(0))
	require.DeepEqual(t, wantRootIndexMap, rootIndexMap)
	require.DeepEqual(t, wantLeafs, leafs)
}

func Test_collectTgTreeNodesByOptimisticSpines_1_forks(t *testing.T) {

	f := &ForkChoice{store: &Store{}}
	f.store.canonicalNodes = map[[32]byte]bool{}
	f.store.nodesIndices = map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
		nrToHash(5): 5,
		nrToHash(6): 6,
		nrToHash(7): 7,
		nrToHash(8): 8,
		nrToHash(9): 9,
	}
	f.store.nodes = []*Node{

		{slot: 0, root: nrToHash(0), parent: NonExistentNode, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 1, root: nrToHash(1), parent: 0, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 2, root: nrToHash(2), parent: 1, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			prefix:       gwatCommon.HashArray{},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 3, root: nrToHash(3), parent: 2, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 1
		{slot: 4, root: nrToHash(4), parent: 1, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 5, root: nrToHash(5), parent: 4, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 2
		{slot: 6, root: nrToHash(6), parent: 2, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			prefix:       gwatCommon.HashArray{{'a', '2'}, {'a', '3'}, {'a', '4'}},
			finalization: gwatCommon.HashArray{{'a', '1'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 7, root: nrToHash(7), parent: 6, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '5'}, {'a', '6'}, {'a', '7'}},
			prefix:       gwatCommon.HashArray{{'a', '3'}, {'a', '4'}, {'a', '5'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		//fork 3
		{slot: 8, root: nrToHash(8), parent: 5, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '6'}, {'a', '7'}, {'a', '8'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
		{slot: 9, root: nrToHash(9), parent: 8, spinesData: &SpinesData{
			spines:       gwatCommon.HashArray{{'a', '7'}, {'a', '8'}, {'a', '9'}, {'a', '1', '0'}},
			prefix:       gwatCommon.HashArray{{'a', '4'}, {'a', '5'}, {'a', '6'}, {'a', '7'}},
			finalization: gwatCommon.HashArray{{'a', '1'}, {'a', '2'}, {'a', '3'}},
			cpFinalized:  gwatCommon.HashArray{{'x', 'x', 'x'}},
		}},
	}

	optSpines := []gwatCommon.HashArray{
		{{'a', '1'}},
		{nrToHash(0), nrToHash(0), {'a', '2'}, nrToHash(0)},
		{{'b', '3'}}, // <<< "b3"
		{nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), nrToHash(0), {'a', '4'}},
		{{'a', '5'}},
		{nrToHash(0), nrToHash(0), nrToHash(0), {'a', '6'}, nrToHash(0), nrToHash(0)},
		{{'a', '7'}},
		{{'a', '8'}, nrToHash(0)},
		{nrToHash(0), nrToHash(0), {'a', '9'}},
		{{'a', '1', '0'}},
	}

	wantRootIndexMap := map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(2): 2,
		nrToHash(3): 3,
		nrToHash(4): 4,
	}
	wantLeafs := map[[32]byte]int{
		nrToHash(4): 3,
		nrToHash(2): 3,
		nrToHash(3): 4,
	}

	rootIndexMap, leafs := collectTgTreeNodesByOptimisticSpines(f, optSpines, nrToHash(0))
	require.DeepEqual(t, wantRootIndexMap, rootIndexMap)
	require.DeepEqual(t, wantLeafs, leafs)
}

func TestGetParentByOptimisticSpines_TwoBranches(t *testing.T) {
	balances := []uint64{1, 1}
	justifiedRoot := nrToHash(0)
	finalizedRoot := nrToHash(0)
	var (
		r, hRoot          [32]byte
		err, hrErr        error
		nodesRootIndexMap map[[32]byte]uint64
	)

	f := New(0, 0)
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 0, nrToHash(0), params.BeaconConfig().ZeroHash, 0, 0, justifiedRoot[:], finalizedRoot[:], nil))

	r, err = f.Head(context.Background(), 0, nrToHash(0), balances, 0)
	require.NoError(t, err)
	assert.Equal(t, nrToHash(0), r, "Incorrect head with genesis")

	nodesRootIndexMap = map[[32]byte]uint64{nrToHash(0): 0}
	fcBase, _, diffNodes := getCompatibleFc(nodesRootIndexMap, f)
	hRoot, hrErr = calculateHeadRootByNodesIndexes(context.Background(), fcBase, diffNodes, nodesRootIndexMap, justifiedRoot)
	require.NoError(t, hrErr)
	assert.Equal(t, nrToHash(0), hRoot, "Incorrect head with justified epoch at 0")

	// Define the following tree:
	//                                0
	//                               / \
	//  justified: 0, finalization: 0 -> 1   2 <- justified: 0, finalization: 0
	//                              |   |
	//  justified: 1, finalization: 0 -> 3   4 <- justified: 0, finalization: 0
	//                              |   |
	//  justified: 1, finalization: 0 -> 5   6 <- justified: 0, finalization: 0
	//                              |   |
	//  justified: 1, finalization: 0 -> 7   8 <- justified: 1, finalization: 0
	//                              |   |
	//  justified: 2, finalization: 0 -> 9  10 <- justified: 2, finalization: 0
	// Left branch.
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 1, nrToHash(1), nrToHash(0), 0, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 2, nrToHash(3), nrToHash(1), 1, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 3, nrToHash(5), nrToHash(3), 1, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 4, nrToHash(7), nrToHash(5), 1, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 4, nrToHash(9), nrToHash(7), 2, 0, justifiedRoot[:], finalizedRoot[:], nil))
	// Right branch.
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 1, nrToHash(2), nrToHash(0), 0, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 2, nrToHash(4), nrToHash(2), 0, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 3, nrToHash(6), nrToHash(4), 0, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 4, nrToHash(8), nrToHash(6), 1, 0, justifiedRoot[:], finalizedRoot[:], nil))
	require.NoError(t, f.InsertOptimisticBlock(context.Background(), 4, nrToHash(10), nrToHash(8), 2, 0, justifiedRoot[:], finalizedRoot[:], nil))

	// With start at 0, the head should be 10:
	//           0  <-- start
	//          / \
	//         1   2
	//         |   |
	//         3   4
	//         |   |
	//         5   6
	//         |   |
	//         7   8
	//         |   |
	//         9  10 <-- head
	r, err = f.Head(context.Background(), 0, nrToHash(0), balances, 0)
	require.NoError(t, err)
	assert.Equal(t, nrToHash(10), r, "Incorrect head with justified epoch at 0")

	nodesRootIndexMap = map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(3): 2,
		nrToHash(5): 3,
		nrToHash(7): 4,
		nrToHash(9): 5,

		nrToHash(2):  6,
		nrToHash(4):  7,
		nrToHash(6):  8,
		nrToHash(8):  9,
		nrToHash(10): 10,
	}

	fcBase, _, diffNodes = getCompatibleFc(nodesRootIndexMap, f)

	hRoot, hrErr = calculateHeadRootByNodesIndexes(context.Background(), fcBase, diffNodes, nodesRootIndexMap, justifiedRoot)
	require.NoError(t, hrErr)
	assert.Equal(t, nrToHash(10), hRoot, "Incorrect head with justified epoch at 0")

	// Add a vote to 1:
	//                 0
	//                / \
	//    +1 vote -> 1   2
	//               |   |
	//               3   4
	//               |   |
	//               5   6
	//               |   |
	//               7   8
	//               |   |
	//               9  10
	f.ProcessAttestation(context.Background(), []uint64{0}, nrToHash(1), 0)

	// With the additional vote to the left branch, the head should be 9:
	//           0  <-- start
	//          / \
	//         1   2
	//         |   |
	//         3   4
	//         |   |
	//         5   6
	//         |   |
	//         7   8
	//         |   |
	// head -> 9  10
	r, err = f.Head(context.Background(), 0, nrToHash(0), balances, 0)
	require.NoError(t, err)
	assert.Equal(t, nrToHash(9), r, "Incorrect head with justified epoch at 0")

	nodesRootIndexMap = map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(3): 2,
		nrToHash(5): 3,
		nrToHash(7): 4,
		nrToHash(9): 5,

		//nrToHash(2):  6,
		//nrToHash(4):  7,
		//nrToHash(6):  8,
		//nrToHash(8):  9,
		//nrToHash(10): 10,
	}
	fcBase, _, diffNodes = getCompatibleFc(nodesRootIndexMap, f)
	hRoot, hrErr = calculateHeadRootByNodesIndexes(context.Background(), fcBase, diffNodes, nodesRootIndexMap, justifiedRoot)
	require.NoError(t, hrErr)
	assert.Equal(t, nrToHash(9), hRoot, "Incorrect head with justified epoch at 0")

	// Add a vote to 2:
	//                 0
	//                / \
	//               1   2 <- +1 vote
	//               |   |
	//               3   4
	//               |   |
	//               5   6
	//               |   |
	//               7   8
	//               |   |
	//               9  10
	f.ProcessAttestation(context.Background(), []uint64{1}, nrToHash(2), 0)

	// With the additional vote to the right branch, the head should be 10:
	//           0  <-- start
	//          / \
	//         1   2
	//         |   |
	//         3   4
	//         |   |
	//         5   6
	//         |   |
	//         7   8
	//         |   |
	//         9  10 <-- head
	r, err = f.Head(context.Background(), 0, nrToHash(0), balances, 0)
	require.NoError(t, err)
	assert.Equal(t, nrToHash(10), r, "Incorrect head with justified epoch at 0")

	nodesRootIndexMap = map[[32]byte]uint64{
		nrToHash(0): 0,
		nrToHash(1): 1,
		nrToHash(3): 2,
		nrToHash(5): 3,
		nrToHash(7): 4,
		nrToHash(9): 5,

		//nrToHash(2):  2,
		//nrToHash(4):  4,
		//nrToHash(6):  6,
		//nrToHash(8):  8,
		//nrToHash(10): 10,
	}
	fcBase, _, diffNodes = getCompatibleFc(nodesRootIndexMap, f)
	hRoot, hrErr = calculateHeadRootByNodesIndexes(context.Background(), fcBase, diffNodes, nodesRootIndexMap, justifiedRoot)
	require.NoError(t, hrErr)
	assert.Equal(t, nrToHash(9), hRoot, "Incorrect head with justified epoch at 0")

	r, err = f.Head(context.Background(), 1, nrToHash(1), balances, 0)
	require.NoError(t, err)
	assert.Equal(t, nrToHash(7), r, "Incorrect head with justified epoch at 0")
}

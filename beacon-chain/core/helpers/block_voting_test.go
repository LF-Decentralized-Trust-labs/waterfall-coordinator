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

package helpers_test

import (
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"github.com/prysmaticlabs/go-bitfield"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/helpers"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

// Deprecated
func TestBlockVotingsCalcFinalization_AggregateCommitteeVote_OK(t *testing.T) {
	t.Skip() // Unstable
	// Case 1: aggregate
	list_0 := bitfield.NewBitlist(10)
	list_0.SetBitAt(0, true)
	list_0.SetBitAt(1, true)
	list_0.SetBitAt(2, true)

	var votes []*ethpb.CommitteeVote
	votes = append(votes, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: list_0,
	})

	// Vote to aggregate
	list_1 := bitfield.NewBitlist(10)
	list_1.SetBitAt(4, true)
	list_1.SetBitAt(6, true)
	list_1.SetBitAt(8, true)
	addVote := &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: list_1,
	}
	votes = append(votes, addVote)

	//diff committee
	list_2 := bitfield.NewBitlist(10)
	list_2.SetBitAt(5, true)
	votes = append(votes, &ethpb.CommitteeVote{
		Index:           1,
		Slot:            types.Slot(5),
		AggregationBits: list_2,
	})
	//diff slot
	list_3 := bitfield.NewBitlist(10)
	list_3.SetBitAt(7, true)
	votes = append(votes, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(6),
		AggregationBits: list_3,
	})

	var want []*ethpb.CommitteeVote
	var want_bits = bitfield.NewBitlist(10)
	want_bits.SetBitAt(0, true)
	want_bits.SetBitAt(1, true)
	want_bits.SetBitAt(2, true)
	want_bits.SetBitAt(4, true)
	want_bits.SetBitAt(6, true)
	want_bits.SetBitAt(8, true)

	//aggregated
	want = append(want, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: want_bits,
	})
	//diff committee
	want_bits_2 := bitfield.NewBitlist(10)
	want_bits_2.SetBitAt(5, true)
	want = append(want, &ethpb.CommitteeVote{
		Index:           1,
		Slot:            types.Slot(5),
		AggregationBits: want_bits_2,
	})
	//diff slot
	want_bits_3 := bitfield.NewBitlist(10)
	want_bits_3.SetBitAt(7, true)
	want = append(want, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(6),
		AggregationBits: want_bits_3,
	})

	agrVotes := helpers.AggregateCommitteeVote(votes)
	assert.DeepEqual(t, want, agrVotes)

	// Case 2: append new
	// Vote to append
	list_4 := bitfield.NewBitlist(10)
	list_4.SetBitAt(4, true)
	appVote := &ethpb.CommitteeVote{
		Index:           7,
		Slot:            types.Slot(10),
		AggregationBits: list_4,
	}
	votes = append(votes, appVote)

	want_bits_4 := bitfield.NewBitlist(10)
	want_bits_4.SetBitAt(4, true)
	want = append(want, &ethpb.CommitteeVote{
		Index:           7,
		Slot:            types.Slot(10),
		AggregationBits: want_bits_4,
	})

	agrVotes = helpers.AggregateCommitteeVote(votes)

	assert.DeepEqual(t, want, agrVotes)
}

func TestBlockVotingsCalcFinalization_AddAggregateCommitteeVote_OK(t *testing.T) {
	// Case 1: aggregate
	list_0 := bitfield.NewBitlist(10)
	list_0.SetBitAt(0, true)
	list_0.SetBitAt(1, true)
	list_0.SetBitAt(2, true)

	var votes []*ethpb.CommitteeVote
	votes = append(votes, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: list_0,
	})

	// Vote to aggregate
	list_1 := bitfield.NewBitlist(10)
	list_1.SetBitAt(4, true)
	list_1.SetBitAt(6, true)
	list_1.SetBitAt(8, true)
	addVote := &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: list_1,
	}
	//diff committee
	list_2 := bitfield.NewBitlist(10)
	list_2.SetBitAt(5, true)
	votes = append(votes, &ethpb.CommitteeVote{
		Index:           1,
		Slot:            types.Slot(5),
		AggregationBits: list_2,
	})
	//diff slot
	list_3 := bitfield.NewBitlist(10)
	list_3.SetBitAt(7, true)
	votes = append(votes, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(6),
		AggregationBits: list_3,
	})

	var want []*ethpb.CommitteeVote
	var want_bits = bitfield.NewBitlist(10)
	want_bits.SetBitAt(0, true)
	want_bits.SetBitAt(1, true)
	want_bits.SetBitAt(2, true)
	want_bits.SetBitAt(4, true)
	want_bits.SetBitAt(6, true)
	want_bits.SetBitAt(8, true)

	//aggregated
	want = append(want, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: want_bits,
	})
	//diff committee
	want_bits_2 := bitfield.NewBitlist(10)
	want_bits_2.SetBitAt(5, true)
	want = append(want, &ethpb.CommitteeVote{
		Index:           1,
		Slot:            types.Slot(5),
		AggregationBits: want_bits_2,
	})
	//diff slot
	want_bits_3 := bitfield.NewBitlist(10)
	want_bits_3.SetBitAt(7, true)
	want = append(want, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(6),
		AggregationBits: want_bits_3,
	})

	agrVotes := helpers.AddAggregateCommitteeVote(votes, addVote)
	assert.DeepEqual(t, want, agrVotes)

	// Case 2: append new
	// Vote to append
	list_4 := bitfield.NewBitlist(10)
	list_4.SetBitAt(4, true)
	appVote := &ethpb.CommitteeVote{
		Index:           7,
		Slot:            types.Slot(10),
		AggregationBits: list_4,
	}

	want_bits_4 := bitfield.NewBitlist(10)
	want_bits_4.SetBitAt(4, true)
	want = append(want, &ethpb.CommitteeVote{
		Index:           7,
		Slot:            types.Slot(10),
		AggregationBits: want_bits_4,
	})
	agrVotes = helpers.AddAggregateCommitteeVote(votes, appVote)
	assert.DeepEqual(t, want, agrVotes)
}

func TestAttestationArrSort(t *testing.T) {
	list := bitfield.NewBitlist(4)
	list.SetBitAt(0, true)
	list.SetBitAt(1, true)
	list.SetBitAt(2, true)

	att_0 := &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: list,
	}
	att_00 := &ethpb.CommitteeVote{
		AggregationBits: list,
		Slot:            types.Slot(5),
		Index:           0,
	}
	att_1 := &ethpb.CommitteeVote{
		Slot:            types.Slot(6),
		Index:           0,
		AggregationBits: list,
	}
	att_2 := &ethpb.CommitteeVote{
		Slot:            types.Slot(7),
		Index:           0,
		AggregationBits: list,
	}

	testAtts := []*ethpb.CommitteeVote{
		att_0,
		att_1,
		att_2,
	}

	testWant := []*ethpb.CommitteeVote{
		att_1,
		att_2,
		att_00,
	}

	//attestation invariant
	att_0HTR, err := att_0.HashTreeRoot()
	assert.NoError(t, err)
	att_00HTR, err := att_00.HashTreeRoot()
	assert.NoError(t, err)
	assert.DeepEqual(t, att_00HTR, att_0HTR)

	sortedAtts, err := helpers.CommitteeVoteArrSort(testAtts)
	assert.NoError(t, err)
	assert.DeepEqual(t, testWant, sortedAtts)
}

func TestBlockVotingArrSort(t *testing.T) {
	list := bitfield.NewBitlist(4)
	list.SetBitAt(0, true)
	list.SetBitAt(1, true)
	list.SetBitAt(2, true)
	root_0 := gwatCommon.BytesToHash([]byte("root-0--------------------------"))
	var atts_0 []*ethpb.CommitteeVote
	atts_0 = append(atts_0, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            5,
		AggregationBits: list,
	})

	bv_0 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 5,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: atts_0,
	}
	bv_1 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 7,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: atts_0,
	}
	bv_2 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 6,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: atts_0,
	}

	blobVotings := []*ethpb.BlockVoting{
		bv_0,
		bv_1,
		bv_2,
	}
	testWant := []*ethpb.BlockVoting{
		bv_1,
		bv_2,
		bv_0,
	}

	sortedBv, err := helpers.BlockVotingArrSort(blobVotings)
	assert.NoError(t, err)
	assert.DeepEqual(t, testWant, sortedBv)
}

func TestBlockVotingArrStateOrder(t *testing.T) {
	list := bitfield.NewBitlist(4)
	list.SetBitAt(0, true)
	list.SetBitAt(1, true)
	list.SetBitAt(2, true)
	root_0 := gwatCommon.BytesToHash([]byte("root-0--------------------------"))
	var atts_0 []*ethpb.CommitteeVote
	atts_0 = append(atts_0, &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: list,
	})

	att_0 := &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(5),
		AggregationBits: list,
	}
	att_00 := &ethpb.CommitteeVote{
		AggregationBits: list,
		Slot:            types.Slot(5),
		Index:           0,
	}
	att_1 := &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(6),
		AggregationBits: list,
	}
	att_2 := &ethpb.CommitteeVote{
		Index:           0,
		Slot:            types.Slot(7),
		AggregationBits: list,
	}

	testAtts := []*ethpb.CommitteeVote{
		att_0,
		att_1,
		att_2,
	}

	bv_0 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 5,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: testAtts,
	}
	bv_1 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 6,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: atts_0,
	}
	bv_2 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 7,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: atts_0,
	}

	blobVotings := []*ethpb.BlockVoting{
		bv_0,
		bv_1,
		bv_2,
	}

	// expected
	bv_00 := helpers.BlockVotingCopy(bv_0)
	bv_00.Votes = []*ethpb.CommitteeVote{
		att_1,
		att_2,
		att_00,
	}
	testWant := []*ethpb.BlockVoting{
		bv_00,
		bv_2,
		bv_1,
	}

	sortedBv, err := helpers.BlockVotingArrStateOrder(blobVotings)
	assert.NoError(t, err)
	testWant, err = helpers.BlockVotingArrStateOrder(testWant)
	assert.NoError(t, err)
	assert.DeepEqual(t, testWant, sortedBv)
}

func TestBlockVotingArrStateOrder_attEmpty(t *testing.T) {
	list := bitfield.NewBitlist(4)
	list.SetBitAt(0, true)
	list.SetBitAt(1, true)
	list.SetBitAt(2, true)

	root_0 := gwatCommon.BytesToHash([]byte("root-0--------------------------"))

	bv_0 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 5,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: []*ethpb.CommitteeVote{},
	}
	bv_1 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 6,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: []*ethpb.CommitteeVote{},
	}
	bv_2 := &ethpb.BlockVoting{
		Root: root_0[:],
		Slot: 7,
		Candidates: gwatCommon.HashArray{
			gwatCommon.Hash{0x11, 0x11},
			gwatCommon.Hash{0x11, 0x22},
		}.ToBytes(),
		Votes: []*ethpb.CommitteeVote{},
	}

	blobVotings := []*ethpb.BlockVoting{
		bv_0,
		bv_1,
		bv_2,
	}

	// expected
	bv_00 := helpers.BlockVotingCopy(bv_0)
	bv_00.Votes = []*ethpb.CommitteeVote{}
	testWant := []*ethpb.BlockVoting{
		bv_1,
		bv_00,
		bv_2,
	}

	sortedBv, err := helpers.BlockVotingArrStateOrder(blobVotings)
	assert.NoError(t, err)

	testWant, err = helpers.BlockVotingArrStateOrder(testWant)
	assert.NoError(t, err)

	assert.DeepEqual(t, testWant, sortedBv)
}

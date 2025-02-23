package slasher

import (
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	slashertypes "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/slasher/types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func Test_attestationsQueue(t *testing.T) {
	t.Run("push_and_dequeue", func(tt *testing.T) {
		attQueue := newAttestationsQueue()
		wantedAtts := []*slashertypes.IndexedAttestationWrapper{
			createAttestationWrapper(t, 0, 1, []uint64{1}, make([]byte, 32)),
			createAttestationWrapper(t, 1, 2, []uint64{1}, make([]byte, 32)),
		}
		attQueue.push(wantedAtts[0])
		attQueue.push(wantedAtts[1])
		require.DeepEqual(t, 2, attQueue.size())

		received := attQueue.dequeue()
		require.DeepEqual(t, 0, attQueue.size())
		require.DeepEqual(t, wantedAtts, received)
	})

	t.Run("extend_and_dequeue", func(tt *testing.T) {
		attQueue := newAttestationsQueue()
		wantedAtts := []*slashertypes.IndexedAttestationWrapper{
			createAttestationWrapper(t, 0, 1, []uint64{1}, make([]byte, 32)),
			createAttestationWrapper(t, 1, 2, []uint64{1}, make([]byte, 32)),
		}
		attQueue.extend(wantedAtts)
		require.DeepEqual(t, 2, attQueue.size())

		received := attQueue.dequeue()
		require.DeepEqual(t, 0, attQueue.size())
		require.DeepEqual(t, wantedAtts, received)
	})
}

func Test_blocksQueue(t *testing.T) {
	t.Run("push_and_dequeue", func(tt *testing.T) {
		blkQueue := newBlocksQueue()
		wantedBlks := []*slashertypes.SignedBlockHeaderWrapper{
			createProposalWrapper(t, 0, types.ValidatorIndex(1), make([]byte, 32)),
			createProposalWrapper(t, 1, types.ValidatorIndex(1), make([]byte, 32)),
		}
		blkQueue.push(wantedBlks[0])
		blkQueue.push(wantedBlks[1])
		require.DeepEqual(t, 2, blkQueue.size())

		received := blkQueue.dequeue()
		require.DeepEqual(t, 0, blkQueue.size())
		require.DeepEqual(t, wantedBlks, received)
	})

	t.Run("extend_and_dequeue", func(tt *testing.T) {
		blkQueue := newBlocksQueue()
		wantedBlks := []*slashertypes.SignedBlockHeaderWrapper{
			createProposalWrapper(t, 0, types.ValidatorIndex(1), make([]byte, 32)),
			createProposalWrapper(t, 1, types.ValidatorIndex(1), make([]byte, 32)),
		}
		blkQueue.extend(wantedBlks)
		require.DeepEqual(t, 2, blkQueue.size())

		received := blkQueue.dequeue()
		require.DeepEqual(t, 0, blkQueue.size())
		require.DeepEqual(t, wantedBlks, received)
	})
}

package kv

import (
	"context"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
)

func TestArchivedPointIndexRoot_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	i1 := types.Slot(100)
	r1 := [32]byte{'A'}

	received := db.ArchivedPointRoot(ctx, i1)
	require.NotEqual(t, r1, received, "Should not have been saved")
	st, err := NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(i1))
	require.NoError(t, db.SaveState(ctx, st, r1))
	received = db.ArchivedPointRoot(ctx, i1)
	assert.Equal(t, r1, received, "Should have been saved")
}

func TestLastArchivedPoint_CanRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	i, err := db.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, types.Slot(0), i, "Did not get correct index")

	st, err := NewBeaconState()
	require.NoError(t, err)
	assert.NoError(t, db.SaveState(ctx, st, [32]byte{'A'}))
	assert.Equal(t, [32]byte{'A'}, db.LastArchivedRoot(ctx), "Did not get wanted root")

	assert.NoError(t, st.SetSlot(2))
	assert.NoError(t, db.SaveState(ctx, st, [32]byte{'B'}))
	assert.Equal(t, [32]byte{'B'}, db.LastArchivedRoot(ctx))

	assert.NoError(t, st.SetSlot(3))
	assert.NoError(t, db.SaveState(ctx, st, [32]byte{'C'}))

	i, err = db.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, types.Slot(3), i, "Did not get correct index")
}

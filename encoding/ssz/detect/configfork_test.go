package detect

import (
	"context"
	"fmt"
	"math"
	"testing"

	types "github.com/prysmaticlabs/eth2-types"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/util"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
)

func TestSlotFromBlock(t *testing.T) {
	b := testBlockGenesis()
	var slot types.Slot = 3
	b.Block.Slot = slot
	bb, err := b.MarshalSSZ()
	require.NoError(t, err)
	sfb, err := slotFromBlock(bb)
	require.NoError(t, err)
	require.Equal(t, slot, sfb)

	ba := testBlockAltair()
	ba.Block.Slot = slot
	bab, err := ba.MarshalSSZ()
	require.NoError(t, err)
	sfba, err := slotFromBlock(bab)
	require.NoError(t, err)
	require.Equal(t, slot, sfba)
}

func TestByState(t *testing.T) {
	bc, cleanup := hackBellatrixMaxuint()
	defer cleanup()
	altairSlot, err := slots.EpochStart(bc.AltairForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		name        string
		version     int
		slot        types.Slot
		forkversion [4]byte
	}{
		{
			name:        "genesis",
			version:     version.Phase0,
			slot:        0,
			forkversion: bytesutil.ToBytes4(bc.GenesisForkVersion),
		},
		{
			name:        "altair",
			version:     version.Altair,
			slot:        altairSlot,
			forkversion: bytesutil.ToBytes4(bc.AltairForkVersion),
		},
	}
	for _, c := range cases {
		st, err := stateForVersion(c.version)
		require.NoError(t, err)
		require.NoError(t, st.SetFork(&ethpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  c.forkversion[:],
			Epoch:           0,
		}))
		require.NoError(t, st.SetSlot(c.slot))
		m, err := st.MarshalSSZ()
		require.NoError(t, err)
		cf, err := FromState(m)
		require.NoError(t, err)
		require.Equal(t, c.version, cf.Fork)
		require.Equal(t, c.forkversion, cf.Version)
		require.Equal(t, bc.ConfigName, cf.Config.ConfigName)
	}
}

func stateForVersion(v int) (state.BeaconState, error) {
	switch v {
	case version.Phase0:
		return util.NewBeaconState()
	case version.Altair:
		return util.NewBeaconStateAltair()
	default:
		return nil, fmt.Errorf("unrecognoized version %d", v)
	}
}

func TestUnmarshalState(t *testing.T) {
	ctx := context.Background()
	bc, cleanup := hackBellatrixMaxuint()
	defer cleanup()
	altairSlot, err := slots.EpochStart(bc.AltairForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		name        string
		version     int
		slot        types.Slot
		forkversion [4]byte
	}{
		{
			name:        "genesis",
			version:     version.Phase0,
			slot:        0,
			forkversion: bytesutil.ToBytes4(bc.GenesisForkVersion),
		},
		{
			name:        "altair",
			version:     version.Altair,
			slot:        altairSlot,
			forkversion: bytesutil.ToBytes4(bc.AltairForkVersion),
		},
	}
	for _, c := range cases {
		st, err := stateForVersion(c.version)
		require.NoError(t, err)
		require.NoError(t, st.SetFork(&ethpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  c.forkversion[:],
			Epoch:           0,
		}))
		require.NoError(t, st.SetSlot(c.slot))
		m, err := st.MarshalSSZ()
		require.NoError(t, err)
		cf, err := FromState(m)
		require.NoError(t, err)
		s, err := cf.UnmarshalBeaconState(m)
		require.NoError(t, err)
		expected, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		actual, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, expected, actual)
	}
}

func hackBellatrixMaxuint() (*params.BeaconChainConfig, func()) {
	// We monkey patch the config to use a smaller value for the bellatrix fork epoch.
	// Upstream configs use MaxUint64, which leads to a multiplication overflow when converting epoch->slot.
	// Unfortunately we have unit tests that assert our config matches the upstream config, so we have to choose between
	// breaking conformance, adding a special case to the conformance unit test, or patch it here.
	previous := params.BeaconConfig()
	bc := params.Testnet8Config().Copy()
	bc.BellatrixForkEpoch = math.MaxUint32
	bc.InitializeForkSchedule()
	params.OverrideBeaconConfig(bc)
	// override the param used for mainnet with the patched version
	params.KnownConfigs[params.Mainnet] = func() *params.BeaconChainConfig {
		return bc
	}
	return bc, func() {
		// put the previous BeaconChainConfig back in place at the end of the test
		params.OverrideBeaconConfig(previous)
		// restore the normal MainnetConfig func in the KnownConfigs mapping
		params.KnownConfigs[params.Mainnet] = params.MainnetConfig
	}
}

func TestUnmarshalBlock(t *testing.T) {
	bc, cleanup := hackBellatrixMaxuint()
	defer cleanup()
	require.Equal(t, types.Epoch(math.MaxUint32), params.KnownConfigs[params.Mainnet]().BellatrixForkEpoch)
	genv := bytesutil.ToBytes4(bc.GenesisForkVersion)
	altairv := bytesutil.ToBytes4(bc.AltairForkVersion)
	altairS, err := slots.EpochStart(bc.AltairForkEpoch)
	require.NoError(t, err)
	bellaS, err := slots.EpochStart(bc.BellatrixForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		b       func(*testing.T, types.Slot) block.SignedBeaconBlock
		name    string
		version [4]byte
		slot    types.Slot
		err     error
	}{
		{
			name:    "genesis - slot 0",
			b:       signedTestBlockGenesis,
			version: genv,
		},
		{
			name:    "last slot of phase 0",
			b:       signedTestBlockGenesis,
			version: genv,
			slot:    altairS - 1,
		},
		{
			name:    "first slot of altair",
			b:       signedTestBlockAltair,
			version: altairv,
			slot:    altairS,
		},
		{
			name:    "last slot of altair",
			b:       signedTestBlockAltair,
			version: altairv,
			slot:    bellaS - 1,
		},
		{
			name:    "genesis block in altair slot",
			b:       signedTestBlockGenesis,
			version: genv,
			slot:    bellaS - 1,
			err:     errBlockForkMismatch,
		},
		{
			name:    "altair block in genesis slot",
			b:       signedTestBlockAltair,
			version: altairv,
			err:     errBlockForkMismatch,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.b(t, c.slot)
			marshaled, err := b.MarshalSSZ()
			require.NoError(t, err)
			cf, err := FromForkVersion(c.version)
			require.NoError(t, err)
			bcf, err := cf.UnmarshalBeaconBlock(marshaled)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			expected, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			actual, err := bcf.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func signedTestBlockGenesis(t *testing.T, slot types.Slot) block.SignedBeaconBlock {
	b := testBlockGenesis()
	b.Block.Slot = slot
	s, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func testBlockGenesis() *ethpb.SignedBeaconBlock {
	return &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			ProposerIndex: types.ValidatorIndex(0),
			ParentRoot:    make([]byte, 32),
			StateRoot:     make([]byte, 32),
			Body: &ethpb.BeaconBlockBody{
				RandaoReveal:      make([]byte, 96),
				Graffiti:          make([]byte, 32),
				ProposerSlashings: []*ethpb.ProposerSlashing{},
				AttesterSlashings: []*ethpb.AttesterSlashing{},
				Attestations:      []*ethpb.Attestation{},
				Deposits:          []*ethpb.Deposit{},
				VoluntaryExits:    []*ethpb.VoluntaryExit{},
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					Candidates:   make([]byte, 0),
					BlockHash:    make([]byte, 32),
				},
			},
		},
		Signature: make([]byte, 96),
	}
}

func signedTestBlockAltair(t *testing.T, slot types.Slot) block.SignedBeaconBlock {
	b := testBlockAltair()
	b.Block.Slot = slot
	s, err := wrapper.WrappedSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func testBlockAltair() *ethpb.SignedBeaconBlockAltair {
	return &ethpb.SignedBeaconBlockAltair{
		Block: &ethpb.BeaconBlockAltair{
			ProposerIndex: types.ValidatorIndex(0),
			ParentRoot:    make([]byte, 32),
			StateRoot:     make([]byte, 32),
			Body: &ethpb.BeaconBlockBodyAltair{
				RandaoReveal: make([]byte, 96),
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					Candidates:   make([]byte, 0),
					BlockHash:    make([]byte, 32),
				},
				Graffiti:          make([]byte, 32),
				ProposerSlashings: []*ethpb.ProposerSlashing{},
				AttesterSlashings: []*ethpb.AttesterSlashing{},
				Attestations:      []*ethpb.Attestation{},
				Deposits:          []*ethpb.Deposit{},
				VoluntaryExits:    []*ethpb.VoluntaryExit{},
				SyncAggregate: &ethpb.SyncAggregate{
					SyncCommitteeBits:      make([]byte, 64),
					SyncCommitteeSignature: make([]byte, 96),
				},
				Withdrawals: make([]*ethpb.Withdrawal, 0),
			},
		},
		Signature: make([]byte, 96),
	}
}

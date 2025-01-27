/*
Package implement decode and loading compressed genesis state.
*/
package genesis

import (
	_ "embed"

	"github.com/golang/snappy"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/io/file"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

var (
	//go:embed mainnet.ssz.snappy
	mainnetRawSSZCompressed []byte // 1.8Mb
	//go:embed testnet8.ssz.snappy
	testnet8RawSSZCompressed []byte // 1.8Mb
)

// State returns a copy of the genesis state from a hardcoded value.
func State(netName, genesisPath string) (state.BeaconState, error) {
	switch netName {
	case params.ConfigNames[params.Mainnet]:
		//to update snappy file use func GenerateSszSnappyState()
		if genesisPath == "" {
			return load(mainnetRawSSZCompressed)
		}
		return nil, nil
	case params.ConfigNames[params.Testnet8]:
		if genesisPath == "" {
			return load(testnet8RawSSZCompressed)
		}
		return nil, nil
	default:
		// No state found.
		return nil, nil
	}
}

// GenerateSszSnappyState generate predefined mainnet.ssz.snappy from genesis.
func GenerateSszSnappyState() error {
	serState, err := file.ReadFileAsBytes("/home/mezin/go/src/tesseract/.data/coordinator-genesis.ssz")
	if err != nil {
		return err
	}
	encodedState := snappy.Encode(nil, serState)
	err = file.WriteFile("/tmp/mainnet.ssz.snappy", []byte(encodedState))
	if err != nil {
		return err
	}
	return nil
}

// load a compressed ssz state file into a beacon state struct.
func load(b []byte) (state.BeaconState, error) {
	st := &ethpb.BeaconState{}
	b, err := snappy.Decode(nil /*dst*/, b)
	if err != nil {
		return nil, err
	}
	if err = st.UnmarshalSSZ(b); err != nil {
		return nil, err
	}
	return v1.InitializeFromProtoUnsafe(st)
}

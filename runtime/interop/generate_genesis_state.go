// Package interop contains deterministic utilities for generating
// genesis states and keys.
package interop

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/async"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/signing"
	coreState "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/transition"
	v1 "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/v1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/container/trie"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/bls"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/crypto/hash"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time"
	gwatCommon "gitlab.waterfall.network/waterfall/protocol/gwat/common"
)

// GenerateGenesisState deterministically given a genesis time and number of validators.
// If a genesis time of 0 is supplied it is set to the current time.
func GenerateGenesisState(ctx context.Context, genesisTime, numValidators uint64) (*ethpb.BeaconState, []*ethpb.Deposit, error) {
	privKeys, pubKeys, err := DeterministicallyGenerateKeys(0 /*startIndex*/, numValidators)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not deterministically generate keys for %d validators", numValidators)
	}
	depositDataItems, depositDataRoots, err := DepositDataFromKeys(privKeys, pubKeys)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate deposit data from keys")
	}
	return GenerateGenesisStateFromDepositData(ctx, gwatCommon.Hash{}, genesisTime, depositDataItems, depositDataRoots)
}

// GenerateGenesisStateFromDepositData creates a genesis state given a list of
// deposit data items and their corresponding roots.
func GenerateGenesisStateFromDepositData(ctx context.Context, gwtGenesisHash gwatCommon.Hash, genesisTime uint64, depositData []*ethpb.Deposit_Data, depositDataRoots [][]byte) (*ethpb.BeaconState, []*ethpb.Deposit, error) {
	itemsTrie, err := trie.GenerateTrieFromItems(depositDataRoots, params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate Merkle trie for deposit proofs")
	}
	deposits, err := GenerateDepositsFromData(depositData, itemsTrie)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate deposits from the deposit data provided")
	}
	root := itemsTrie.HashTreeRoot()
	if genesisTime == 0 {
		genesisTime = uint64(time.Now().Unix() + 60)
	}
	genesisCandidates := gwatCommon.HashArray{gwtGenesisHash}
	beaconState, err := coreState.GenesisBeaconState(ctx, deposits, genesisTime, &ethpb.Eth1Data{
		DepositRoot:  root[:],
		DepositCount: uint64(len(deposits)),
		BlockHash:    gwtGenesisHash.Bytes(),
		Candidates:   genesisCandidates.ToBytes(),
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate genesis state")
	}

	pbState, err := v1.ProtobufBeaconState(beaconState.CloneInnerState())
	if err != nil {
		return nil, nil, err
	}
	return pbState, deposits, nil
}

// GenerateDepositsFromData a list of deposit items by creating proofs for each of them from a sparse Merkle trie.
func GenerateDepositsFromData(depositDataItems []*ethpb.Deposit_Data, itemsTrie *trie.SparseMerkleTrie) ([]*ethpb.Deposit, error) {
	deposits := make([]*ethpb.Deposit, len(depositDataItems))
	results, err := async.Scatter(len(depositDataItems), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		return generateDepositsFromData(depositDataItems[offset:offset+entries], offset, itemsTrie)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate deposits from data")
	}
	for _, result := range results {
		if depositExtent, ok := result.Extent.([]*ethpb.Deposit); ok {
			copy(deposits[result.Offset:], depositExtent)
		} else {
			return nil, errors.New("extent not of expected type")
		}
	}
	return deposits, nil
}

// generateDepositsFromData a list of deposit items by creating proofs for each of them from a sparse Merkle trie.
func generateDepositsFromData(depositDataItems []*ethpb.Deposit_Data, offset int, trie *trie.SparseMerkleTrie) ([]*ethpb.Deposit, error) {
	deposits := make([]*ethpb.Deposit, len(depositDataItems))
	for i, item := range depositDataItems {
		if item.InitTxHash == nil {
			empyHash := [32]byte{}
			item.InitTxHash = empyHash[:]
		}

		proof, err := trie.MerkleProof(i + offset)
		if err != nil {
			return nil, errors.Wrapf(err, "could not generate proof for deposit %d", i+offset)
		}
		deposits[i] = &ethpb.Deposit{
			Proof: proof,
			Data:  item,
		}
	}
	return deposits, nil
}

// DepositDataFromKeys generates a list of deposit data items from a set of BLS validator keys.
func DepositDataFromKeys(privKeys []bls.SecretKey, pubKeys []bls.PublicKey) ([]*ethpb.Deposit_Data, [][]byte, error) {
	type depositData struct {
		items []*ethpb.Deposit_Data
		roots [][]byte
	}
	depositDataItems := make([]*ethpb.Deposit_Data, len(privKeys))
	depositDataRoots := make([][]byte, len(privKeys))
	results, err := async.Scatter(len(privKeys), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		items, roots, err := depositDataFromKeys(privKeys[offset:offset+entries], pubKeys[offset:offset+entries])
		return &depositData{items: items, roots: roots}, err
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate deposit data from keys")
	}
	for _, result := range results {
		if depositDataExtent, ok := result.Extent.(*depositData); ok {
			copy(depositDataItems[result.Offset:], depositDataExtent.items)
			copy(depositDataRoots[result.Offset:], depositDataExtent.roots)
		} else {
			return nil, nil, errors.New("extent not of expected type")
		}
	}
	return depositDataItems, depositDataRoots, nil
}

func depositDataFromKeys(privKeys []bls.SecretKey, pubKeys []bls.PublicKey) ([]*ethpb.Deposit_Data, [][]byte, error) {
	dataRoots := make([][]byte, len(privKeys))
	depositDataItems := make([]*ethpb.Deposit_Data, len(privKeys))
	for i := 0; i < len(privKeys); i++ {
		data, err := createDepositData(privKeys[i], pubKeys[i])
		if err != nil {
			return nil, nil, errors.Wrapf(err, "could not create deposit data for key: %#x", privKeys[i].Marshal())
		}
		h, err := data.HashTreeRoot()
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not hash tree root deposit data item")
		}
		dataRoots[i] = h[:]
		depositDataItems[i] = data
	}
	return depositDataItems, dataRoots, nil
}

// Generates a deposit data item from BLS keys and signs the hash tree root of the data.
func createDepositData(privKey bls.SecretKey, pubKey bls.PublicKey) (*ethpb.Deposit_Data, error) {
	depositMessage := &ethpb.DepositMessage{
		PublicKey:             pubKey.Marshal(),
		CreatorAddress:        withdrawalCredentialsHash(pubKey.Marshal()),
		WithdrawalCredentials: withdrawalCredentialsHash(pubKey.Marshal()),
	}
	sr, err := depositMessage.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return nil, err
	}
	root, err := (&ethpb.SigningData{ObjectRoot: sr[:], Domain: domain}).HashTreeRoot()
	if err != nil {
		return nil, err
	}
	di := &ethpb.Deposit_Data{
		PublicKey:             depositMessage.PublicKey,
		CreatorAddress:        depositMessage.CreatorAddress,
		WithdrawalCredentials: depositMessage.WithdrawalCredentials,
		Amount:                params.BeaconConfig().MaxEffectiveBalance,
		Signature:             privKey.Sign(root[:]).Marshal(),
		InitTxHash:            make([]byte, 32),
	}
	return di, nil
}

// withdrawalCredentialsHash forms a 20 byte hash of the withdrawal public
// address.
//
// The specification is as follows:
//
//	withdrawal_credentials[:1] == BLS_WITHDRAWAL_PREFIX_BYTE
//	withdrawal_credentials[1:] == hash(withdrawal_pubkey)[1:]
//
// where withdrawal_credentials is of type bytes32.
func withdrawalCredentialsHash(pubKey []byte) []byte {
	h := hash.Hash(pubKey)
	return append([]byte{blsWithdrawalPrefixByte}, h[1:]...)[:20]
}

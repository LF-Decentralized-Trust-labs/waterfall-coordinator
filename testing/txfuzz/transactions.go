package txfuzz

import (
	"context"
	"errors"
	"math/big"
	"math/rand"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/MariusVanDerWijden/FuzzyVM/generator"
	"gitlab.waterfall.network/waterfall/protocol/gwat/common"
	"gitlab.waterfall.network/waterfall/protocol/gwat/core/types"
	"gitlab.waterfall.network/waterfall/protocol/gwat/ethclient"
	"gitlab.waterfall.network/waterfall/protocol/gwat/rpc"
)

var (
	SK   = "0xcdfbe6f7602f67a97602e3e9fc24cde1cdffa88acd47745c0b84c5ff55891e1b"
	ADDR = "0xb02A2EdA1b317FBd16760128836B0Ac59B560e9D"
	SK2  = "0x8c04e41e317a7cf0cf4c2f7431d0a890a950f352df41ff6d053698df61a73bba"
)

// RandomCode creates a random byte code from the passed filler.
func RandomCode(f *filler.Filler) []byte {
	_, code := generator.GenerateProgram(f)
	return code
}

// RandomTx creates a random transaction.
func RandomTx(f *filler.Filler) (*types.Transaction, error) {
	nonce := uint64(rand.Int63())
	gasPrice := big.NewInt(rand.Int63())
	chainID := big.NewInt(rand.Int63())
	return RandomValidTx(nil, f, common.Address{}, nonce, gasPrice, chainID, false)
}

// RandomValidTx creates a random valid transaction.
// It does not mean that the transaction will succeed, but that it is well-formed.
// If gasPrice is not set, we will try to get it from the rpc
// If chainID is not set, we will try to get it from the rpc
func RandomValidTx(rpc *rpc.Client, f *filler.Filler, sender common.Address, nonce uint64, gasPrice, chainID *big.Int, al bool) (*types.Transaction, error) {
	// Set fields if non-nil
	if rpc != nil {
		client := ethclient.NewClient(rpc)
		var err error
		if gasPrice == nil {
			gasPrice, err = client.SuggestGasPrice(context.Background())
			if err != nil {
				gasPrice = big.NewInt(1)
			}
		}
		if chainID == nil {
			chainID, err = client.ChainID(context.Background())
			if err != nil {
				chainID = big.NewInt(1)
			}
		}
	}
	gas := uint64(100000)
	to := randomAddress()
	code := RandomCode(f)
	value := big.NewInt(0)
	if len(code) > 128 {
		code = code[:128]
	}
	mod := 10
	if al {
		mod = 6
	}
	switch f.Byte() % byte(mod) {
	case 0:
		// Legacy contract creation
		return types.NewContractCreation(nonce, value, gas, gasPrice, code), nil
	case 1:
		// Legacy transaction
		return types.NewTransaction(nonce, to, value, gas, gasPrice, code), nil
	case 2:
		// AccessList contract creation
		return newALTx(nonce, nil, gas, chainID, gasPrice, value, code, make(types.AccessList, 0)), nil
	case 3:
		// AccessList transaction
		return newALTx(nonce, &to, gas, chainID, gasPrice, value, code, make(types.AccessList, 0)), nil

	case 4:
		// 1559 contract creation
		tip, feecap, err := getCaps(rpc)
		if err != nil {
			return nil, err
		}
		return new1559Tx(nonce, nil, gas, chainID, tip, feecap, value, code, make(types.AccessList, 0)), nil
	case 5:
		// 1559 transaction
		tip, feecap, err := getCaps(rpc)
		if err != nil {
			return nil, err
		}
		return new1559Tx(nonce, &to, gas, chainID, tip, feecap, value, code, make(types.AccessList, 0)), nil
	case 6:
		// AccessList contract creation with AL
		tx := types.NewContractCreation(nonce, value, gas, gasPrice, code)
		al, err := CreateAccessList(rpc, tx, sender)
		if err != nil {
			return nil, err
		}
		return newALTx(nonce, nil, gas, chainID, gasPrice, value, code, *al), nil
	case 7:
		// AccessList transaction with AL
		tx := types.NewTransaction(nonce, to, value, gas, gasPrice, code)
		al, err := CreateAccessList(rpc, tx, sender)
		if err != nil {
			return nil, err
		}
		return newALTx(nonce, &to, gas, chainID, gasPrice, value, code, *al), nil
	case 8:
		// 1559 contract creation with AL
		tx := types.NewContractCreation(nonce, value, gas, gasPrice, code)
		al, err := CreateAccessList(rpc, tx, sender)
		if err != nil {
			return nil, err
		}
		tip, feecap, err := getCaps(rpc)
		if err != nil {
			return nil, err
		}
		return new1559Tx(nonce, nil, gas, chainID, tip, feecap, value, code, *al), nil
	case 9:
		// 1559 tx with AL
		tx := types.NewTransaction(nonce, to, value, gas, gasPrice, code)
		al, err := CreateAccessList(rpc, tx, sender)
		if err != nil {
			return nil, err
		}
		tip, feecap, err := getCaps(rpc)
		if err != nil {
			return nil, err
		}
		return new1559Tx(nonce, &to, gas, chainID, tip, feecap, value, code, *al), nil
	}
	return nil, errors.New("asdf")
}

func newALTx(nonce uint64, to *common.Address, gasLimit uint64, chainID, gasPrice, value *big.Int, code []byte, al types.AccessList) *types.Transaction {
	return types.NewTx(&types.AccessListTx{
		ChainID:    chainID,
		Nonce:      nonce,
		GasPrice:   gasPrice,
		Gas:        gasLimit,
		To:         to,
		Value:      value,
		Data:       code,
		AccessList: al,
	})
}

func new1559Tx(nonce uint64, to *common.Address, gasLimit uint64, chainID, tip, feeCap, value *big.Int, code []byte, al types.AccessList) *types.Transaction {
	return types.NewTx(&types.DynamicFeeTx{
		ChainID:    chainID,
		Nonce:      nonce,
		GasTipCap:  tip,
		GasFeeCap:  feeCap,
		Gas:        gasLimit,
		To:         to,
		Value:      value,
		Data:       code,
		AccessList: al,
	})
}

func getCaps(rpc *rpc.Client) (*big.Int, *big.Int, error) {
	client := ethclient.NewClient(rpc)
	tip, err := client.SuggestGasTipCap(context.Background())
	if err != nil {
		return nil, nil, err
	}
	feeCap, err := client.SuggestGasPrice(context.Background())
	return tip, feeCap, err
}

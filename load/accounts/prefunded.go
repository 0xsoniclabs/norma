package accounts

import (
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	PrefundedAccountsCount = 10_000
)

var PrefundedAccountBalanceWei = big.NewInt(1_000_000_000_000_000_000)

// DerivePrivateKey deterministically derives the private key of a prefunded load account.
func DerivePrivateKey(index uint64) (*ecdsa.PrivateKey, error) {
	seed := make([]byte, 16)
	binary.BigEndian.PutUint64(seed[:8], index)
	copy(seed[8:], []byte("norma-load"))

	keyBytes := crypto.Keccak256(seed)
	privateKey, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to derive private key for account %d: %w", index, err)
	}
	return privateKey, nil
}

// DeriveAddress returns the address corresponding to the deterministically derived account.
func DeriveAddress(index uint64) (common.Address, error) {
	privateKey, err := DerivePrivateKey(index)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(privateKey.PublicKey), nil
}

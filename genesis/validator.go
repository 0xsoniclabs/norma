package genesis

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/0xsoniclabs/sonic/inter/validatorpk"
	"github.com/0xsoniclabs/sonic/valkeystore"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func fakeKey(n uint32) (k *ecdsa.PrivateKey, retErr error) {
	defer func() {
		if err := recover(); err != nil {
			retErr = fmt.Errorf("failed to get key #%d; %v", n, err)
		}
	}()
	k = evmcore.FakeKey(n)
	return k, nil
}

// DeriveValidatorKey derives a fake validator private key, pubkey and address from validator ID.
func DeriveValidatorKey(id int) (privKeyHex string, pubKey string, address string, err error) {
	privateKeyECDSA, err := fakeKey(uint32(id))
	if err != nil {
		return "", "", "", err
	}

	privateKey := crypto.FromECDSA(privateKeyECDSA)
	publicKey := validatorpk.PubKey{
		Raw:  crypto.FromECDSAPub(&privateKeyECDSA.PublicKey),
		Type: validatorpk.Types.Secp256k1,
	}

	return hexutil.Encode(privateKey), publicKey.String(), crypto.PubkeyToAddress(privateKeyECDSA.PublicKey).String(), nil
}

// WriteValidatorKeystore writes a validator key to a sonic-compatible keystore in datastoreDir.
func WriteValidatorKeystore(privKeyHex, datastoreDir string) error {
	if datastoreDir == "" {
		return fmt.Errorf("datastore directory must not be empty")
	}

	privateKeyBytes, err := hexutil.Decode(privKeyHex)
	if err != nil {
		return fmt.Errorf("failed to decode private key hex: %w", err)
	}
	privateKeyECDSA, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	privateKey := crypto.FromECDSA(privateKeyECDSA)
	publicKey := validatorpk.PubKey{
		Raw:  crypto.FromECDSAPub(&privateKeyECDSA.PublicKey),
		Type: validatorpk.Types.Secp256k1,
	}

	valKeystore := valkeystore.NewDefaultFileRawKeystore(filepath.Join(datastoreDir, "keystore", "validator"))
	if err = valKeystore.Add(publicKey, privateKey, "password"); err != nil && !errors.Is(err, valkeystore.ErrAlreadyExists) {
		return fmt.Errorf("failed to create account: %w", err)
	}

	if _, err = valKeystore.Get(publicKey, "password"); err != nil {
		return fmt.Errorf("failed to decrypt the account: %w", err)
	}

	return nil
}

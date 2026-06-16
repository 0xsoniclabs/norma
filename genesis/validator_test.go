package genesis

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveValidatorKey_ReturnsData(t *testing.T) {
	privKey, pubKey, address, err := DeriveValidatorKey(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if privKey == "" {
		t.Fatalf("private key should not be empty")
	}
	if pubKey == "" {
		t.Fatalf("pubkey should not be empty")
	}
	if address == "" {
		t.Fatalf("address should not be empty")
	}
}

func TestWriteValidatorKeystore_CreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	privKey, _, _, err := DeriveValidatorKey(1)
	if err != nil {
		t.Fatalf("failed to derive key: %v", err)
	}

	if err := WriteValidatorKeystore(privKey, tmpDir); err != nil {
		t.Fatalf("failed to write keystore: %v", err)
	}

	keystoreDir := filepath.Join(tmpDir, "keystore", "validator")
	entries, err := os.ReadDir(keystoreDir)
	if err != nil {
		t.Fatalf("failed to read keystore dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least one keystore file in %s", keystoreDir)
	}
}

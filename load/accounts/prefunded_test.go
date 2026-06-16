package accounts
package accounts

import "testing"

func TestDeriveAddress_DeterministicAndDistinct(t *testing.T) {
	a0, err := DeriveAddress(0)
	if err != nil {
		t.Fatalf("failed to derive account 0: %v", err)
	}
	a0b, err := DeriveAddress(0)
	if err != nil {
		t.Fatalf("failed to derive account 0 second time: %v", err)
	}
	a1, err := DeriveAddress(1)
	if err != nil {
		t.Fatalf("failed to derive account 1: %v", err)
	}

	if a0 != a0b {
		t.Fatalf("address derivation is not deterministic: %s != %s", a0, a0b)
	}
	if a0 == a1 {
		t.Fatalf("different indices generated same address: %s", a0)
	}
}

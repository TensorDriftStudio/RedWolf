package crypto

import (
	"bytes"
	"testing"
)

func TestAES256GCM_RoundTrip(t *testing.T) {
	key := "my-datacenter-vault-key-2026"
	secret := []byte("RedWolfSuperSafeP@ss15")

	encrypted, err := VaultEncrypt(key, secret)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if encrypted == "" {
		t.Fatalf("encrypted ciphertext is empty")
	}

	decrypted, err := VaultDecrypt(key, encrypted)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, secret) {
		t.Fatalf("decrypted text mismatch: got %s, want %s", string(decrypted), string(secret))
	}
}

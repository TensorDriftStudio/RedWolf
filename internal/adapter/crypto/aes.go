package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// VaultEncrypt encrypts plaintext bytes with AES-256-GCM using the provided secret key.
func VaultEncrypt(keyPhrase string, plaintext []byte) (string, error) {
	key := deriveKey(keyPhrase)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed creating gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// VaultDecrypt decrypts base64 ciphertext using the secret key.
func VaultDecrypt(keyPhrase string, b64Ciphertext string) ([]byte, error) {
	key := deriveKey(keyPhrase)
	data, err := base64.StdEncoding.DecodeString(b64Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed creating gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (corrupted data or wrong key): %w", err)
	}

	return plaintext, nil
}

func deriveKey(keyPhrase string) []byte {
	if keyPhrase == "" {
		keyPhrase = "redwolf-default-appliance-vault-secret-key-32b"
	}
	hash := sha256.Sum256([]byte(keyPhrase))
	return hash[:]
}

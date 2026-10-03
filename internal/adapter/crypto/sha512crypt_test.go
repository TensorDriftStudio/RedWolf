package crypto_test

import (
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/adapter/crypto"
)

func TestHashSHA512Crypt(t *testing.T) {
	// Known test vector matching glibc / openssl passwd -6 -salt test saltpass
	password := "saltpass"
	salt := "test"
	expected := "$6$test$cuezCCPvWT63pod4oBmGnPnOftwtiJw4GinB6pxdMu/s7aPe8rvRhZ7Pryj.nitpgNZ909gMNnBYy.uk2bHt/0"

	hash := crypto.HashSHA512Crypt(password, salt)
	if hash != expected {
		t.Fatalf("expected hash %q, got %q", expected, hash)
	}

	// Test idempotent pass-through if already hashed
	if crypto.HashSHA512Crypt(expected, "othersalt") != expected {
		t.Fatalf("expected already hashed password to be returned unchanged")
	}

	// Test random salt generation
	hashRandom := crypto.HashSHA512Crypt("anotherpass", "")
	if len(hashRandom) < 20 || hashRandom[:3] != "$6$" {
		t.Fatalf("expected valid $6$ prefix with random salt, got %q", hashRandom)
	}
}

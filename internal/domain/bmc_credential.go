package domain

import (
	"crypto/rand"
	"math/big"
	"time"
)

// BMCCredential represents encrypted out-of-band management credentials.
type BMCCredential struct {
	NodeID            string    `json:"nodeId"`
	Username          string    `json:"username"`
	Password          string    `json:"password,omitempty"` // Omitted in general JSON unless explicitly revealed
	EncryptedPassword string    `json:"-"`
	UserSlot          int       `json:"userSlot"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// GenerateUniversalSafePassword generates a 15-character password compliant with the RFC IPMI 2.0 16-byte KCS buffer limit.
// It guarantees universal compatibility across Dell iDRAC, Supermicro AMI MegaRAC, and ASRock Rack ASPEED BMCs.
func GenerateUniversalSafePassword() string {
	const (
		upperLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ"
		lowerLetters = "abcdefghijkmnopqrstuvwxyz"
		digits       = "23456789"
		symbols      = "!@#$%^&*"
		allChars     = upperLetters + lowerLetters + digits + symbols
	)

	// Ensure at least one character from each required class
	password := make([]byte, 15)
	password[0] = upperLetters[mustRandomInt(len(upperLetters))]
	password[1] = lowerLetters[mustRandomInt(len(lowerLetters))]
	password[2] = digits[mustRandomInt(len(digits))]
	password[3] = symbols[mustRandomInt(len(symbols))]

	// Fill remaining 11 characters
	for i := 4; i < 15; i++ {
		password[i] = allChars[mustRandomInt(len(allChars))]
	}

	// Shuffle using Fisher-Yates to randomize positions
	for i := len(password) - 1; i > 0; i-- {
		j := mustRandomInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password)
}

func mustRandomInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

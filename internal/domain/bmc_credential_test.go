package domain

import (
	"strings"
	"testing"
	"unicode"
)

func TestGenerateUniversalSafePassword_RFCCompliance(t *testing.T) {
	const iterations = 1000
	const validSymbols = "!@#$%^&*"

	for i := 0; i < iterations; i++ {
		pwd := GenerateUniversalSafePassword()

		// Strict RFC length check (strictly 14 to 16 bytes)
		if len(pwd) != 15 {
			t.Fatalf("iteration %d: expected exactly 15 characters for BMC safety, got %d (%s)", i, len(pwd), pwd)
		}

		var hasUpper, hasLower, hasDigit, hasSymbol bool
		for _, r := range pwd {
			switch {
			case unicode.IsUpper(r):
				hasUpper = true
			case unicode.IsLower(r):
				hasLower = true
			case unicode.IsDigit(r):
				hasDigit = true
			case strings.ContainsRune(validSymbols, r):
				hasSymbol = true
			}
		}

		if !hasUpper {
			t.Fatalf("iteration %d: missing uppercase letter in %s", i, pwd)
		}
		if !hasLower {
			t.Fatalf("iteration %d: missing lowercase letter in %s", i, pwd)
		}
		if !hasDigit {
			t.Fatalf("iteration %d: missing digit in %s", i, pwd)
		}
		if !hasSymbol {
			t.Fatalf("iteration %d: missing symbol in %s", i, pwd)
		}
	}
}

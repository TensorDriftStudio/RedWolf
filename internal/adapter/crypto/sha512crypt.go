package crypto

import (
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"math/big"
	"strings"
)

const itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func b64from24bit(b2, b1, b0 byte, n int) string {
	w := uint32(b2)<<16 | uint32(b1)<<8 | uint32(b0)
	var sb strings.Builder
	for n > 0 {
		sb.WriteByte(itoa64[w&0x3f])
		w >>= 6
		n--
	}
	return sb.String()
}

// GenerateSalt creates an alphanumeric salt string for password hashing.
func GenerateSalt(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			result[i] = chars[i%len(chars)]
			continue
		}
		result[i] = chars[num.Int64()]
	}
	return string(result)
}

// HashSHA512Crypt computes a standard POSIX/glibc Unix SHA-512 crypt hash ($6$<salt>$<hash>).
// It is fully compliant with Ulrich Drepper's specification and compatible with shadow/Cloud-Init.
func HashSHA512Crypt(key, salt string) string {
	// If key is already a valid Unix crypt hash, return it directly
	if strings.HasPrefix(key, "$6$") || strings.HasPrefix(key, "$y$") || strings.HasPrefix(key, "$5$") {
		return key
	}

	if salt == "" {
		salt = GenerateSalt(16)
	}

	keyBytes := []byte(key)
	saltBytes := []byte(salt)
	if len(saltBytes) > 16 {
		saltBytes = saltBytes[:16]
	}

	// Step 1: Start digest A
	hA := sha512.New()
	hA.Write(keyBytes)
	hA.Write(saltBytes)

	// Step 2: Compute digest B
	hB := sha512.New()
	hB.Write(keyBytes)
	hB.Write(saltBytes)
	hB.Write(keyBytes)
	altResult := hB.Sum(nil)

	// Step 3: Add for each block of 64 bytes
	cnt := len(keyBytes)
	for cnt > 64 {
		hA.Write(altResult)
		cnt -= 64
	}
	hA.Write(altResult[:cnt])

	// Step 4: For each bit of key len
	cnt = len(keyBytes)
	for cnt > 0 {
		if cnt&1 != 0 {
			hA.Write(altResult)
		} else {
			hA.Write(keyBytes)
		}
		cnt >>= 1
	}
	digestA := hA.Sum(nil)

	// Step 5: Produce sequence P of length len(key)
	hP := sha512.New()
	for i := 0; i < len(keyBytes); i++ {
		hP.Write(keyBytes)
	}
	dP := hP.Sum(nil)
	seqP := make([]byte, len(keyBytes))
	for i := 0; i < len(keyBytes); i++ {
		seqP[i] = dP[i%64]
	}

	// Step 6: Produce sequence S of length len(salt)
	hS := sha512.New()
	for i := 0; i < 16+int(digestA[0]); i++ {
		hS.Write(saltBytes)
	}
	dS := hS.Sum(nil)
	seqS := make([]byte, len(saltBytes))
	for i := 0; i < len(saltBytes); i++ {
		seqS[i] = dS[i%64]
	}

	// Step 7: 5000 rounds
	curDigest := digestA
	for r := 0; r < 5000; r++ {
		hR := sha512.New()
		if r&1 != 0 {
			hR.Write(seqP)
		} else {
			hR.Write(curDigest)
		}
		if r%3 != 0 {
			hR.Write(seqS)
		}
		if r%7 != 0 {
			hR.Write(seqP)
		}
		if r&1 != 0 {
			hR.Write(curDigest)
		} else {
			hR.Write(seqP)
		}
		curDigest = hR.Sum(nil)
	}

	// Step 8: Permutation and encoding
	var b strings.Builder
	b.WriteString(b64from24bit(curDigest[0], curDigest[21], curDigest[42], 4))
	b.WriteString(b64from24bit(curDigest[22], curDigest[43], curDigest[1], 4))
	b.WriteString(b64from24bit(curDigest[44], curDigest[2], curDigest[23], 4))
	b.WriteString(b64from24bit(curDigest[3], curDigest[24], curDigest[45], 4))
	b.WriteString(b64from24bit(curDigest[25], curDigest[46], curDigest[4], 4))
	b.WriteString(b64from24bit(curDigest[47], curDigest[5], curDigest[26], 4))
	b.WriteString(b64from24bit(curDigest[6], curDigest[27], curDigest[48], 4))
	b.WriteString(b64from24bit(curDigest[28], curDigest[49], curDigest[7], 4))
	b.WriteString(b64from24bit(curDigest[50], curDigest[8], curDigest[29], 4))
	b.WriteString(b64from24bit(curDigest[9], curDigest[30], curDigest[51], 4))
	b.WriteString(b64from24bit(curDigest[31], curDigest[52], curDigest[10], 4))
	b.WriteString(b64from24bit(curDigest[53], curDigest[11], curDigest[32], 4))
	b.WriteString(b64from24bit(curDigest[12], curDigest[33], curDigest[54], 4))
	b.WriteString(b64from24bit(curDigest[34], curDigest[55], curDigest[13], 4))
	b.WriteString(b64from24bit(curDigest[56], curDigest[14], curDigest[35], 4))
	b.WriteString(b64from24bit(curDigest[15], curDigest[36], curDigest[57], 4))
	b.WriteString(b64from24bit(curDigest[37], curDigest[58], curDigest[16], 4))
	b.WriteString(b64from24bit(curDigest[59], curDigest[17], curDigest[38], 4))
	b.WriteString(b64from24bit(curDigest[18], curDigest[39], curDigest[60], 4))
	b.WriteString(b64from24bit(curDigest[40], curDigest[61], curDigest[19], 4))
	b.WriteString(b64from24bit(curDigest[62], curDigest[20], curDigest[41], 4))
	b.WriteString(b64from24bit(0, 0, curDigest[63], 2))

	return fmt.Sprintf("$6$%s$%s", string(saltBytes), b.String())
}

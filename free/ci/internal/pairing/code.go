package pairing

// Purpose: the pairing code (Epic D5): 10 characters of the RFC 4648 base32 alphabet from crypto/rand.
// Inputs:  none for GenerateCode; a code typed by a person for Normalize and Hash.
// Outputs: the code (50 bits), its SHA-256 hash (what the store keeps).
// Constraints: crypto/rand only; each character takes 5 bits of one random byte masked with 31, so the
//          alphabet is hit uniformly; Normalize upper-cases and drops spaces and dashes, so "abcd-efgh-jk"
//          typed in groups still works; a code is never logged and never placed on argv or in the env.

import (
	"crypto/rand"
	"crypto/sha256"
	"strings"
)

const (
	alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	// CodeLen is the number of characters in a code.
	CodeLen = 10
)

// GenerateCode returns a fresh pairing code.
func GenerateCode() (string, error) {
	b := make([]byte, CodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[b[i]&31]
	}
	return string(b), nil
}

// Normalize upper-cases a typed code and removes spaces and dashes.
func Normalize(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.ToUpper(code))
}

// HashCode is the SHA-256 of the normalized code.
func HashCode(code string) []byte {
	sum := sha256.Sum256([]byte(Normalize(code)))
	return sum[:]
}

// ValidShape reports whether code (normalized) has the right length and alphabet.
func ValidShape(code string) bool {
	code = Normalize(code)
	if len(code) != CodeLen {
		return false
	}
	for _, r := range code {
		if !strings.ContainsRune(alphabet, r) {
			return false
		}
	}
	return true
}

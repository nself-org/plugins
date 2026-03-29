package internal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// EncryptKeyMaterial encrypts plaintext key material using AES-256-CBC with
// a key derived from the master encryption key. Returns "iv_hex:ciphertext_hex".
func EncryptKeyMaterial(plaintext, masterKey string) (string, error) {
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate IV: %w", err)
	}

	key := deriveKeyFromMaster(masterKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	// PKCS7 pad the plaintext
	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)

	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return hex.EncodeToString(iv) + ":" + hex.EncodeToString(ciphertext), nil
}

// DecryptKeyMaterial decrypts "iv_hex:ciphertext_hex" back to plaintext using
// the master encryption key.
func DecryptKeyMaterial(encrypted, masterKey string) (string, error) {
	parts := strings.SplitN(encrypted, ":", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid encrypted format: expected iv:ciphertext")
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode IV: %w", err)
	}

	ciphertext, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	key := deriveKeyFromMaster(masterKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("ciphertext is not a multiple of block size")
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	unpadded, err := pkcs7Unpad(plaintext, aes.BlockSize)
	if err != nil {
		return "", fmt.Errorf("unpad: %w", err)
	}

	return string(unpadded), nil
}

// GenerateToken creates an HMAC-SHA256 signed JWT-like token from a payload.
// Format: base64url(header).base64url(payload).base64url(hmac_signature)
func GenerateToken(payload map[string]interface{}, signingKey string) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	sigInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(sigInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return headerB64 + "." + payloadB64 + "." + signature, nil
}

// HashToken computes a deterministic HMAC-SHA256 hash of a token for storage.
func HashToken(token string) string {
	mac := hmac.New(sha256.New, []byte("token-hash"))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateRandomHex generates n random bytes and returns their hex encoding.
func GenerateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateRandomBytes generates n random bytes.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate random bytes: %w", err)
	}
	return b, nil
}

// ConstantTimeEqual compares two byte slices in constant time (timing-safe).
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// deriveKeyFromMaster derives a 32-byte AES-256 key from the master key using HMAC-SHA256.
func deriveKeyFromMaster(masterKey string) []byte {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte("key-material"))
	return mac.Sum(nil)
}

// pkcs7Pad pads data to a multiple of blockSize using PKCS#7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padByte := byte(padding)
	for i := 0; i < padding; i++ {
		data = append(data, padByte)
	}
	return data
}

// pkcs7Unpad removes PKCS#7 padding from data.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("invalid padded data length")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize {
		return nil, errors.New("invalid padding value")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("invalid padding bytes")
		}
	}
	return data[:len(data)-padding], nil
}

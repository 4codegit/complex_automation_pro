// Package auth implements local password hashing and session tokens
// (TZ §12). Passwords use PBKDF2-HMAC-SHA256 from the standard library
// (Go 1.24+); sessions are opaque 256-bit tokens delivered as an HttpOnly
// cookie, stored server-side by their SHA-256 hash.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	hashAlgorithm = "pbkdf2-sha256"
	hashIter      = 60000
	hashKeyLen    = 32
	saltLen       = 16
)

// HashPassword encodes a password for storage. Format:
// pbkdf2-sha256$<iter>$<salt-b64>$<hash-b64>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, hashIter, hashKeyLen)
	if err != nil {
		return "", fmt.Errorf("pbkdf2: %w", err)
	}
	return fmt.Sprintf("%s$%d$%s$%s",
		hashAlgorithm, hashIter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks a password against a stored hash in constant time
// semantics (the underlying comparison is fixed-length after decode).
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != hashAlgorithm {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > 10_000_000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) != hashKeyLen {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	difference := 0
	for i := range want {
		difference |= int(got[i] ^ want[i])
	}
	return difference == 0
}

// NewToken returns a fresh random 256-bit session token (hex).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken derives the server-side storage key for a session token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

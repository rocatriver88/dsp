package user

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// GenerateTempPassword returns (plaintext, bcryptHash, error). The plaintext
// is hex-encoded random bytes and should be shown to the recipient exactly
// once; only the hash is persisted to the users table.
func GenerateTempPassword(nBytes int) (plain string, hash string, err error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate random: %w", err)
	}
	plain = hex.EncodeToString(b)
	h, err := bcrypt.GenerateFromPassword([]byte(plain), 10)
	if err != nil {
		return "", "", fmt.Errorf("hash temp password: %w", err)
	}
	return plain, string(h), nil
}

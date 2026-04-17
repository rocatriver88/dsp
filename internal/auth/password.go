package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of the given password at cost 10.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	return string(hash), err
}

// CheckPassword compares a bcrypt hash with a plaintext password.
// Returns nil on success, an error on mismatch.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

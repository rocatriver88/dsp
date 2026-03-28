package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndValidate(t *testing.T) {
	secret := "test-secret-key"
	token := GenerateToken(secret, "123", "req-abc")

	if !ValidateToken(secret, token, "123", "req-abc") {
		t.Fatal("valid token rejected")
	}
}

func TestRejectWrongSecret(t *testing.T) {
	token := GenerateToken("secret-a", "123", "req-abc")
	if ValidateToken("secret-b", token, "123", "req-abc") {
		t.Fatal("wrong secret accepted")
	}
}

func TestRejectTamperedParams(t *testing.T) {
	secret := "test-secret"
	token := GenerateToken(secret, "123", "req-abc")
	if ValidateToken(secret, token, "456", "req-abc") {
		t.Fatal("tampered params accepted")
	}
}

func TestRejectExpiredToken(t *testing.T) {
	// Manually create a token with an old timestamp
	secret := "test-secret"
	oldTS := time.Now().Add(-10 * time.Minute).Unix()
	token := generateWithTS(secret, oldTS, "123")
	if ValidateToken(secret, token, "123") {
		t.Fatal("expired token accepted")
	}
}

func TestRejectMalformedToken(t *testing.T) {
	if ValidateToken("secret", "", "123") {
		t.Fatal("empty token accepted")
	}
	if ValidateToken("secret", "not-a-token", "123") {
		t.Fatal("malformed token accepted")
	}
	if ValidateToken("secret", "abc:def", "123") {
		t.Fatal("non-numeric timestamp accepted")
	}
}

// helper: generate token with a specific timestamp for expiry testing
func generateWithTS(secret string, ts int64, params ...string) string {
	tsStr := fmt.Sprintf("%d", ts)
	payload := tsStr + ":" + strings.Join(params, ":")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return tsStr + ":" + sig
}

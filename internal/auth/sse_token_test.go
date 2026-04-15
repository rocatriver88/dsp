package auth

import (
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-long-enough-for-hmac-12345678")

func TestIssueAndValidateSSEToken_HappyPath(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	if tok == "" {
		t.Fatal("IssueSSEToken returned empty token")
	}
	if !strings.Contains(tok, ".") {
		t.Fatalf("token should contain . separator, got %q", tok)
	}
	advID, err := ValidateSSEToken(testSecret, tok, now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if advID != 42 {
		t.Fatalf("expected advID 42, got %d", advID)
	}
}

func TestValidateSSEToken_Expired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	if _, err := ValidateSSEToken(testSecret, tok, now.Add(6*time.Minute)); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestValidateSSEToken_WrongSecret(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	other := []byte("other-secret-long-enough-for-hmac-1234567")
	if _, err := ValidateSSEToken(other, tok, now); err == nil {
		t.Fatal("expected wrong-secret token to be rejected")
	}
}

func TestValidateSSEToken_MalformedTokens(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cases := []string{
		"",
		"notoken",
		"bogus.signature.extra",
		"aGVsbG8.",
		".notsigned",
		"!!!.!!!",
	}
	for _, c := range cases {
		if _, err := ValidateSSEToken(testSecret, c, now); err == nil {
			t.Errorf("expected malformed token %q to be rejected", c)
		}
	}
}

func TestValidateSSEToken_TamperedPayload(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	idx := strings.Index(tok, ".")
	if idx < 1 {
		t.Fatal("unexpected token format")
	}
	// Flip first payload character via byte arithmetic
	tampered := string(rune(tok[0]+1)) + tok[1:]
	if _, err := ValidateSSEToken(testSecret, tampered, now); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

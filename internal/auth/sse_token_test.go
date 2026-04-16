package auth

import (
	"encoding/base64"
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

// TestValidateSSEToken_TamperedPayload exercises the signature-mismatch
// branch explicitly: it decodes the payload, flips a byte in the decoded
// bytes (changing advID from '4' to '5'), re-encodes the tampered payload
// as base64url, and concatenates the ORIGINAL (unchanged) signature. The
// result is a syntactically valid token whose signature provably does not
// match the tampered payload, so the failure cannot short-circuit through
// the "malformed base64" path.
//
// The error message assertion enforces the oracle-free property: all
// non-expiry failures must return the generic "invalid token" so an
// attacker cannot distinguish signature mismatch from parse failure.
func TestValidateSSEToken_TamperedPayload(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)

	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected token format: %q", tok)
	}
	origPayloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode original payload: %v", err)
	}

	tamperedPayload := make([]byte, len(origPayloadBytes))
	copy(tamperedPayload, origPayloadBytes)
	if tamperedPayload[0] == '4' {
		tamperedPayload[0] = '5'
	} else {
		tamperedPayload[0]++ // fallback in case test data changes
	}
	tampered := base64.RawURLEncoding.EncodeToString(tamperedPayload) + "." + parts[1]

	_, err = ValidateSSEToken(testSecret, tampered, now)
	if err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
	if err.Error() != "invalid token" {
		t.Errorf("expected generic 'invalid token' error (oracle-free), got %q", err.Error())
	}
}

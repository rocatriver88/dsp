package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// IssueSSEToken returns a short-lived HMAC-signed token that binds an
// advertiser ID to an expiry time. The token format is:
//
//	base64url(<advID>:<expUnix>) + "." + hex(HMAC-SHA256(secret, <advID>:<expUnix>))
//
// The raw payload (before base64url) is what gets signed, so validation
// decodes the payload, verifies the signature, then parses advID/exp.
//
// This is distinct from the bidder's click/win token (auth.GenerateToken
// and ValidateToken) on cryptographic-hygiene grounds: analytics SSE and
// bidder callbacks are different trust domains and must not share a
// signing key or signature format. See V5.1 hotfix P1-1.
func IssueSSEToken(secret []byte, advID int64, ttl time.Duration, now time.Time) string {
	payload := fmt.Sprintf("%d:%d", advID, now.Add(ttl).Unix())
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

// ValidateSSEToken parses and verifies an SSE token. It returns the
// authenticated advertiser ID on success. Errors cover malformed tokens,
// signature mismatches, and expired tokens. HMAC comparison is
// constant-time to prevent timing oracles.
func ValidateSSEToken(secret []byte, token string, now time.Time) (int64, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, errors.New("invalid token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, errors.New("invalid token")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payloadBytes)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return 0, errors.New("invalid token")
	}
	idStr, expStr, ok := strings.Cut(string(payloadBytes), ":")
	if !ok || idStr == "" || expStr == "" {
		return 0, errors.New("invalid token")
	}
	advID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || advID <= 0 {
		return 0, errors.New("invalid token")
	}
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return 0, errors.New("invalid token")
	}
	if now.Unix() >= expUnix {
		return 0, errors.New("token expired")
	}
	return advID, nil
}

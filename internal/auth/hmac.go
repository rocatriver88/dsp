package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const tokenMaxAge = 5 * time.Minute

// GenerateToken creates an HMAC-SHA256 token signing the given params with a timestamp.
// Returns "timestamp:signature" string.
func GenerateToken(secret string, params ...string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := ts + ":" + strings.Join(params, ":")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return ts + ":" + sig
}

// ValidateToken checks that the token was signed with the given secret and params,
// and that the timestamp is within tokenMaxAge.
func ValidateToken(secret, token string, params ...string) bool {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return false
	}
	tsStr, sig := parts[0], parts[1]

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Since(time.Unix(ts, 0)) > tokenMaxAge {
		return false
	}

	payload := tsStr + ":" + strings.Join(params, ":")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

// FormatTokenParams builds the canonical parameter string for HMAC signing.
func FormatTokenParams(campaignID, requestID string) string {
	return fmt.Sprintf("%s:%s", campaignID, requestID)
}

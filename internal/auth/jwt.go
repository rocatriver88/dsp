package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role constants — duplicated here until internal/user lands.
// When internal/user/model.go exists, these should be removed and imported from there.
const (
	RolePlatformAdmin = "platform_admin"
	RoleAdvertiser    = "advertiser"
)

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 7 * 24 * time.Hour
)

// Claims is the JWT payload for access tokens.
type Claims struct {
	UserID       int64  `json:"uid"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	AdvertiserID int64  `json:"aid,omitempty"`
	jwt.RegisteredClaims
}

// IssueAccessToken creates a signed HS256 access token with 15-min TTL.
func IssueAccessToken(secret []byte, userID int64, email, role string, advertiserID *int64) (string, error) {
	var aid int64
	if advertiserID != nil {
		aid = *advertiserID
	}
	return issueJWT(secret, userID, email, role, aid, AccessTokenTTL)
}

// IssueRefreshToken creates a signed HS256 refresh token with 7-day TTL.
// Only carries the subject (user ID) — no custom claims.
func IssueRefreshToken(secret []byte, userID int64) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(RefreshTokenTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ValidateJWT parses and validates an HS256 JWT, returning the Claims on success.
func ValidateJWT(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// issueJWT is the internal helper that both IssueAccessToken and tests use.
func issueJWT(secret []byte, userID int64, email, role string, aid int64, ttl time.Duration) (string, error) {
	claims := &Claims{
		UserID:       userID,
		Email:        email,
		Role:         role,
		AdvertiserID: aid,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

package auth

import (
	"testing"
	"time"
)

var jwtTestSecret = []byte("test-jwt-secret-at-least-32bytes!!")

func TestIssueAccessToken_AdvertiserClaims(t *testing.T) {
	aid := int64(42)
	token, err := IssueAccessToken(jwtTestSecret, 1, "user@test.com", RoleAdvertiser, &aid)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateJWT(token, jwtTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 1 {
		t.Errorf("uid: got %d want 1", claims.UserID)
	}
	if claims.Role != RoleAdvertiser {
		t.Errorf("role: got %s", claims.Role)
	}
	if claims.AdvertiserID != 42 {
		t.Errorf("aid: got %d want 42", claims.AdvertiserID)
	}
}

func TestIssueAccessToken_AdminClaims(t *testing.T) {
	token, err := IssueAccessToken(jwtTestSecret, 1, "admin@test.com", RolePlatformAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateJWT(token, jwtTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.AdvertiserID != 0 {
		t.Errorf("admin should have aid=0, got %d", claims.AdvertiserID)
	}
}

func TestValidateJWT_Expired(t *testing.T) {
	token, _ := issueJWT(jwtTestSecret, 1, "u@t.com", "advertiser", 0, -1*time.Hour)
	_, err := ValidateJWT(token, jwtTestSecret)
	if err == nil {
		t.Fatal("expired token should fail")
	}
}

func TestValidateJWT_WrongKey(t *testing.T) {
	token, _ := IssueAccessToken(jwtTestSecret, 1, "u@t.com", "advertiser", nil)
	_, err := ValidateJWT(token, []byte("wrong-secret-wrong-secret-wrong!!"))
	if err == nil {
		t.Fatal("wrong key should fail")
	}
}

func TestValidateJWT_Malformed(t *testing.T) {
	_, err := ValidateJWT("not.a.jwt", jwtTestSecret)
	if err == nil {
		t.Fatal("malformed token should fail")
	}
}

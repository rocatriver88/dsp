package auth

import "testing"

func TestHashPassword_ProducesValidHash(t *testing.T) {
	hash, err := HashPassword("testpass123")
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) < 50 {
		t.Fatalf("hash too short: %s", hash)
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	hash, _ := HashPassword("testpass123")
	if err := CheckPassword(hash, "testpass123"); err != nil {
		t.Fatalf("correct password should pass: %v", err)
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("testpass123")
	if err := CheckPassword(hash, "wrongpass"); err == nil {
		t.Fatal("wrong password should fail")
	}
}

package registration

import "testing"

func TestBlockedDomains(t *testing.T) {
	blocked := []string{
		"mailinator.com",
		"guerrillamail.com",
		"tempmail.com",
		"throwaway.email",
		"yopmail.com",
	}
	for _, d := range blocked {
		if !blockedDomains[d] {
			t.Errorf("expected %s to be blocked", d)
		}
	}
}

func TestBlockedDomains_Legitimate(t *testing.T) {
	legitimate := []string{
		"gmail.com",
		"company.com",
		"qq.com",
		"163.com",
	}
	for _, d := range legitimate {
		if blockedDomains[d] {
			t.Errorf("expected %s to not be blocked", d)
		}
	}
}

func TestEmailValidation(t *testing.T) {
	tests := []struct {
		email   string
		wantErr bool
		errMsg  string
	}{
		{"user@gmail.com", false, ""},
		{"user@company.cn", false, ""},
		{"invalid-email", true, "invalid email"},
		{"user@mailinator.com", true, "email domain not allowed"},
		{"user@yopmail.com", true, "email domain not allowed"},
		{"@nodomain.com", true, "invalid email"},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			err := validateEmail(tt.email)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %s", tt.email)
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error for %s: %v", tt.email, err)
			}
		})
	}
}

func TestGenerateAPIKey_Format(t *testing.T) {
	key := generateAPIKey()
	if len(key) == 0 {
		t.Fatal("empty API key")
	}
	if key[:4] != "dsp_" {
		t.Errorf("API key should start with dsp_, got %s", key[:4])
	}
	if len(key) != 68 { // "dsp_" + 64 hex chars
		t.Errorf("expected key length 68, got %d", len(key))
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	key1 := generateAPIKey()
	key2 := generateAPIKey()
	if key1 == key2 {
		t.Error("generated keys should be unique")
	}
}

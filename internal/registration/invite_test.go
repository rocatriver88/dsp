package registration

import "testing"

func TestGenerateInviteCode(t *testing.T) {
	code := GenerateInviteCode()
	if len(code) < 8 {
		t.Errorf("invite code too short: %s", code)
	}
	code2 := GenerateInviteCode()
	if code == code2 {
		t.Error("two generated codes should not be equal")
	}
}

func TestInviteCodeFormat(t *testing.T) {
	code := GenerateInviteCode()
	for _, c := range code {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("invite code contains invalid char: %c", c)
		}
	}
}

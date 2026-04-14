package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/campaign"
)

// TestAdvertiserResponse_OmitsAPIKey guards the top security property of the
// P0 tenant-isolation fix: no read path may serialize api_key.
func TestAdvertiserResponse_OmitsAPIKey(t *testing.T) {
	adv := &campaign.Advertiser{
		ID:           42,
		CompanyName:  "Acme",
		ContactEmail: "ops@acme.test",
		APIKey:       "dsp_THIS_MUST_NOT_LEAK",
		BalanceCents: 100000,
		BillingType:  "prepaid",
		CreatedAt:    time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
	}

	resp := NewAdvertiserResponse(adv)
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	body := string(data)
	if strings.Contains(body, "api_key") {
		t.Errorf("api_key field leaked into response body: %s", body)
	}
	if strings.Contains(body, "dsp_THIS_MUST_NOT_LEAK") {
		t.Errorf("api key value leaked into response body: %s", body)
	}
}

// TestAdvertiserResponse_PreservesPublicFields asserts that the sanitized
// response still carries every field a client legitimately needs.
func TestAdvertiserResponse_PreservesPublicFields(t *testing.T) {
	adv := &campaign.Advertiser{
		ID:              7,
		CompanyName:     "Acme",
		ContactEmail:    "ops@acme.test",
		APIKey:          "hidden",
		BalanceCents:    2500,
		BillingType:     "postpaid",
		ActiveCampaigns: 3,
		TotalSpentCents: 1000,
	}

	resp := NewAdvertiserResponse(adv)
	if resp.ID != 7 || resp.CompanyName != "Acme" || resp.ContactEmail != "ops@acme.test" {
		t.Errorf("identity fields lost: %+v", resp)
	}
	if resp.BalanceCents != 2500 || resp.BillingType != "postpaid" {
		t.Errorf("billing fields lost: %+v", resp)
	}
	if resp.ActiveCampaigns != 3 || resp.TotalSpentCents != 1000 {
		t.Errorf("activity fields lost: %+v", resp)
	}
}

// TestAdvertiserCreatedResponse_MatchesLegacyShape ensures the create-path
// response still has {id, api_key, message} so existing clients don't break.
func TestAdvertiserCreatedResponse_MatchesLegacyShape(t *testing.T) {
	resp := AdvertiserCreatedResponse{
		ID:      42,
		APIKey:  "dsp_new_key",
		Message: "advertiser created",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["id"]; !ok {
		t.Error("missing id field")
	}
	if _, ok := decoded["api_key"]; !ok {
		t.Error("missing api_key field (this is the one-time disclosure path)")
	}
	if _, ok := decoded["message"]; !ok {
		t.Error("missing message field")
	}
}

// TestRegistrationApprovedResponse_MatchesLegacyShape guards the admin
// approval response shape {advertiser_id, api_key, message}.
func TestRegistrationApprovedResponse_MatchesLegacyShape(t *testing.T) {
	resp := RegistrationApprovedResponse{
		AdvertiserID: 42,
		APIKey:       "dsp_new_key",
		Message:      "Registration approved. Advertiser account created.",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["advertiser_id"]; !ok {
		t.Error("missing advertiser_id field")
	}
	if _, ok := decoded["api_key"]; !ok {
		t.Error("missing api_key field")
	}
	if _, ok := decoded["message"]; !ok {
		t.Error("missing message field")
	}
}

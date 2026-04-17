package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
)

func TestEntryJSON(t *testing.T) {
	e := Entry{
		AdvertiserID: 1,
		Actor:        "admin",
		Action:       "campaign.update",
		ResourceType: "campaign",
		ResourceID:   42,
		Details:      map[string]any{"field": "budget_daily_cents", "old": 1000, "new": 2000},
		CreatedAt:    time.Now(),
	}
	if e.Actor != "admin" {
		t.Errorf("expected actor 'admin', got '%s'", e.Actor)
	}
	if e.Action != "campaign.update" {
		t.Errorf("expected action 'campaign.update', got '%s'", e.Action)
	}
}

func TestActions(t *testing.T) {
	actions := []string{
		ActionCampaignCreate, ActionCampaignUpdate, ActionCampaignStart,
		ActionCampaignPause, ActionCreativeCreate, ActionCreativeDelete,
		ActionTopUp, ActionRegistrationApprove,
	}
	for _, a := range actions {
		if a == "" {
			t.Error("action constant should not be empty")
		}
	}
}

// --- ActorFromRequest tests ---

func TestActorFromRequest_JWTAdmin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/topup", nil)
	ctx := auth.WithUser(req.Context(), &auth.User{
		ID:    1,
		Email: "admin@test.com",
		Role:  auth.RolePlatformAdmin,
	})
	req = req.WithContext(ctx)

	actor, userID := ActorFromRequest(req)

	if actor != "user:1" {
		t.Errorf("expected actor='user:1', got %q", actor)
	}
	if userID == nil {
		t.Fatal("expected non-nil userID for JWT admin")
	}
	if *userID != 1 {
		t.Errorf("expected userID=1, got %d", *userID)
	}
}

func TestActorFromRequest_JWTAdvertiser(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/topup", nil)
	ctx := auth.WithUser(req.Context(), &auth.User{
		ID:           5,
		Email:        "user@test.com",
		Role:         auth.RoleAdvertiser,
		AdvertiserID: 42,
	})
	ctx = auth.WithAdvertiser(ctx, &auth.Advertiser{ID: 42})
	req = req.WithContext(ctx)

	actor, userID := ActorFromRequest(req)

	if actor != "user:5" {
		t.Errorf("expected actor='user:5', got %q", actor)
	}
	if userID == nil {
		t.Fatal("expected non-nil userID for JWT advertiser")
	}
	if *userID != 5 {
		t.Errorf("expected userID=5, got %d", *userID)
	}
}

func TestActorFromRequest_APIKeyOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/topup", nil)
	ctx := auth.WithAdvertiser(req.Context(), &auth.Advertiser{
		ID:           42,
		CompanyName:  "Test Corp",
		ContactEmail: "test@test.com",
	})
	req = req.WithContext(ctx)

	actor, userID := ActorFromRequest(req)

	if actor != "apikey:42" {
		t.Errorf("expected actor='apikey:42', got %q", actor)
	}
	if userID != nil {
		t.Errorf("expected nil userID for API key path, got %d", *userID)
	}
}

func TestActorFromRequest_AdminTokenService(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/topup", nil)

	actor, userID := ActorFromRequest(req)

	if actor != "service:admin-token" {
		t.Errorf("expected actor='service:admin-token', got %q", actor)
	}
	if userID != nil {
		t.Errorf("expected nil userID for admin-token path, got %d", *userID)
	}
}

func TestActorFromRequest_UserTakesPrecedenceOverAdvertiser(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	ctx := auth.WithAdvertiser(req.Context(), &auth.Advertiser{ID: 42})
	ctx = auth.WithUser(ctx, &auth.User{ID: 7, Role: auth.RoleAdvertiser, AdvertiserID: 42})
	req = req.WithContext(ctx)

	actor, userID := ActorFromRequest(req)

	if actor != "user:7" {
		t.Errorf("expected user to take precedence, got actor=%q", actor)
	}
	if userID == nil || *userID != 7 {
		t.Errorf("expected userID=7")
	}
}

package campaign

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// CreateAdvertiser creates a new advertiser and returns the ID.
func (s *Store) CreateAdvertiser(ctx context.Context, adv *Advertiser) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO advertisers (company_name, contact_email, api_key, balance_cents, billing_type)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		adv.CompanyName, adv.ContactEmail, adv.APIKey, adv.BalanceCents, adv.BillingType,
	).Scan(&id)
	return id, err
}

// GetAdvertiser returns an advertiser by ID.
func (s *Store) GetAdvertiser(ctx context.Context, id int64) (*Advertiser, error) {
	adv := &Advertiser{}
	err := s.db.QueryRow(ctx,
		`SELECT id, company_name, contact_email, api_key, balance_cents, billing_type, created_at, updated_at
		 FROM advertisers WHERE id = $1`, id,
	).Scan(&adv.ID, &adv.CompanyName, &adv.ContactEmail, &adv.APIKey,
		&adv.BalanceCents, &adv.BillingType, &adv.CreatedAt, &adv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return adv, nil
}

// GetAdvertiserByAPIKey returns an advertiser by API key.
func (s *Store) GetAdvertiserByAPIKey(ctx context.Context, key string) (*Advertiser, error) {
	adv := &Advertiser{}
	err := s.db.QueryRow(ctx,
		`SELECT id, company_name, contact_email, api_key, balance_cents, billing_type, created_at, updated_at
		 FROM advertisers WHERE api_key = $1`, key,
	).Scan(&adv.ID, &adv.CompanyName, &adv.ContactEmail, &adv.APIKey,
		&adv.BalanceCents, &adv.BillingType, &adv.CreatedAt, &adv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return adv, nil
}

// CreateCampaign creates a new campaign in draft status.
func (s *Store) CreateCampaign(ctx context.Context, c *Campaign) (int64, error) {
	if c.Targeting == nil {
		c.Targeting = json.RawMessage(`{}`)
	}
	if c.BillingModel == "" {
		c.BillingModel = BillingCPM
	}
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO campaigns (advertiser_id, name, status, billing_model, budget_total_cents, budget_daily_cents,
		  bid_cpm_cents, bid_cpc_cents, ocpm_target_cpa_cents, start_date, end_date, targeting)
		 VALUES ($1, $2, 'draft', $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		c.AdvertiserID, c.Name, c.BillingModel, c.BudgetTotalCents, c.BudgetDailyCents,
		c.BidCPMCents, c.BidCPCCents, c.OCPMTargetCPACents, c.StartDate, c.EndDate, c.Targeting,
	).Scan(&id)
	return id, err
}

// GetCampaign returns a campaign by ID.
func (s *Store) GetCampaign(ctx context.Context, id int64) (*Campaign, error) {
	c := &Campaign{}
	err := s.db.QueryRow(ctx,
		`SELECT id, advertiser_id, name, status, billing_model, budget_total_cents, budget_daily_cents,
		        spent_cents, bid_cpm_cents, bid_cpc_cents, ocpm_target_cpa_cents,
		        start_date, end_date, targeting, created_at, updated_at
		 FROM campaigns WHERE id = $1 AND status != 'deleted'`, id,
	).Scan(&c.ID, &c.AdvertiserID, &c.Name, &c.Status, &c.BillingModel,
		&c.BudgetTotalCents, &c.BudgetDailyCents, &c.SpentCents,
		&c.BidCPMCents, &c.BidCPCCents, &c.OCPMTargetCPACents,
		&c.StartDate, &c.EndDate, &c.Targeting, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ListCampaigns returns all campaigns for an advertiser.
func (s *Store) ListCampaigns(ctx context.Context, advertiserID int64) ([]*Campaign, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, advertiser_id, name, status, billing_model, budget_total_cents, budget_daily_cents,
		        spent_cents, bid_cpm_cents, bid_cpc_cents, ocpm_target_cpa_cents,
		        start_date, end_date, targeting, created_at, updated_at
		 FROM campaigns WHERE advertiser_id = $1 AND status != 'deleted'
		 ORDER BY created_at DESC`, advertiserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var campaigns []*Campaign
	for rows.Next() {
		c := &Campaign{}
		if err := rows.Scan(&c.ID, &c.AdvertiserID, &c.Name, &c.Status, &c.BillingModel,
			&c.BudgetTotalCents, &c.BudgetDailyCents, &c.SpentCents,
			&c.BidCPMCents, &c.BidCPCCents, &c.OCPMTargetCPACents,
			&c.StartDate, &c.EndDate, &c.Targeting, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, nil
}

// GetCampaignForAdvertiser returns a campaign scoped to an advertiser (IDOR-safe).
func (s *Store) GetCampaignForAdvertiser(ctx context.Context, id, advertiserID int64) (*Campaign, error) {
	c := &Campaign{}
	err := s.db.QueryRow(ctx,
		`SELECT id, advertiser_id, name, status, billing_model, budget_total_cents, budget_daily_cents,
		        spent_cents, bid_cpm_cents, bid_cpc_cents, ocpm_target_cpa_cents,
		        start_date, end_date, targeting, created_at, updated_at
		 FROM campaigns WHERE id = $1 AND advertiser_id = $2 AND status != 'deleted'`, id, advertiserID,
	).Scan(&c.ID, &c.AdvertiserID, &c.Name, &c.Status, &c.BillingModel,
		&c.BudgetTotalCents, &c.BudgetDailyCents, &c.SpentCents,
		&c.BidCPMCents, &c.BidCPCCents, &c.OCPMTargetCPACents,
		&c.StartDate, &c.EndDate, &c.Targeting, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateCampaign updates mutable fields, scoped to advertiser (IDOR-safe).
func (s *Store) UpdateCampaign(ctx context.Context, id, advertiserID int64, name string, bidCPM int, budgetDaily int64, targeting json.RawMessage) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE campaigns SET name = $2, bid_cpm_cents = $3, budget_daily_cents = $4, targeting = $5, updated_at = NOW()
		 WHERE id = $1 AND advertiser_id = $6 AND status != 'deleted'`,
		id, name, bidCPM, budgetDaily, targeting, advertiserID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("campaign not found")
	}
	return nil
}

// TransitionStatus changes campaign status with validation, scoped to advertiser (IDOR-safe).
func (s *Store) TransitionStatus(ctx context.Context, id, advertiserID int64, to Status) error {
	c, err := s.GetCampaignForAdvertiser(ctx, id, advertiserID)
	if err != nil {
		return fmt.Errorf("campaign not found: %w", err)
	}

	if err := ValidateTransition(c.Status, to); err != nil {
		return err
	}

	_, err = s.db.Exec(ctx,
		`UPDATE campaigns SET status = $2, updated_at = NOW() WHERE id = $1 AND advertiser_id = $3`,
		id, to, advertiserID,
	)
	return err
}

// TransitionStatusInternal changes campaign status without advertiser scoping (for internal use only).
func (s *Store) TransitionStatusInternal(ctx context.Context, id int64, to Status) error {
	c, err := s.GetCampaign(ctx, id)
	if err != nil {
		return fmt.Errorf("campaign not found: %w", err)
	}
	if err := ValidateTransition(c.Status, to); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx,
		`UPDATE campaigns SET status = $2, updated_at = NOW() WHERE id = $1`,
		id, to,
	)
	return err
}

// ListActiveCampaigns returns all active campaigns (for bidder loading).
func (s *Store) ListActiveCampaigns(ctx context.Context) ([]*Campaign, error) {
	rows, err := s.db.Query(ctx,
		`SELECT c.id, c.advertiser_id, c.name, c.status, c.billing_model, c.budget_total_cents, c.budget_daily_cents,
		        c.spent_cents, c.bid_cpm_cents, c.bid_cpc_cents, c.ocpm_target_cpa_cents,
		        c.start_date, c.end_date, c.targeting, c.created_at, c.updated_at
		 FROM campaigns c
		 WHERE c.status = 'active'
		 ORDER BY c.bid_cpm_cents DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var campaigns []*Campaign
	for rows.Next() {
		c := &Campaign{}
		if err := rows.Scan(&c.ID, &c.AdvertiserID, &c.Name, &c.Status, &c.BillingModel,
			&c.BudgetTotalCents, &c.BudgetDailyCents, &c.SpentCents,
			&c.BidCPMCents, &c.BidCPCCents, &c.OCPMTargetCPACents,
			&c.StartDate, &c.EndDate, &c.Targeting, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, nil
}

// GetCreativesByCampaign returns all approved creatives for a campaign.
func (s *Store) GetCreativesByCampaign(ctx context.Context, campaignID int64) ([]*Creative, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, campaign_id, name, ad_type, format, size, ad_markup, destination_url, status,
		        COALESCE(native_title,''), COALESCE(native_desc,''), COALESCE(native_icon_url,''),
		        COALESCE(native_image_url,''), COALESCE(native_cta,''), created_at
		 FROM creatives WHERE campaign_id = $1 AND status = 'approved'`,
		campaignID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creatives []*Creative
	for rows.Next() {
		cr := &Creative{}
		if err := rows.Scan(&cr.ID, &cr.CampaignID, &cr.Name, &cr.AdType, &cr.Format, &cr.Size,
			&cr.AdMarkup, &cr.DestinationURL, &cr.Status,
			&cr.NativeTitle, &cr.NativeDesc, &cr.NativeIconURL,
			&cr.NativeImageURL, &cr.NativeCTA, &cr.CreatedAt); err != nil {
			return nil, err
		}
		creatives = append(creatives, cr)
	}
	return creatives, nil
}

// CreateCreative creates a new creative.
func (s *Store) CreateCreative(ctx context.Context, cr *Creative) (int64, error) {
	if cr.AdType == "" {
		cr.AdType = AdTypeBanner
	}
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO creatives (campaign_id, name, ad_type, format, size, ad_markup, destination_url, status,
		  native_title, native_desc, native_icon_url, native_image_url, native_cta)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'approved', $8, $9, $10, $11, $12) RETURNING id`,
		cr.CampaignID, cr.Name, cr.AdType, cr.Format, cr.Size, cr.AdMarkup, cr.DestinationURL,
		cr.NativeTitle, cr.NativeDesc, cr.NativeIconURL, cr.NativeImageURL, cr.NativeCTA,
	).Scan(&id)
	return id, err
}

package bidder

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// CampaignLoader loads active campaigns from PostgreSQL and keeps them
// in sync via Redis pub/sub + periodic full pull (30s).
//
// Sync pattern (from eng review):
//   1. Startup: full load from DB
//   2. Listen Redis pub/sub channel "campaign:updates" for incremental changes
//   3. Every 30s: full reconciliation from DB (safety net for missed pub/sub)
type CampaignLoader struct {
	db              *pgxpool.Pool
	rdb             *redis.Client
	store           *campaign.Store
	budgetSvc       *budget.Service // optional; if set, total budget is initialized on load
	mu              sync.RWMutex
	campaigns       map[int64]*LoadedCampaign
	stopCh          chan struct{}
	stopOnce        sync.Once
	refreshInterval time.Duration
	sub             *redis.PubSub
}

// LoaderOption configures a CampaignLoader at construction time.
type LoaderOption func(*CampaignLoader)

// WithRefreshInterval overrides the full-reload period. Defaults to 30s.
// Used by tests to drive the fallback path faster.
func WithRefreshInterval(d time.Duration) LoaderOption {
	return func(cl *CampaignLoader) { cl.refreshInterval = d }
}

// WithBudgetService enables total budget initialization on campaign load.
// When set, the loader calls budgetSvc.InitTotalBudget for every campaign
// after fullLoad and on incremental pub/sub "activated"/"updated" events.
func WithBudgetService(svc *budget.Service) LoaderOption {
	return func(cl *CampaignLoader) { cl.budgetSvc = svc }
}

// LoadedCampaign is a campaign ready for bidding with parsed targeting.
type LoadedCampaign struct {
	ID                 int64
	AdvertiserID       int64
	Name               string
	BillingModel       string
	BidCPMCents        int
	BidCPCCents        int
	OCPMTargetCPACents int
	BudgetTotalCents   int64
	BudgetDailyCents   int64
	StartDate          *time.Time
	EndDate            *time.Time
	Targeting          campaign.Targeting
	Creatives          []*campaign.Creative
}

// EffectiveBidCPMCents returns the CPM-equivalent bid for auction ranking.
func (lc *LoadedCampaign) EffectiveBidCPMCents(predictedCTR, predictedCVR float64) int {
	switch lc.BillingModel {
	case campaign.BillingCPC:
		if predictedCTR <= 0 {
			predictedCTR = 0.01
		}
		return int(float64(lc.BidCPCCents) * predictedCTR * 1000)
	case campaign.BillingOCPM:
		if predictedCTR <= 0 {
			predictedCTR = 0.01
		}
		if predictedCVR <= 0 {
			predictedCVR = 0.05
		}
		return int(float64(lc.OCPMTargetCPACents) * predictedCTR * predictedCVR * 1000)
	default:
		return lc.BidCPMCents
	}
}

func NewCampaignLoader(db *pgxpool.Pool, rdb *redis.Client, opts ...LoaderOption) *CampaignLoader {
	cl := &CampaignLoader{
		db:              db,
		rdb:             rdb,
		store:           campaign.NewStore(db),
		campaigns:       make(map[int64]*LoadedCampaign),
		stopCh:          make(chan struct{}),
		refreshInterval: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(cl)
	}
	return cl
}

// Start loads campaigns and begins background sync.
//
// Start establishes the Redis pub/sub subscription SYNCHRONOUSLY (waiting for
// the server's SUBSCRIBE acknowledgment via sub.Receive) before returning.
// This guarantees that any PUBLISH sent by a caller after Start returns is
// routed to this subscriber — without the synchronous wait, go-redis's lazy
// Subscribe creates a startup race window where early messages are dropped.
func (cl *CampaignLoader) Start(ctx context.Context) error {
	// Initial full load
	if err := cl.fullLoad(ctx); err != nil {
		return err
	}
	log.Printf("[LOADER] Initial load: %d active campaigns", len(cl.campaigns))

	// Subscribe synchronously so the subscription is live before Start returns.
	sub := cl.rdb.Subscribe(ctx, "campaign:updates")
	// Receive blocks until we get the first message, which for a fresh
	// subscription is always the *redis.Subscription confirmation from the
	// server. Once this returns, subsequent PUBLISHes are guaranteed to be
	// delivered to this subscriber.
	msg, err := sub.Receive(ctx)
	if err != nil {
		_ = sub.Close()
		return fmt.Errorf("campaign:updates subscribe: %w", err)
	}
	if _, ok := msg.(*redis.Subscription); !ok {
		_ = sub.Close()
		return fmt.Errorf("campaign:updates: expected subscription ack, got %T", msg)
	}
	cl.sub = sub

	// Background: periodic full pull every 30s
	go cl.periodicRefresh(ctx)

	// Background: listen Redis pub/sub for incremental updates
	go cl.listenPubSub(ctx, sub)

	return nil
}

// Stop stops background goroutines. Safe to call multiple times.
func (cl *CampaignLoader) Stop() {
	cl.stopOnce.Do(func() {
		close(cl.stopCh)
		if cl.sub != nil {
			_ = cl.sub.Close()
		}
	})
}

// GetActiveCampaigns returns a snapshot of all active campaigns.
func (cl *CampaignLoader) GetActiveCampaigns() []*LoadedCampaign {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	result := make([]*LoadedCampaign, 0, len(cl.campaigns))
	for _, c := range cl.campaigns {
		result = append(result, c)
	}
	return result
}

// GetCampaign returns a single campaign by ID, or nil.
func (cl *CampaignLoader) GetCampaign(id int64) *LoadedCampaign {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.campaigns[id]
}

func (cl *CampaignLoader) fullLoad(ctx context.Context) error {
	dbCampaigns, err := cl.store.ListActiveCampaigns(ctx)
	if err != nil {
		return err
	}

	// Batch-load all creatives in a single query (fixes N+1)
	ids := make([]int64, len(dbCampaigns))
	for i, c := range dbCampaigns {
		ids[i] = c.ID
	}
	creativesMap, err := cl.store.GetCreativesByCampaigns(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch load creatives: %w", err)
	}

	newMap := make(map[int64]*LoadedCampaign, len(dbCampaigns))
	for _, c := range dbCampaigns {
		var targeting campaign.Targeting
		if len(c.Targeting) > 0 {
			if err := json.Unmarshal(c.Targeting, &targeting); err != nil {
				log.Printf("[LOADER] skip campaign %d: %v", c.ID, err)
				continue
			}
		}
		newMap[c.ID] = &LoadedCampaign{
			ID:                 c.ID,
			AdvertiserID:       c.AdvertiserID,
			Name:               c.Name,
			BillingModel:       c.BillingModel,
			BidCPMCents:        c.BidCPMCents,
			BidCPCCents:        c.BidCPCCents,
			OCPMTargetCPACents: c.OCPMTargetCPACents,
			BudgetTotalCents:   c.BudgetTotalCents,
			BudgetDailyCents:   c.BudgetDailyCents,
			StartDate:          c.StartDate,
			EndDate:            c.EndDate,
			Targeting:          targeting,
			Creatives:          creativesMap[c.ID],
		}
	}

	cl.mu.Lock()
	cl.campaigns = newMap
	cl.mu.Unlock()

	// Initialize total budget counters in Redis for all loaded campaigns.
	// Uses SetNX so reloads don't reset partially spent counters.
	if cl.budgetSvc != nil {
		for _, lc := range newMap {
			if lc.BudgetTotalCents > 0 {
				if err := cl.budgetSvc.InitTotalBudget(ctx, lc.ID, lc.BudgetTotalCents); err != nil {
					log.Printf("[LOADER] init total budget for campaign %d: %v", lc.ID, err)
				}
			}
		}
	}

	return nil
}

func (cl *CampaignLoader) toCampaignWithCreatives(ctx context.Context, c *campaign.Campaign) (*LoadedCampaign, error) {
	var targeting campaign.Targeting
	if len(c.Targeting) > 0 {
		if err := json.Unmarshal(c.Targeting, &targeting); err != nil {
			return nil, err
		}
	}

	creatives, err := cl.store.GetCreativesByCampaign(ctx, c.ID)
	if err != nil {
		return nil, err
	}

	return &LoadedCampaign{
		ID:                 c.ID,
		AdvertiserID:       c.AdvertiserID,
		Name:               c.Name,
		BillingModel:       c.BillingModel,
		BidCPMCents:        c.BidCPMCents,
		BidCPCCents:        c.BidCPCCents,
		OCPMTargetCPACents: c.OCPMTargetCPACents,
		BudgetTotalCents:   c.BudgetTotalCents,
		BudgetDailyCents:   c.BudgetDailyCents,
		StartDate:          c.StartDate,
		EndDate:            c.EndDate,
		Targeting:          targeting,
		Creatives:          creatives,
	}, nil
}

func (cl *CampaignLoader) periodicRefresh(ctx context.Context) {
	ticker := time.NewTicker(cl.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cl.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := cl.fullLoad(ctx); err != nil {
				log.Printf("[LOADER] periodic refresh error: %v", err)
			}
		}
	}
}

func (cl *CampaignLoader) listenPubSub(ctx context.Context, sub *redis.PubSub) {
	// sub is already subscribed and confirmed by Start; Stop closes it.
	ch := sub.Channel()
	for {
		select {
		case <-cl.stopCh:
			return
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			// Message payload: campaign ID that changed
			var update struct {
				CampaignID int64  `json:"campaign_id"`
				Action     string `json:"action"` // "activated", "paused", "updated"
			}
			if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
				log.Printf("[LOADER] invalid pub/sub message: %v", err)
				continue
			}

			log.Printf("[LOADER] pub/sub: campaign %d %s", update.CampaignID, update.Action)

			switch update.Action {
			case "activated", "updated":
				// Reload this campaign from DB
				c, err := cl.store.GetCampaign(ctx, update.CampaignID)
				if err != nil {
					log.Printf("[LOADER] reload campaign %d: %v", update.CampaignID, err)
					continue
				}
				if c.Status == campaign.StatusActive {
					loaded, err := cl.toCampaignWithCreatives(ctx, c)
					if err != nil {
						log.Printf("[LOADER] parse campaign %d: %v", update.CampaignID, err)
						continue
					}
					cl.mu.Lock()
					cl.campaigns[c.ID] = loaded
					cl.mu.Unlock()
					// Initialize total budget on activation/update
					if cl.budgetSvc != nil && loaded.BudgetTotalCents > 0 {
						if err := cl.budgetSvc.InitTotalBudget(ctx, loaded.ID, loaded.BudgetTotalCents); err != nil {
							log.Printf("[LOADER] init total budget for campaign %d: %v", loaded.ID, err)
						}
					}
				}
			case "paused", "completed", "deleted":
				cl.mu.Lock()
				delete(cl.campaigns, update.CampaignID)
				cl.mu.Unlock()
			}
		}
	}
}

// NotifyCampaignUpdate publishes a campaign change to Redis pub/sub.
// Call this from the API server when a campaign is created/started/paused/updated.
func NotifyCampaignUpdate(ctx context.Context, rdb *redis.Client, campaignID int64, action string) {
	payload, _ := json.Marshal(map[string]any{
		"campaign_id": campaignID,
		"action":      action,
	})
	if err := rdb.Publish(ctx, "campaign:updates", payload).Err(); err != nil {
		log.Printf("[NOTIFY] pub/sub error: %v", err)
	}
}

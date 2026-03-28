package bidder

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

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
	db       *pgxpool.Pool
	rdb      *redis.Client
	store    *campaign.Store
	mu       sync.RWMutex
	campaigns map[int64]*LoadedCampaign
	stopCh   chan struct{}
}

// LoadedCampaign is a campaign ready for bidding with parsed targeting.
type LoadedCampaign struct {
	ID               int64
	AdvertiserID     int64
	Name             string
	BidCPMCents      int
	BudgetDailyCents int64
	Targeting        campaign.Targeting
	Creatives        []*campaign.Creative
}

func NewCampaignLoader(db *pgxpool.Pool, rdb *redis.Client) *CampaignLoader {
	return &CampaignLoader{
		db:        db,
		rdb:       rdb,
		store:     campaign.NewStore(db),
		campaigns: make(map[int64]*LoadedCampaign),
		stopCh:    make(chan struct{}),
	}
}

// Start loads campaigns and begins background sync.
func (cl *CampaignLoader) Start(ctx context.Context) error {
	// Initial full load
	if err := cl.fullLoad(ctx); err != nil {
		return err
	}
	log.Printf("[LOADER] Initial load: %d active campaigns", len(cl.campaigns))

	// Background: periodic full pull every 30s
	go cl.periodicRefresh(ctx)

	// Background: listen Redis pub/sub for incremental updates
	go cl.listenPubSub(ctx)

	return nil
}

// Stop stops background goroutines.
func (cl *CampaignLoader) Stop() {
	close(cl.stopCh)
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

	newMap := make(map[int64]*LoadedCampaign, len(dbCampaigns))
	for _, c := range dbCampaigns {
		loaded, err := cl.toCampaignWithCreatives(ctx, c)
		if err != nil {
			log.Printf("[LOADER] skip campaign %d: %v", c.ID, err)
			continue
		}
		newMap[c.ID] = loaded
	}

	cl.mu.Lock()
	cl.campaigns = newMap
	cl.mu.Unlock()

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
		ID:               c.ID,
		AdvertiserID:     c.AdvertiserID,
		Name:             c.Name,
		BidCPMCents:      c.BidCPMCents,
		BudgetDailyCents: c.BudgetDailyCents,
		Targeting:        targeting,
		Creatives:        creatives,
	}, nil
}

func (cl *CampaignLoader) periodicRefresh(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
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

func (cl *CampaignLoader) listenPubSub(ctx context.Context) {
	sub := cl.rdb.Subscribe(ctx, "campaign:updates")
	defer sub.Close()

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

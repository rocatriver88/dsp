package audit

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	ActionCampaignCreate      = "campaign.create"
	ActionCampaignUpdate      = "campaign.update"
	ActionCampaignStart       = "campaign.start"
	ActionCampaignPause       = "campaign.pause"
	ActionCreativeCreate      = "creative.create"
	ActionCreativeUpdate      = "creative.update"
	ActionCreativeDelete      = "creative.delete"
	ActionCreativeApprove     = "creative.approve"
	ActionCreativeReject      = "creative.reject"
	ActionTopUp               = "billing.topup"
	ActionRegistrationApprove = "registration.approve"
	ActionRegistrationReject  = "registration.reject"
)

var auditErrors = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "dsp_audit_errors_total",
	Help: "Total number of failed audit log writes",
})

func init() {
	prometheus.MustRegister(auditErrors)
}

type Entry struct {
	ID           int64          `json:"id"`
	AdvertiserID int64          `json:"advertiser_id"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   int64          `json:"resource_id"`
	Details      map[string]any `json:"details,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type Logger struct {
	db *pgxpool.Pool
}

func NewLogger(db *pgxpool.Pool) *Logger {
	return &Logger{db: db}
}

func (l *Logger) Record(ctx context.Context, e Entry) {
	detailsJSON, _ := json.Marshal(e.Details)
	_, err := l.db.Exec(ctx,
		`INSERT INTO audit_log (advertiser_id, actor, action, resource_type, resource_id, details)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.AdvertiserID, e.Actor, e.Action, e.ResourceType, e.ResourceID, detailsJSON,
	)
	if err != nil {
		log.Printf("[AUDIT] Failed to record %s: %v", e.Action, err)
		auditErrors.Inc()
	}
}

func (l *Logger) Query(ctx context.Context, advertiserID int64, limit, offset int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := l.db.Query(ctx,
		`SELECT id, advertiser_id, actor, action, resource_type, resource_id, details, created_at
		 FROM audit_log WHERE advertiser_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		advertiserID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.AdvertiserID, &e.Actor, &e.Action,
			&e.ResourceType, &e.ResourceID, &detailsJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		if detailsJSON != nil {
			json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (l *Logger) QueryAll(ctx context.Context, limit, offset int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := l.db.Query(ctx,
		`SELECT id, advertiser_id, actor, action, resource_type, resource_id, details, created_at
		 FROM audit_log ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.AdvertiserID, &e.Actor, &e.Action,
			&e.ResourceType, &e.ResourceID, &detailsJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		if detailsJSON != nil {
			json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

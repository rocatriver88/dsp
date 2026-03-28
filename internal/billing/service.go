package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service handles billing operations: top-up, spend tracking,
// reconciliation, and invoice generation.
//
// Phase 1: prepaid only (balance check + Redis deduction)
// Phase 4: adds postpaid (credit limit), invoices, reconciliation
type Service struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// Transaction types
const (
	TxTopup      = "topup"
	TxSpend      = "spend"
	TxAdjustment = "adjustment"
	TxRefund     = "refund"
)

type Transaction struct {
	ID           int64     `json:"id"`
	AdvertiserID int64     `json:"advertiser_id"`
	Type         string    `json:"type"`
	AmountCents  int64     `json:"amount_cents"`
	BalanceAfter int64     `json:"balance_after"`
	Description  string    `json:"description"`
	ReferenceID  string    `json:"reference_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Invoice struct {
	ID           int64      `json:"id"`
	AdvertiserID int64      `json:"advertiser_id"`
	PeriodStart  time.Time  `json:"period_start"`
	PeriodEnd    time.Time  `json:"period_end"`
	TotalCents   int64      `json:"total_cents"`
	Status       string     `json:"status"`
	IssuedAt     *time.Time `json:"issued_at,omitempty"`
	PaidAt       *time.Time `json:"paid_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Reconciliation struct {
	ID              int64     `json:"id"`
	CampaignID      int64     `json:"campaign_id"`
	Date            time.Time `json:"date"`
	RedisSpent      int64     `json:"redis_spent"`
	ClickhouseSpent int64     `json:"clickhouse_spent"`
	Adjustment      int64     `json:"adjustment"`
	Status          string    `json:"status"`
}

// TopUp adds funds to an advertiser's balance.
func (s *Service) TopUp(ctx context.Context, advertiserID int64, amountCents int64, description string) (*Transaction, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Update balance
	var newBalance int64
	err = tx.QueryRow(ctx,
		`UPDATE advertisers SET balance_cents = balance_cents + $1, updated_at = NOW()
		 WHERE id = $2 RETURNING balance_cents`,
		amountCents, advertiserID,
	).Scan(&newBalance)
	if err != nil {
		return nil, fmt.Errorf("update balance: %w", err)
	}

	// Record transaction
	var txn Transaction
	err = tx.QueryRow(ctx,
		`INSERT INTO transactions (advertiser_id, type, amount_cents, balance_after, description)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		advertiserID, TxTopup, amountCents, newBalance, description,
	).Scan(&txn.ID, &txn.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("record transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	txn.AdvertiserID = advertiserID
	txn.Type = TxTopup
	txn.AmountCents = amountCents
	txn.BalanceAfter = newBalance
	txn.Description = description
	return &txn, nil
}

// RecordSpend records a spend transaction (called during daily reconciliation).
func (s *Service) RecordSpend(ctx context.Context, advertiserID int64, amountCents int64, campaignID int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var newBalance int64
	err = tx.QueryRow(ctx,
		`UPDATE advertisers SET balance_cents = balance_cents - $1, updated_at = NOW()
		 WHERE id = $2 RETURNING balance_cents`,
		amountCents, advertiserID,
	).Scan(&newBalance)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO transactions (advertiser_id, type, amount_cents, balance_after, description, reference_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		advertiserID, TxSpend, -amountCents, newBalance,
		fmt.Sprintf("Campaign %d daily spend", campaignID),
		fmt.Sprintf("campaign:%d", campaignID),
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetTransactions returns transaction history for an advertiser.
func (s *Service) GetTransactions(ctx context.Context, advertiserID int64, limit, offset int) ([]Transaction, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, advertiser_id, type, amount_cents, balance_after, description, reference_id, created_at
		 FROM transactions WHERE advertiser_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		advertiserID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []Transaction
	for rows.Next() {
		var t Transaction
		var ref *string
		if err := rows.Scan(&t.ID, &t.AdvertiserID, &t.Type, &t.AmountCents,
			&t.BalanceAfter, &t.Description, &ref, &t.CreatedAt); err != nil {
			return nil, err
		}
		if ref != nil {
			t.ReferenceID = *ref
		}
		txns = append(txns, t)
	}
	return txns, nil
}

// GenerateInvoice creates a monthly invoice for a postpaid advertiser.
func (s *Service) GenerateInvoice(ctx context.Context, advertiserID int64, periodStart, periodEnd time.Time) (*Invoice, error) {
	// Sum all spend transactions in the period
	var totalCents int64
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(ABS(amount_cents)), 0)
		 FROM transactions
		 WHERE advertiser_id = $1 AND type = 'spend'
		 AND created_at >= $2 AND created_at < $3`,
		advertiserID, periodStart, periodEnd,
	).Scan(&totalCents)
	if err != nil {
		return nil, err
	}

	var inv Invoice
	err = s.db.QueryRow(ctx,
		`INSERT INTO invoices (advertiser_id, period_start, period_end, total_cents, status, issued_at)
		 VALUES ($1, $2, $3, $4, 'issued', NOW())
		 RETURNING id, created_at, issued_at`,
		advertiserID, periodStart, periodEnd, totalCents,
	).Scan(&inv.ID, &inv.CreatedAt, &inv.IssuedAt)
	if err != nil {
		return nil, err
	}

	inv.AdvertiserID = advertiserID
	inv.PeriodStart = periodStart
	inv.PeriodEnd = periodEnd
	inv.TotalCents = totalCents
	inv.Status = "issued"
	return &inv, nil
}

// GetInvoices returns invoices for an advertiser.
func (s *Service) GetInvoices(ctx context.Context, advertiserID int64) ([]Invoice, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, advertiser_id, period_start, period_end, total_cents, status, issued_at, paid_at, created_at
		 FROM invoices WHERE advertiser_id = $1 ORDER BY period_start DESC`,
		advertiserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invoices []Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.AdvertiserID, &inv.PeriodStart, &inv.PeriodEnd,
			&inv.TotalCents, &inv.Status, &inv.IssuedAt, &inv.PaidAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		invoices = append(invoices, inv)
	}
	return invoices, nil
}

// SaveReconciliation saves a daily reconciliation result.
func (s *Service) SaveReconciliation(ctx context.Context, r *Reconciliation) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO daily_reconciliation (campaign_id, date, redis_spent, clickhouse_spent, adjustment, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (campaign_id, date) DO UPDATE SET
		   redis_spent = $3, clickhouse_spent = $4, adjustment = $5, status = $6`,
		r.CampaignID, r.Date, r.RedisSpent, r.ClickhouseSpent, r.Adjustment, r.Status,
	)
	return err
}

// GetBalance returns advertiser balance and billing type.
func (s *Service) GetBalance(ctx context.Context, advertiserID int64) (balanceCents int64, billingType string, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT balance_cents, billing_type FROM advertisers WHERE id = $1`,
		advertiserID,
	).Scan(&balanceCents, &billingType)
	return
}

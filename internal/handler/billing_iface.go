package handler

import (
	"context"

	"github.com/heartgryphon/dsp/internal/billing"
)

// BillingService is the handler-facing view of billing.Service. Must cover
// every method currently called through d.BillingSvc — `GetBalance` by
// HandleBalance + HandleStartCampaign, `TopUp` by HandleTopUp,
// `GetTransactions` by HandleTransactions. Narrowing below this set
// breaks compilation (Codex Finding #1).
//
// billing.Service satisfies this interface automatically — no changes
// needed to the concrete type.
type BillingService interface {
	GetBalance(ctx context.Context, advertiserID int64) (balanceCents int64, billingType string, err error)
	TopUp(ctx context.Context, advertiserID int64, amountCents int64, description string) (*billing.Transaction, error)
	GetTransactions(ctx context.Context, advertiserID int64, limit, offset int) ([]billing.Transaction, error)
}

// Compile-time assertion that *billing.Service satisfies BillingService.
// If this line fails to compile after a billing.Service refactor, a
// method signature has drifted; fix billing.Service or update this interface.
var _ BillingService = (*billing.Service)(nil)

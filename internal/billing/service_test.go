package billing

import "testing"

func TestTransactionTypes(t *testing.T) {
	// Verify constants match expected values
	if TxTopup != "topup" {
		t.Errorf("expected topup, got %s", TxTopup)
	}
	if TxSpend != "spend" {
		t.Errorf("expected spend, got %s", TxSpend)
	}
	if TxAdjustment != "adjustment" {
		t.Errorf("expected adjustment, got %s", TxAdjustment)
	}
	if TxRefund != "refund" {
		t.Errorf("expected refund, got %s", TxRefund)
	}
}

func TestNew(t *testing.T) {
	// Service should initialize without panic with nil db (for testing)
	svc := New(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestTransaction_Fields(t *testing.T) {
	txn := Transaction{
		ID:           1,
		AdvertiserID: 42,
		Type:         TxTopup,
		AmountCents:  100000,
		BalanceAfter: 100000,
		Description:  "Initial deposit",
	}

	if txn.AmountCents != 100000 {
		t.Errorf("expected 100000, got %d", txn.AmountCents)
	}
	if txn.Type != "topup" {
		t.Errorf("expected topup, got %s", txn.Type)
	}
}

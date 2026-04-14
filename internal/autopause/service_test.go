package autopause

import (
	"context"
	"testing"
	"time"
)

// TestStart_NilReportStoreReturnsImmediately verifies the early-exit guard
// at the top of Start. This is the only autopause-side lifecycle assertion
// that can be tested without a real ClickHouse; cross-cancellation of an
// active polling loop is deferred to the Batch 6 integration suite.
func TestStart_NilReportStoreReturnsImmediately(t *testing.T) {
	svc := New(nil, nil, nil) // reportStore deliberately nil

	done := make(chan struct{})
	go func() {
		svc.Start(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// expected: Start returns immediately because reportStore is nil
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Start did not return within 500ms despite nil reportStore")
	}
}

package reconciliation

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestStartHourlySchedule_ExitsOnContextCancel verifies the Batch 3
// lifecycle contract: StartHourlySchedule spawns a goroutine whose only
// exit path is <-ctx.Done(). Cancelling the context must let it leave
// before the process shutdown deadline.
//
// The ticker fires once per hour so during this test the goroutine is
// parked in the select the entire time — we're exercising the cancel
// branch, not the reconciliation logic. Passing nil dependencies is
// safe because RunHourly is never reached.
func TestStartHourlySchedule_ExitsOnContextCancel(t *testing.T) {
	svc := New(nil, nil, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	before := runtime.NumGoroutine()
	svc.StartHourlySchedule(ctx, 1.0)

	// Give the spawned goroutine time to reach its select.
	time.Sleep(20 * time.Millisecond)
	cancel()

	// Poll briefly for the goroutine count to return to the pre-start
	// baseline. The runtime may take a few scheduling ticks to reap a
	// goroutine after its function returns.
	deadline := time.After(500 * time.Millisecond)
	for {
		if runtime.NumGoroutine() <= before {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("reconciliation goroutine did not exit within 500ms (before=%d now=%d)", before, runtime.NumGoroutine())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

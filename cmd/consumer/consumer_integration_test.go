//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/segmentio/kafka-go"
)

// ensureTopic creates `topic` on the broker if it does not already exist.
// Safe to call repeatedly; kafka-go returns a TopicAlreadyExists error
// which we ignore. Needed because the dev compose stack ships without
// dsp.dead-letter pre-created, and the producer's async writer cannot
// surface the "Unknown Topic Or Partition" error back to callers.
func ensureTopic(t *testing.T, brokers []string, topic string) {
	t.Helper()
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		t.Fatalf("ensureTopic: dial: %v", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		t.Fatalf("ensureTopic: controller: %v", err)
	}
	ctrlConn, err := kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		t.Fatalf("ensureTopic: dial controller: %v", err)
	}
	defer ctrlConn.Close()

	err = ctrlConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil {
		t.Logf("ensureTopic(%s): %v (ignoring — may already exist)", topic, err)
	}
}

// consumerFixture bundles a running in-process consumer wired to the harness.
type consumerFixture struct {
	*qaharness.TestHarness
	store  BidLogStore
	dlq    *events.Producer
	cancel context.CancelFunc
}

// startConsumer spins up RunConsumer in the background with a unique group id.
// The test MUST set f.store before calling startConsumer.
func (f *consumerFixture) startConsumer(t *testing.T, topics []string) {
	t.Helper()
	ctx, cancel := context.WithCancel(f.Ctx)
	f.cancel = cancel
	t.Cleanup(cancel)

	groupID := fmt.Sprintf("qa-consumer-%d", time.Now().UnixNano())
	deps := RunnerDeps{
		Brokers:     f.Env.KafkaBrokers,
		Topics:      topics,
		GroupID:     groupID,
		Store:       f.store,
		DLQProducer: f.dlq,
	}
	go RunConsumer(ctx, deps)
	// Give the reader goroutines a moment to subscribe.
	time.Sleep(500 * time.Millisecond)
}

func newConsumerFixture(t *testing.T) *consumerFixture {
	h := qaharness.New(t)

	store, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatalf("reporting store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	dlq := events.NewProducer(h.Env.KafkaBrokers, t.TempDir())
	t.Cleanup(dlq.Close)

	return &consumerFixture{
		TestHarness: h,
		store:       store,
		dlq:         dlq,
	}
}

// Scenario 31 — all 5 event types land in bid_log, and event_time preserves
// the original (historical) timestamp. Probes CB3.
func TestConsumer_AllEventTypesLand(t *testing.T) {
	f := newConsumerFixture(t)
	f.startConsumer(t, []string{"dsp.bids", "dsp.impressions"})

	prod := events.NewProducer(f.Env.KafkaBrokers, t.TempDir())
	defer prod.Close()

	// Use a per-run unique campaign_id so this test never reads stale rows
	// from a previous run (bid_log rows are not pruned between runs).
	campID := int64(900060000) + time.Now().UnixNano()%1000000
	historicalTS := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	mkEvt := func(reqSuffix string) events.Event {
		return events.Event{
			CampaignID:   campID,
			AdvertiserID: 900060,
			CreativeID:   1,
			RequestID:    fmt.Sprintf("qa-types-%s", reqSuffix),
			BidPrice:     0.05,
			Timestamp:    historicalTS,
		}
	}

	prod.SendBid(f.Ctx, mkEvt("bid"))
	prod.SendWin(f.Ctx, mkEvt("win"))
	prod.SendImpression(f.Ctx, mkEvt("imp"))
	prod.SendClick(f.Ctx, mkEvt("click"))
	prod.SendConversion(f.Ctx, mkEvt("conv"))

	// Wait for all 5 event types to land in bid_log for this campaign.
	f.WaitForBidLogRows(campID, "bid", 1, 30*time.Second)
	f.WaitForBidLogRows(campID, "win", 1, 30*time.Second)
	f.WaitForBidLogRows(campID, "impression", 1, 30*time.Second)
	f.WaitForBidLogRows(campID, "click", 1, 30*time.Second)
	f.WaitForBidLogRows(campID, "conversion", 1, 30*time.Second)

	// CB3 probe: event_time in CH should match historicalTS (~2 hours ago).
	// If it's within 5 seconds of "now", the producer overwrote the Timestamp.
	var storedTime time.Time
	err := f.CH.QueryRow(f.Ctx, `
		SELECT event_time FROM bid_log WHERE campaign_id = ? AND event_type = 'bid' LIMIT 1
	`, uint64(campID)).Scan(&storedTime)
	if err != nil {
		t.Fatalf("scan event_time: %v", err)
	}

	diffFromHistorical := storedTime.Sub(historicalTS).Abs()
	diffFromNow := time.Now().UTC().Sub(storedTime).Abs()

	t.Logf("CB3 probe: historicalTS=%v storedTime=%v diff_from_historical=%v diff_from_now=%v",
		historicalTS, storedTime, diffFromHistorical, diffFromNow)

	if diffFromHistorical > 5*time.Second {
		t.Errorf("CB3 CONFIRMED: event_time in bid_log (%v) does not match historical Timestamp (%v); diff=%v. Producer.Send is overwriting caller-supplied Timestamp.",
			storedTime, historicalTS, diffFromHistorical)
	}
}

// Scenario 32 — malformed JSON is skipped; valid event still lands.
func TestConsumer_MalformedJSONSkipped(t *testing.T) {
	f := newConsumerFixture(t)
	f.startConsumer(t, []string{"dsp.bids"})

	// Write a malformed message directly via kafka-go (bypassing events.Producer
	// which would JSON-marshal correctly).
	w := &kafka.Writer{
		Addr:         kafka.TCP(f.Env.KafkaBrokers...),
		Topic:        "dsp.bids",
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
	}
	defer w.Close()

	err := w.WriteMessages(f.Ctx, kafka.Message{
		Key:   []byte("qa-malformed"),
		Value: []byte("{not json"),
	})
	if err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	// Then publish a valid event via the producer.
	prod := events.NewProducer(f.Env.KafkaBrokers, t.TempDir())
	defer prod.Close()

	campID := int64(900061000) + time.Now().UnixNano()%1000000
	validReqID := fmt.Sprintf("qa-valid-after-malformed-%d", time.Now().UnixNano())
	prod.SendBid(f.Ctx, events.Event{
		CampaignID:   campID,
		AdvertiserID: 900061,
		RequestID:    validReqID,
		BidPrice:     0.01,
	})

	// The valid event must land regardless of the malformed one before it.
	f.WaitForBidLogRows(campID, "bid", 1, 30*time.Second)
}

// failingStore is a BidLogStore that always errors — used to exercise the DLQ
// path without needing to stop ClickHouse.
type failingStore struct{}

func (f *failingStore) InsertEvent(ctx context.Context, e reporting.BidEvent) error {
	return fmt.Errorf("qa: forced failure for DLQ test")
}

// Scenario 33 — CH write failure routes events to dsp.dead-letter.
func TestConsumer_CHFailureDLQ(t *testing.T) {
	f := newConsumerFixture(t)
	// Ensure the DLQ topic exists — the dev compose broker does not
	// pre-create it, and the producer's async writer cannot surface the
	// resulting "Unknown Topic Or Partition" error.
	ensureTopic(t, f.Env.KafkaBrokers, "dsp.dead-letter")
	// Swap in the failing store BEFORE starting the consumer.
	f.store = &failingStore{}
	f.startConsumer(t, []string{"dsp.bids"})

	prod := events.NewProducer(f.Env.KafkaBrokers, t.TempDir())
	defer prod.Close()

	dlqReqID := fmt.Sprintf("qa-dlq-%d", time.Now().UnixNano())
	prod.SendBid(f.Ctx, events.Event{
		CampaignID:   900062,
		AdvertiserID: 900062,
		RequestID:    dlqReqID,
		BidPrice:     0.01,
	})

	// Wait for one message to land on dsp.dead-letter.
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     f.Env.KafkaBrokers,
		Topic:       "dsp.dead-letter",
		GroupID:     fmt.Sprintf("qa-dlq-reader-%d", time.Now().UnixNano()),
		MinBytes:    1,
		MaxBytes:    1_000_000,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	deadline := time.Now().Add(30 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(f.Ctx, 2*time.Second)
		msg, err := reader.ReadMessage(ctx)
		cancel()
		if err != nil {
			continue
		}
		var dlq map[string]any
		if err := json.Unmarshal(msg.Value, &dlq); err != nil {
			continue
		}
		dataStr, _ := dlq["data"].(string)
		if dataStr != "" && strings.Contains(dataStr, dlqReqID) {
			if ot, _ := dlq["original_topic"].(string); ot != "dsp.bids" {
				t.Errorf("DLQ original_topic: want dsp.bids, got %v", ot)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("DLQ message for request_id=%s not found within 30s", dlqReqID)
	}
}


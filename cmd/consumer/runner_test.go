package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/reporting"
)

// --- mock stores for unit tests ---

// recordingStore records every single-event insert.
type recordingStore struct {
	mu     sync.Mutex
	events []reporting.BidEvent
}

func (s *recordingStore) InsertEvent(_ context.Context, e reporting.BidEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

func (s *recordingStore) getEvents() []reporting.BidEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]reporting.BidEvent, len(s.events))
	copy(cp, s.events)
	return cp
}

// recordingBatchStore records batch inserts and also supports single inserts.
type recordingBatchStore struct {
	recordingStore
	batchMu  sync.Mutex
	batches  [][]reporting.BidEvent
	batchErr error // if set, InsertBatch returns this error
}

func (s *recordingBatchStore) InsertBatch(_ context.Context, evts []reporting.BidEvent) error {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	if s.batchErr != nil {
		return s.batchErr
	}
	cp := make([]reporting.BidEvent, len(evts))
	copy(cp, evts)
	s.batches = append(s.batches, cp)
	return nil
}

func (s *recordingBatchStore) getBatches() [][]reporting.BidEvent {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	cp := make([][]reporting.BidEvent, len(s.batches))
	copy(cp, s.batches)
	return cp
}

func (s *recordingBatchStore) totalBatchedEvents() int {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	n := 0
	for _, b := range s.batches {
		n += len(b)
	}
	return n
}

// --- tests ---

func TestDecodeEvent(t *testing.T) {
	data := []byte(`{"type":"bid","campaign_id":42,"request_id":"req-1","bid_price":1.23,"ts":"2026-04-16T10:00:00Z"}`)
	evt, err := decodeEvent(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if evt.CampaignID != 42 {
		t.Errorf("campaign_id: want 42, got %d", evt.CampaignID)
	}
	if evt.RequestID != "req-1" {
		t.Errorf("request_id: want req-1, got %s", evt.RequestID)
	}
	if evt.BidPriceCents != 123 {
		t.Errorf("bid_price_cents: want 123, got %d", evt.BidPriceCents)
	}
	if evt.EventType != "bid" {
		t.Errorf("event_type: want bid, got %s", evt.EventType)
	}
}

func TestDecodeEvent_Malformed(t *testing.T) {
	_, err := decodeEvent([]byte("{not json"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestBatchBidLogStoreInterface(t *testing.T) {
	// Verify that recordingBatchStore satisfies BatchBidLogStore.
	var _ BatchBidLogStore = (*recordingBatchStore)(nil)
}

func TestBidLogStoreInterface(t *testing.T) {
	// Verify that recordingStore satisfies BidLogStore but NOT BatchBidLogStore.
	var _ BidLogStore = (*recordingStore)(nil)
	// This should NOT compile if uncommented:
	// var _ BatchBidLogStore = (*recordingStore)(nil)
}

func TestBatchConstants(t *testing.T) {
	if batchSize != 1000 {
		t.Errorf("batchSize: want 1000, got %d", batchSize)
	}
	if batchTimeout != 1*time.Second {
		t.Errorf("batchTimeout: want 1s, got %v", batchTimeout)
	}
}

func TestFailingStoreCompatibility(t *testing.T) {
	// The existing failingStore from integration tests must still satisfy
	// BidLogStore (single-event path). Verify the interface hasn't changed.
	s := &failingStore{}
	err := s.InsertEvent(context.Background(), reporting.BidEvent{})
	if err == nil {
		t.Fatal("failingStore should always error")
	}
}

// failingStore duplicated from integration test file for unit test use.
// (integration test file has build tag, so we need our own copy here.)
type failingStore struct{}

func (f *failingStore) InsertEvent(_ context.Context, _ reporting.BidEvent) error {
	return fmt.Errorf("qa: forced failure for DLQ test")
}

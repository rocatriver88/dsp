//go:build integration

package events_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

// Scenario 28 — normal publish: 100 events on dsp.bids land in Kafka, disk buffer empty.
func TestProducer_NormalPublish(t *testing.T) {
	h := qaharness.New(t)
	bufDir := t.TempDir()
	p := events.NewProducer(h.Env.KafkaBrokers, bufDir)
	defer p.Close()

	prefix := fmt.Sprintf("qa-prod-%d", time.Now().UnixNano())
	for i := 0; i < 100; i++ {
		p.SendBid(h.Ctx, events.Event{
			CampaignID: 900050,
			RequestID:  fmt.Sprintf("%s-%d", prefix, i),
			BidPrice:   0.05,
		})
	}
	// Let Async writer flush
	time.Sleep(2 * time.Second)

	got := h.CountMessages("dsp.bids", prefix, 30*time.Second)
	if got != 100 {
		t.Errorf("expected 100 messages on dsp.bids, got %d", got)
	}

	// Disk buffer directory should be empty or every .jsonl file should be size 0.
	entries, _ := os.ReadDir(bufDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".jsonl" {
			info, _ := e.Info()
			if info.Size() > 0 {
				t.Errorf("disk buffer should be empty but %s has %d bytes", e.Name(), info.Size())
			}
		}
	}
}

// Scenario 29 — CB4 probe: Kafka unreachable → disk buffer MUST fill.
// If the buffer stays empty, CB4 is confirmed (kafka.Writer.Async=true silently
// drops messages when the broker is unreachable).
func TestProducer_AsyncFailureBuffers(t *testing.T) {
	bufDir := t.TempDir()
	// Point to a port guaranteed unreachable (nothing listens on 127.0.0.1:1).
	p := events.NewProducer([]string{"127.0.0.1:1"}, bufDir)
	defer p.Close()

	for i := 0; i < 10; i++ {
		p.SendBid(context.Background(), events.Event{
			CampaignID: 900051,
			RequestID:  fmt.Sprintf("qa-buf-%d", i),
			BidPrice:   0.01,
		})
	}
	// Wait long enough for the Async flusher to attempt delivery and (hopefully)
	// fall back to the disk buffer. The flusher uses BatchTimeout=10ms + TCP dial
	// attempts; 10 seconds is ample.
	time.Sleep(10 * time.Second)

	path := filepath.Join(bufDir, "dsp.bids.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("CB4 CONFIRMED: disk buffer file missing at %s: %v — Async producer silently dropped events", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("CB4 CONFIRMED: disk buffer file %s is empty — Async producer silently dropped events", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read buffer: %v", err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 10 {
		t.Errorf("expected 10 buffered events, got %d lines", lines)
	}
}

// Scenario 30 — ReplayBuffer recovers buffered events once Kafka is reachable.
func TestProducer_ReplayBuffer(t *testing.T) {
	h := qaharness.New(t)
	bufDir := t.TempDir()
	path := filepath.Join(bufDir, "dsp.bids.jsonl")

	prefix := fmt.Sprintf("qa-replay-%d", time.Now().UnixNano())
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create buffer: %v", err)
	}
	for i := 0; i < 5; i++ {
		line := fmt.Sprintf(`{"type":"bid","campaign_id":900052,"request_id":"%s-%d","bid_price":0.05}`+"\n", prefix, i)
		if _, err := f.WriteString(line); err != nil {
			f.Close()
			t.Fatalf("write buffer line: %v", err)
		}
	}
	f.Close()

	p := events.NewProducer(h.Env.KafkaBrokers, bufDir)
	defer p.Close()
	if err := p.ReplayBuffer(h.Ctx); err != nil {
		t.Fatalf("replay: %v", err)
	}

	got := h.CountMessages("dsp.bids", prefix, 30*time.Second)
	if got != 5 {
		t.Errorf("replay: expected 5 messages in dsp.bids, got %d", got)
	}

	// Original buffer file should be renamed
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original buffer file should be renamed after replay")
	}
	if _, err := os.Stat(path + ".replayed"); err != nil {
		t.Errorf("expected .replayed marker, got %v", err)
	}
}

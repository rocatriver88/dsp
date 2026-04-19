package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBufferToDisk_WritesJSONL + TestBufferToDisk_AppendsMultiple (below)
// are the unit-level half of the P2-9 Kafka replay buffer sentinel
// (docs/testing-strategy-bidder.md §3 P2). They guard the on-disk JSONL
// format that TestProducer_AsyncFailureBuffers writes and
// TestProducer_ReplayBuffer consumes during integration tests — a format
// change here would break the full lifecycle.
//
// REGRESSION SENTINEL: P2-9 on-disk buffer format.
func TestBufferToDisk_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	p := &Producer{bufferDir: dir}

	data := []byte(`{"campaign_id":1,"type":"bid"}`)
	p.bufferToDisk("dsp.bids", data)

	path := filepath.Join(dir, "dsp.bids.jsonl")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read buffer: %v", err)
	}
	if string(content) != string(data)+"\n" {
		t.Errorf("expected %q, got %q", string(data)+"\n", string(content))
	}
}

func TestBufferToDisk_AppendsMultiple(t *testing.T) {
	dir := t.TempDir()
	p := &Producer{bufferDir: dir}

	p.bufferToDisk("dsp.bids", []byte(`{"id":1}`))
	p.bufferToDisk("dsp.bids", []byte(`{"id":2}`))

	path := filepath.Join(dir, "dsp.bids.jsonl")
	content, _ := os.ReadFile(path)
	lines := splitLines(content)
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"line1\nline2\n", 2},
		{"single", 1},
		{"", 0},
		{"a\nb\nc", 3},
	}
	for _, tc := range tests {
		lines := splitLines([]byte(tc.input))
		if len(lines) != tc.expected {
			t.Errorf("splitLines(%q): expected %d lines, got %d", tc.input, tc.expected, len(lines))
		}
	}
}

func TestDLQAttemptCountIncrement(t *testing.T) {
	// Simulate a DLQ event payload with attempt=2
	original := map[string]any{
		"campaign_id": 1,
		"attempt":     float64(2), // JSON numbers are float64
	}
	data, _ := json.Marshal(original)

	// Extract attempt count (same logic as SendToDeadLetter)
	attempt := 1
	var existing map[string]any
	if json.Unmarshal(data, &existing) == nil {
		if a, ok := existing["attempt"].(float64); ok {
			attempt = int(a) + 1
		}
	}

	if attempt != 3 {
		t.Errorf("expected attempt 3, got %d", attempt)
	}
}

func TestDLQAttemptCount_FirstAttempt(t *testing.T) {
	// Non-DLQ event (no attempt field)
	data := []byte(`{"campaign_id":1,"type":"bid"}`)

	attempt := 1
	var existing map[string]any
	if json.Unmarshal(data, &existing) == nil {
		if a, ok := existing["attempt"].(float64); ok {
			attempt = int(a) + 1
		}
	}

	if attempt != 1 {
		t.Errorf("expected attempt 1 for first event, got %d", attempt)
	}
}

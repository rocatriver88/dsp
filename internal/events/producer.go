package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer sends events to Kafka with local disk buffer on failure.
//
// Topics (from eng review):
//   dsp.bids        — bid/win/loss events (partition by campaign_id)
//   dsp.impressions — impression/click events
//   dsp.billing     — win notice billing events (strict order)
//
// Local buffer (from design doc):
//   Format: JSON lines, append-only
//   Path: /var/log/bidder/kafka-buffer/{topic}.jsonl (or configurable)
//   Max: 1GB per node
//   Replay on startup and Kafka recovery
type Producer struct {
	writers   map[string]*kafka.Writer
	bufferDir string
	mu        sync.Mutex
	kafkaOK   bool
}

type Event struct {
	Type        string    `json:"type"`                   // bid, win, loss, impression, click
	CampaignID  int64     `json:"campaign_id"`
	CreativeID  int64     `json:"creative_id,omitempty"`
	AdvertiserID int64    `json:"advertiser_id,omitempty"`
	RequestID   string    `json:"request_id"`
	BidPrice    float64   `json:"bid_price,omitempty"`
	ClearPrice  float64   `json:"clear_price,omitempty"`
	GeoCountry  string    `json:"geo_country,omitempty"`
	DeviceOS    string    `json:"device_os,omitempty"`
	Timestamp   time.Time `json:"ts"`
}

func NewProducer(brokers []string, bufferDir string) *Producer {
	p := &Producer{
		writers:   make(map[string]*kafka.Writer),
		bufferDir: bufferDir,
		kafkaOK:   true,
	}

	topics := []string{"dsp.bids", "dsp.impressions", "dsp.billing", "dsp.dead-letter"}
	for _, topic := range topics {
		p.writers[topic] = &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{}, // partition by key (campaign_id)
			BatchTimeout: 10 * time.Millisecond,
			Async:        true, // non-blocking for bidder hot path
		}
	}

	os.MkdirAll(bufferDir, 0755)
	return p
}

// Send sends an event to the appropriate Kafka topic.
// Falls back to local buffer if Kafka is unavailable.
func (p *Producer) Send(ctx context.Context, topic string, evt Event) {
	evt.Timestamp = time.Now().UTC()
	data, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[EVENTS] marshal error: %v", err)
		return
	}

	key := []byte(fmt.Sprintf("%d", evt.CampaignID))

	writer, ok := p.writers[topic]
	if !ok {
		log.Printf("[EVENTS] unknown topic: %s", topic)
		return
	}

	err = writer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: data,
	})

	if err != nil {
		p.bufferToDisk(topic, data)
	}
}

// SendBid sends a bid event.
func (p *Producer) SendBid(ctx context.Context, evt Event) {
	evt.Type = "bid"
	p.Send(ctx, "dsp.bids", evt)
}

// SendWin sends a win event to both bids and billing topics.
func (p *Producer) SendWin(ctx context.Context, evt Event) {
	evt.Type = "win"
	p.Send(ctx, "dsp.bids", evt)
	p.Send(ctx, "dsp.billing", evt)
}

// SendLoss sends a loss event.
func (p *Producer) SendLoss(ctx context.Context, evt Event) {
	evt.Type = "loss"
	p.Send(ctx, "dsp.bids", evt)
}

// SendImpression sends an impression event.
func (p *Producer) SendImpression(ctx context.Context, evt Event) {
	evt.Type = "impression"
	p.Send(ctx, "dsp.impressions", evt)
}

// SendClick sends a click event.
func (p *Producer) SendClick(ctx context.Context, evt Event) {
	evt.Type = "click"
	p.Send(ctx, "dsp.impressions", evt)
}

func (p *Producer) bufferToDisk(topic string, data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	path := filepath.Join(p.bufferDir, topic+".jsonl")

	// Check size limit (1GB)
	info, err := os.Stat(path)
	if err == nil && info.Size() > 1<<30 {
		log.Printf("[EVENTS] buffer full for %s (%d bytes), dropping event", topic, info.Size())
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[EVENTS] buffer write error: %v", err)
		return
	}
	defer f.Close()

	f.Write(data)
	f.Write([]byte("\n"))
}

// SendToDeadLetter publishes a failed event to the dead-letter topic for retry.
// Attempt count is embedded in the payload so it persists across process restarts.
func (p *Producer) SendToDeadLetter(ctx context.Context, originalTopic string, data []byte, reason string) {
	// Extract existing attempt count from payload (for re-queued DLQ events)
	attempt := 1
	var existing map[string]any
	if json.Unmarshal(data, &existing) == nil {
		if a, ok := existing["attempt"].(float64); ok {
			attempt = int(a) + 1
		}
	}

	dlqEvent := map[string]any{
		"original_topic": originalTopic,
		"data":           string(data),
		"error":          reason,
		"attempt":        attempt,
		"ts":             time.Now().UTC(),
	}
	payload, _ := json.Marshal(dlqEvent)

	writer, ok := p.writers["dsp.dead-letter"]
	if !ok {
		log.Printf("[DLQ] dead-letter writer not available, buffering to disk")
		p.bufferToDisk("dsp.dead-letter", payload)
		return
	}
	if err := writer.WriteMessages(ctx, kafka.Message{Value: payload}); err != nil {
		log.Printf("[DLQ] write error: %v", err)
		p.bufferToDisk("dsp.dead-letter", payload)
	}
}

// ReplayBuffer replays buffered events from disk to Kafka.
// Called at startup to recover events from prior Kafka outages.
func (p *Producer) ReplayBuffer(ctx context.Context) error {
	entries, err := os.ReadDir(p.bufferDir)
	if err != nil {
		return nil // no buffer directory
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		topic := entry.Name()[:len(entry.Name())-6] // strip .jsonl
		path := filepath.Join(p.bufferDir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[REPLAY] read error %s: %v", path, err)
			continue
		}

		writer, ok := p.writers[topic]
		if !ok {
			log.Printf("[REPLAY] unknown topic %s, skipping", topic)
			continue
		}

		lines := 0
		for _, line := range splitLines(data) {
			if len(line) == 0 {
				continue
			}
			if err := writer.WriteMessages(ctx, kafka.Message{Value: line}); err != nil {
				log.Printf("[REPLAY] send error on %s (stopping replay): %v", topic, err)
				return err
			}
			lines++
		}
		os.Rename(path, path+".replayed")
		log.Printf("[REPLAY] replayed %d events from %s", lines, entry.Name())
	}
	return nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		idx := -1
		for i, b := range data {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx == -1 {
			if len(data) > 0 {
				lines = append(lines, data)
			}
			break
		}
		lines = append(lines, data[:idx])
		data = data[idx+1:]
	}
	return lines
}

// Close closes all Kafka writers.
func (p *Producer) Close() {
	for _, w := range p.writers {
		w.Close()
	}
}


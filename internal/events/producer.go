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

	topics := []string{"dsp.bids", "dsp.impressions", "dsp.billing"}
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

// Close closes all Kafka writers.
func (p *Producer) Close() {
	for _, w := range p.writers {
		w.Close()
	}
}


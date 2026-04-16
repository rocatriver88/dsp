package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/segmentio/kafka-go"
)

// BidLogStore is the tiny interface the consumer needs from reporting.Store.
// Integration tests can swap in a failing implementation to exercise the
// DLQ path without touching ClickHouse.
type BidLogStore interface {
	InsertEvent(ctx context.Context, e reporting.BidEvent) error
}

// BatchBidLogStore extends BidLogStore with batch insert support.
// When the store implements this interface the consumer uses batch
// inserts for dramatically better ClickHouse throughput.
type BatchBidLogStore interface {
	BidLogStore
	InsertBatch(ctx context.Context, events []reporting.BidEvent) error
}

const (
	// batchSize is the maximum number of events buffered before a flush.
	batchSize = 1000
	// batchTimeout is the max time between the first buffered event and flush.
	batchTimeout = 1 * time.Second
)

// RunnerDeps bundles everything RunConsumer needs.
type RunnerDeps struct {
	Brokers     []string
	Topics      []string
	GroupID     string
	Store       BidLogStore
	DLQProducer *events.Producer
}

// bufferedEvent pairs a decoded BidEvent with the raw Kafka message data
// and topic, so we can route to DLQ on batch insert failure.
type bufferedEvent struct {
	bid      reporting.BidEvent
	rawValue []byte
	topic    string
}

// RunConsumer spawns one reader per topic and blocks until ctx is cancelled.
// Each topic's reader runs in its own goroutine; the function waits for all
// of them to return (which they do only when ctx is cancelled).
//
// If the Store implements BatchBidLogStore, events are collected in a buffer
// and flushed as a single batch INSERT when either batchSize events
// accumulate or batchTimeout elapses since the first buffered event.
// Otherwise, events are inserted one at a time (legacy path).
func RunConsumer(ctx context.Context, deps RunnerDeps) {
	done := make(chan struct{}, len(deps.Topics))
	for _, topic := range deps.Topics {
		t := topic
		go func() {
			defer func() { done <- struct{}{} }()
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:  deps.Brokers,
				Topic:    t,
				GroupID:  deps.GroupID,
				MinBytes: 1,
				MaxBytes: 10e6,
				MaxWait:  1 * time.Second,
			})
			defer reader.Close()
			log.Printf("[CONSUMER] Listening on topic: %s", t)

			batchStore, canBatch := deps.Store.(BatchBidLogStore)
			if canBatch {
				runBatchLoop(ctx, reader, batchStore, deps.DLQProducer, t)
			} else {
				runSingleLoop(ctx, reader, deps.Store, deps.DLQProducer, t)
			}
		}()
	}

	for range deps.Topics {
		<-done
	}
}

// runSingleLoop is the legacy one-at-a-time insert path.
func runSingleLoop(ctx context.Context, reader *kafka.Reader, store BidLogStore, dlq *events.Producer, topic string) {
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[CONSUMER] %s read error: %v", topic, err)
			time.Sleep(time.Second)
			continue
		}

		bidEvt, err := decodeEvent(msg.Value)
		if err != nil {
			log.Printf("[CONSUMER] %s unmarshal error: %v", topic, err)
			continue
		}

		if err := store.InsertEvent(ctx, bidEvt); err != nil {
			log.Printf("[CONSUMER] %s insert error: %v (sending to DLQ)", topic, err)
			dlq.SendToDeadLetter(ctx, topic, msg.Value, err.Error())
			continue
		}
	}
}

// runBatchLoop collects events into a buffer and flushes as a batch.
// Flush triggers: batchSize events accumulated, or batchTimeout since
// the first event in the current buffer, or context cancellation (drain).
func runBatchLoop(ctx context.Context, reader *kafka.Reader, store BatchBidLogStore, dlq *events.Producer, topic string) {
	var (
		mu      sync.Mutex
		buf     []bufferedEvent
		timer   *time.Timer
		timerCh <-chan time.Time // nil when no timer is active
	)

	flush := func() {
		mu.Lock()
		batch := buf
		buf = nil
		if timer != nil {
			timer.Stop()
			timer = nil
			timerCh = nil
		}
		mu.Unlock()

		if len(batch) == 0 {
			return
		}

		bidEvents := make([]reporting.BidEvent, len(batch))
		for i := range batch {
			bidEvents[i] = batch[i].bid
		}

		if err := store.InsertBatch(ctx, bidEvents); err != nil {
			log.Printf("[CONSUMER] %s batch insert error (%d events): %v (sending to DLQ)", topic, len(batch), err)
			for _, be := range batch {
				dlq.SendToDeadLetter(ctx, be.topic, be.rawValue, err.Error())
			}
		}
	}

	for {
		// Use a select to handle both new messages and timer fires.
		// We need a non-blocking approach: read with a short deadline
		// so we can check the timer.
		select {
		case <-ctx.Done():
			flush() // drain remaining buffer
			return
		case <-timerCh:
			flush()
		default:
			// Try to read a message with a short timeout so we can
			// re-check the timer and context promptly.
			readCtx, readCancel := context.WithTimeout(ctx, 200*time.Millisecond)
			msg, err := reader.ReadMessage(readCtx)
			readCancel()

			if err != nil {
				if ctx.Err() != nil {
					flush()
					return
				}
				// Timeout or transient error — loop back to check timer.
				continue
			}

			bidEvt, err := decodeEvent(msg.Value)
			if err != nil {
				log.Printf("[CONSUMER] %s unmarshal error: %v", topic, err)
				continue
			}

			mu.Lock()
			buf = append(buf, bufferedEvent{
				bid:      bidEvt,
				rawValue: msg.Value,
				topic:    topic,
			})
			needsFlush := len(buf) >= batchSize
			if len(buf) == 1 && timer == nil {
				// First event in a new batch — start the timeout.
				timer = time.NewTimer(batchTimeout)
				timerCh = timer.C
			}
			mu.Unlock()

			if needsFlush {
				flush()
			}
		}
	}
}

// decodeEvent unmarshals a Kafka message value into a reporting.BidEvent.
func decodeEvent(data []byte) (reporting.BidEvent, error) {
	var evt events.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		return reporting.BidEvent{}, err
	}
	return reporting.BidEvent{
		EventTime:       evt.Timestamp,
		CampaignID:      uint64(evt.CampaignID),
		CreativeID:      uint64(evt.CreativeID),
		AdvertiserID:    uint64(evt.AdvertiserID),
		RequestID:       evt.RequestID,
		GeoCountry:      evt.GeoCountry,
		DeviceOS:        evt.DeviceOS,
		DeviceID:        evt.DeviceID,
		BidPriceCents:   uint32(evt.BidPrice*100 + 0.5),
		ClearPriceCents: uint32(evt.ClearPrice*100 + 0.5),
		ChargeCents:     uint32(evt.AdvertiserCharge*100 + 0.5),
		EventType:       evt.Type,
	}, nil
}

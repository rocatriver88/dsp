package main

import (
	"context"
	"encoding/json"
	"log"
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

// RunnerDeps bundles everything RunConsumer needs.
type RunnerDeps struct {
	Brokers     []string
	Topics      []string
	GroupID     string
	Store       BidLogStore
	DLQProducer *events.Producer
}

// RunConsumer spawns one reader per topic and blocks until ctx is cancelled.
// Each topic's reader runs in its own goroutine; the function waits for all
// of them to return (which they do only when ctx is cancelled).
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

			for {
				msg, err := reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("[CONSUMER] %s read error: %v", t, err)
					time.Sleep(time.Second)
					continue
				}

				var evt events.Event
				if err := json.Unmarshal(msg.Value, &evt); err != nil {
					log.Printf("[CONSUMER] %s unmarshal error: %v", t, err)
					continue
				}

				bidEvt := reporting.BidEvent{
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
				}

				if err := deps.Store.InsertEvent(ctx, bidEvt); err != nil {
					log.Printf("[CONSUMER] %s insert error: %v (sending to DLQ)", t, err)
					deps.DLQProducer.SendToDeadLetter(ctx, t, msg.Value, err.Error())
					continue
				}
			}
		}()
	}

	for range deps.Topics {
		<-done
	}
}

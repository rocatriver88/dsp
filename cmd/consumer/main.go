package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/segmentio/kafka-go"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Connect ClickHouse
	store, err := reporting.NewStore(cfg.ClickHouseAddr)
	if err != nil {
		log.Fatalf("connect clickhouse: %v", err)
	}
	defer store.Close()
	log.Println("Connected to ClickHouse")

	// Kafka readers for all 3 topics
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	topics := []string{"dsp.bids", "dsp.impressions", "dsp.billing"}

	for _, topic := range topics {
		t := topic
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:  brokers,
			Topic:    t,
			GroupID:  "dsp-clickhouse-consumer",
			MinBytes: 1,
			MaxBytes: 10e6,
			MaxWait:  1 * time.Second,
		})

		go func() {
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
					BidPriceCents:   uint32(evt.BidPrice * 100000),   // dollars to cents*1000
					ClearPriceCents: uint32(evt.ClearPrice * 100000),
					EventType:       evt.Type,
				}

				if err := store.InsertEvent(ctx, bidEvt); err != nil {
					log.Printf("[CONSUMER] %s insert error: %v", t, err)
					continue
				}
			}
		}()
	}

	log.Println("Kafka → ClickHouse consumer running. Press Ctrl+C to stop.")
	<-ctx.Done()
	log.Println("Shutting down consumer...")
}

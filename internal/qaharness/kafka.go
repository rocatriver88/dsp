package qaharness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/segmentio/kafka-go"
)

// ReadMessagesFrom scans the topic from FirstOffset and returns up to `count`
// messages whose request_id starts with `reqPrefix`, or fails the test on
// timeout. The reqPrefix filter MUST be unique per test — it is the only
// thing isolating this test's messages from historical topic contents and
// from other tests running against the same broker.
//
// Unlike CountMessages, this helper blocks until it has collected `count`
// matching messages. Use this when you need the actual event payloads; use
// CountMessages when you only need a count.
func (h *TestHarness) ReadMessagesFrom(topic, reqPrefix string, count int, timeout time.Duration) []events.Event {
	h.TestT.Helper()
	groupID := fmt.Sprintf("qa-test-%s-%d", topic, time.Now().UnixNano())
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     h.Env.KafkaBrokers,
		Topic:       topic,
		GroupID:     groupID,
		MinBytes:    1,
		MaxBytes:    10_000_000,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(h.Ctx, timeout)
	defer cancel()

	var collected []events.Event
	for len(collected) < count {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			h.TestT.Fatalf("ReadMessagesFrom %s: waited for %d, got %d: %v",
				topic, count, len(collected), err)
		}
		var evt events.Event
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			continue
		}
		if reqPrefix != "" && !strings.HasPrefix(evt.RequestID, reqPrefix) {
			continue
		}
		collected = append(collected, evt)
	}
	return collected
}

// CountMessages returns how many events on `topic` match `reqPrefix` within
// `timeout`. Scans from the start of the topic (so it sees messages written
// before the reader was created) and returns as soon as the broker reports
// no new messages for up to 500ms.
func (h *TestHarness) CountMessages(topic, reqPrefix string, timeout time.Duration) int {
	h.TestT.Helper()
	groupID := fmt.Sprintf("qa-count-%s-%d", topic, time.Now().UnixNano())
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     h.Env.KafkaBrokers,
		Topic:       topic,
		GroupID:     groupID,
		MinBytes:    1,
		MaxBytes:    10_000_000,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(h.Ctx, timeout)
	defer cancel()

	n := 0
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			return n
		}
		var evt events.Event
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			continue
		}
		if reqPrefix == "" || strings.HasPrefix(evt.RequestID, reqPrefix) {
			n++
		}
	}
}

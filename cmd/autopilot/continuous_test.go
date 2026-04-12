package main

import (
	"testing"
	"time"
)

func TestTrafficCurve(t *testing.T) {
	sim := &ContinuousSimulator{
		dayStartHour: 8,
		dayEndHour:   22,
		dayQPS:       100,
		nightQPS:     5,
	}

	dayTime := time.Date(2026, 4, 12, 14, 0, 0, 0, time.Local)
	qps := sim.currentQPS(dayTime)
	if qps != 100 {
		t.Errorf("14:00 should be day QPS=100, got %d", qps)
	}

	nightTime := time.Date(2026, 4, 12, 3, 0, 0, 0, time.Local)
	qps = sim.currentQPS(nightTime)
	if qps != 5 {
		t.Errorf("03:00 should be night QPS=5, got %d", qps)
	}

	boundaryTime := time.Date(2026, 4, 12, 8, 0, 0, 0, time.Local)
	qps = sim.currentQPS(boundaryTime)
	if qps != 100 {
		t.Errorf("08:00 should be day QPS=100, got %d", qps)
	}
}

func TestShouldGenerateReport(t *testing.T) {
	sim := &ContinuousSimulator{reportHour: 9}

	at := time.Date(2026, 4, 12, 9, 0, 30, 0, time.Local)
	lastReport := time.Date(2026, 4, 11, 9, 0, 0, 0, time.Local)

	if !sim.shouldGenerateReport(at, lastReport) {
		t.Error("should generate report at 09:00 if last report was yesterday")
	}

	lastReport = time.Date(2026, 4, 12, 9, 0, 0, 0, time.Local)
	if sim.shouldGenerateReport(at, lastReport) {
		t.Error("should not generate report twice on same day")
	}
}

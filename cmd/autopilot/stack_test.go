package main

import (
	"errors"
	"testing"
)

func TestSortedKeys(t *testing.T) {
	keys := sortedKeys(map[string]error{
		"Exchange-Sim": errors.New("x"),
		"API":          errors.New("x"),
		"Bidder":       errors.New("x"),
	})
	want := []string{"API", "Bidder", "Exchange-Sim"}
	if len(keys) != len(want) {
		t.Fatalf("len: want %d got %d", len(want), len(keys))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d]: want %q got %q", i, want[i], keys[i])
		}
	}
}

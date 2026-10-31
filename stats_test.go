package main

import (
	"testing"
	"time"
)

func TestMultiKill(t *testing.T) {
	var stats Stats

	var messages []string
	for idx := range 7 {
		if msg := stats.AddKill(time.Duration(idx) * 100 * time.Millisecond); msg != "" {
			messages = append(messages, msg)
		}
	}

	if len(messages) != 2 || messages[0] != "MULTI KILL" || messages[1] != "MEGA KILL" {
		t.Fatalf("unexpected messages %v", messages)
	}

	// much later the count starts again
	if msg := stats.AddKill(10 * time.Second); msg != "" || len(stats.recentKills) != 1 {
		t.Fatalf("old kills should be forgotten, got %q and %d recent kills", msg, len(stats.recentKills))
	}

	if stats.Kills != 8 {
		t.Fatalf("expected 8 kills, got %d", stats.Kills)
	}
}

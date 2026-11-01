package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatLeaderboard(t *testing.T) {
	var entries []highscoreEntry
	for idx := range 12 {
		entries = append(entries, highscoreEntry{Player: fmt.Sprint("p", idx), Score: 100 - idx})
	}

	// the player's line is appended after the best entries
	lines := strings.Split(formatLeaderboard(entries, "p10"), "\n")
	if len(lines) != 1+highscoreEntries+2 {
		t.Fatalf("unexpected lines: %q", lines)
	}

	if want := "> 11. p10  90 pts <"; lines[len(lines)-1] != want {
		t.Fatalf("got %q, want %q", lines[len(lines)-1], want)
	}

	// a player among the best is only marked
	lines = strings.Split(formatLeaderboard(entries, "p0"), "\n")
	if len(lines) != 1+highscoreEntries || lines[1] != "> 1. p0  100 pts <" {
		t.Fatalf("unexpected lines: %q", lines)
	}
}

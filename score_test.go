package main

import (
	"testing"
	"time"

	. "github.com/oliverbestmann/byke"
)

func TestScoreKill(t *testing.T) {
	var stats Stats
	vt := VirtualTime{Elapsed: time.Second}

	// first kill: walker points plus first blood
	points, multi := stats.scoreKill(vt, ZombieKilled{Kind: ZombieWalker}, 100)
	if points != 100 || multi != 1 || stats.Score != 200 {
		t.Fatalf("got %d points x%d, score %d", points, multi, stats.Score)
	}

	// second kill in the window counts double
	points, multi = stats.scoreKill(vt, ZombieKilled{Kind: ZombieWalker}, 100)
	if points != 200 || multi != 2 || stats.Score != 400 {
		t.Fatalf("got %d points x%d, score %d", points, multi, stats.Score)
	}

	// explosion kills of one frame unlock fireworks once
	for range 2 * fireworksKills {
		stats.scoreKill(vt, ZombieKilled{Kind: ZombieRunner, Source: SourceExplosion}, 100)
	}

	if !stats.achieved[achFireworks] || stats.achieved[achGravedigger] {
		t.Fatalf("unexpected achievements %v", stats.achieved)
	}
}

func TestUntouchable(t *testing.T) {
	var vt VirtualTime
	stats := Stats{Wave: 1}

	stats.trackWave(vt, 100)
	stats.trackWave(vt, 90)
	stats.Wave = 2
	stats.trackWave(vt, 90)
	if stats.achieved[achUntouchable] {
		t.Fatal("hurt in wave 1, no achievement")
	}

	stats.Wave = 3
	stats.trackWave(vt, 95)
	if !stats.achieved[achUntouchable] {
		t.Fatal("wave 2 without damage")
	}
}

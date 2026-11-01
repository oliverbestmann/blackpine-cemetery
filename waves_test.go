package main

import (
	"math/rand/v2"
	"testing"

	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/zombies/physics"
)

func TestWavesGrow(t *testing.T) {
	for wave := 1; wave < 20; wave++ {
		if waveSize(wave+1) <= waveSize(wave) {
			t.Fatalf("wave %d is not bigger than wave %d", wave+1, wave)
		}

		if waveSpawnInterval(wave+1) > waveSpawnInterval(wave) {
			t.Fatalf("wave %d spawns slower than wave %d", wave+1, wave)
		}
	}
}

func TestFirstWaveOnlyWalkers(t *testing.T) {
	for roll := float32(0); roll < 1; roll += 0.01 {
		if kind := waveKind(1, roll); kind != ZombieWalker {
			t.Fatalf("first wave should only have walkers, got %v at %v", kind, roll)
		}
	}

	// later waves have all kinds
	seen := map[ZombieKind]bool{}
	for roll := float32(0); roll < 1; roll += 0.01 {
		seen[waveKind(8, roll)] = true
	}

	if len(seen) != 4 {
		t.Fatalf("wave 8 should have all kinds, got %v", seen)
	}
}

func TestSpawnPositionAwayFromPlayer(t *testing.T) {
	rng := &GameRng{rand.New(rand.NewPCG(1, 2))}
	player := graveyardCenter

	world := physics.PhysicsWorld{World: b3.CreateWorld(b3.DefaultWorldDef())}
	defer world.World.DestroyWorld()

	for range 1000 {
		pos := spawnPosition(rng, world, player)
		if flat(pos.Sub(player)).Length() <= 25 {
			t.Fatalf("zombie spawned at %v, too close to the player", pos)
		}

		if abs32(pos[0]) > arenaHalfSize || abs32(pos[2]) > arenaHalfSize {
			t.Fatalf("zombie spawned outside the arena at %v", pos)
		}
	}
}

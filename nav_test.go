package main

import (
	"testing"

	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestZombieNavWalksAroundWall(t *testing.T) {
	nav := ZombieNav{
		blocked: make([]bool, navCells*navCells),
		dist:    make([]int32, navCells*navCells),
	}

	// a wall along x from -15 to 15 at z=0, with the player behind it
	for x := -15; x <= 15; x++ {
		cx, cz := navCell(glm.V3f(float32(x), 0, 0))
		nav.blocked[cz*navCells+cx] = true
	}

	player := glm.V3f(0, 0, 10)
	nav.solve(player)

	// in the open the zombie walks straight at the player
	open := glm.V3f(30, 0, 10)
	if dir := nav.direction(open, player); dir.Normalize().Sub(glm.V3f(-1, 0, 0)).Length() > 1e-3 {
		t.Fatalf("expected straight line, got %v", dir)
	}

	// behind the wall it walks around it, step by step, and reaches the player
	pos := glm.V3f(2, 0, -10)
	for range 2000 {
		dir := nav.direction(pos, player)
		if dir.Length() < 0.5 {
			break
		}

		next := pos.Add(dir.Normalize().Scale(0.1))
		if x, z := navCell(next); nav.blocked[z*navCells+x] {
			t.Fatalf("walked into the wall at %v", next)
		}

		pos = next
	}

	if pos.Sub(player).Length() > 0.5 {
		t.Fatalf("zombie got stuck at %v", pos)
	}
}

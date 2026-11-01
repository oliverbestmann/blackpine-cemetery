package main

import (
	"testing"

	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestSliceMesh(t *testing.T) {
	// a tall quad from y=0 to y=4, with vertices only at the top and the bottom
	vertices := []glm.Vec3f{{-1, 0, 0}, {1, 0, 0}, {1, 4, 0}, {-1, 4, 0}}
	indices := []uint32{0, 1, 2, 0, 2, 3}

	slabs := sliceMesh(vertices, indices, 4)
	if len(slabs) != 4 {
		t.Fatalf("expected 4 slabs, got %d", len(slabs))
	}

	for idx, points := range slabs {
		lo, hi := points[0], points[0]
		for _, p := range points {
			lo, hi = lo.Min(p), hi.Max(p)
		}

		// every slab covers the full width and exactly its own height
		want := float32(idx)
		if lo[0] != -1 || hi[0] != 1 || abs32(lo[1]-want) > 1e-5 || abs32(hi[1]-want-1) > 1e-5 {
			t.Fatalf("slab %d spans %v to %v", idx, lo, hi)
		}
	}
}

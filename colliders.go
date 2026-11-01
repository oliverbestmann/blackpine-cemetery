package main

import (
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

// slicedHullCollider cuts the mesh into horizontal slabs of equal height and puts a convex hull
// around each slab. Stacked hulls follow models like trees and gravestones closer than one hull.
func slicedHullCollider(mesh *Mesh, slices int) physics.Collider {
	slabs := sliceMesh(mesh.Vertices(), mesh.Indices(), max(slices, 1))

	var hulls []physics.Collider
	for _, points := range slabs {
		// a hull needs some volume, skip slabs with only a sliver of the mesh
		if len(points) >= 4 {
			hulls = append(hulls, physics.HullCollider(points))
		}
	}

	if len(hulls) == 1 {
		return hulls[0]
	}

	return physics.CompoundCollider(hulls...)
}

// sliceMesh clips the triangles to slices horizontal slabs and returns the points in each slab.
func sliceMesh(vertices []glm.Vec3f, indices []uint32, slices int) [][]glm.Vec3f {
	lo, hi := vertices[0][1], vertices[0][1]
	for _, v := range vertices {
		lo, hi = min(lo, v[1]), max(hi, v[1])
	}

	height := (hi - lo) / float32(slices)
	slabs := make([][]glm.Vec3f, slices)

	for i := 0; i+2 < len(indices); i += 3 {
		triangle := []glm.Vec3f{vertices[indices[i]], vertices[indices[i+1]], vertices[indices[i+2]]}

		for idx := range slabs {
			bottom := lo + float32(idx)*height
			top := bottom + height

			polygon := clipY(triangle, bottom, 1)
			polygon = clipY(polygon, top, -1)
			slabs[idx] = append(slabs[idx], polygon...)
		}
	}

	return slabs
}

// clipY keeps the part of a convex polygon above y (side 1) or below y (side -1).
func clipY(polygon []glm.Vec3f, y float32, side float32) []glm.Vec3f {
	var out []glm.Vec3f

	for idx, a := range polygon {
		b := polygon[(idx+1)%len(polygon)]
		da, db := (a[1]-y)*side, (b[1]-y)*side

		if da >= 0 {
			out = append(out, a)
		}

		// the edge crosses the plane
		if (da >= 0) != (db >= 0) {
			t := da / (da - db)
			out = append(out, a.Add(b.Sub(a).Scale(t)))
		}
	}

	return out
}

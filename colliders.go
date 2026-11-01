package main

import (
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

// slicedHullCollider cuts the mesh into horizontal slabs of equal height and puts a convex hull
// around each slab. Stacked hulls follow models like trees and gravestones closer than one hull.
func slicedHullCollider(mesh meshBuilder, slices int) physics.Collider {
	var hulls []physics.Collider
	for _, slab := range mesh.slice(max(slices, 1)) {
		// a hull needs some volume, skip slabs with only a sliver of the mesh
		if len(slab.vertices) >= 4 {
			hulls = append(hulls, physics.HullCollider(slab.vertices))
		}
	}

	if len(hulls) == 1 {
		return hulls[0]
	}

	return physics.CompoundCollider(hulls...)
}

// meshVertex is one vertex with all its attributes, for clipping.
type meshVertex struct {
	pos, normal glm.Vec3f
	uv          glm.Vec2f
}

func (a meshVertex) lerp(b meshVertex, t float32) meshVertex {
	return meshVertex{
		pos:    a.pos.Add(b.pos.Sub(a.pos).Scale(t)),
		normal: a.normal.Add(b.normal.Sub(a.normal).Scale(t)).Normalize(),
		uv:     a.uv.Add(b.uv.Sub(a.uv).Scale(t)),
	}
}

func (b *meshBuilder) vertex(idx uint32) meshVertex {
	return meshVertex{pos: b.vertices[idx], normal: b.normals[idx], uv: b.uvs[idx]}
}

// addPolygon adds a convex polygon as a triangle fan.
func (b *meshBuilder) addPolygon(polygon []meshVertex) {
	if len(polygon) < 3 {
		return
	}

	base := uint32(len(b.vertices))
	for _, v := range polygon {
		b.vertices = append(b.vertices, v.pos)
		b.normals = append(b.normals, v.normal)
		b.uvs = append(b.uvs, v.uv)
	}

	for idx := uint32(1); idx+1 < uint32(len(polygon)); idx++ {
		b.indices = append(b.indices, base, base+idx, base+idx+1)
	}
}

// slice clips the triangles to horizontal slabs of equal height, one mesh per slab.
// The cut faces stay open.
func (b *meshBuilder) slice(slices int) []meshBuilder {
	lo, hi := b.vertices[0][1], b.vertices[0][1]
	for _, v := range b.vertices {
		lo, hi = min(lo, v[1]), max(hi, v[1])
	}

	height := (hi - lo) / float32(slices)
	slabs := make([]meshBuilder, slices)

	for i := 0; i+2 < len(b.indices); i += 3 {
		triangle := []meshVertex{b.vertex(b.indices[i]), b.vertex(b.indices[i+1]), b.vertex(b.indices[i+2])}

		for idx := range slabs {
			bottom := lo + float32(idx)*height
			top := bottom + height

			polygon := clipY(triangle, bottom, 1)
			polygon = clipY(polygon, top, -1)
			slabs[idx].addPolygon(polygon)
		}
	}

	return slabs
}

// clipY keeps the part of a convex polygon above y (side 1) or below y (side -1).
func clipY(polygon []meshVertex, y float32, side float32) []meshVertex {
	var out []meshVertex

	for idx, a := range polygon {
		b := polygon[(idx+1)%len(polygon)]
		da, db := (a.pos[1]-y)*side, (b.pos[1]-y)*side

		if da >= 0 {
			out = append(out, a)
		}

		// the edge crosses the plane
		if (da >= 0) != (db >= 0) {
			out = append(out, a.lerp(b, da/(da-db)))
		}
	}

	return out
}

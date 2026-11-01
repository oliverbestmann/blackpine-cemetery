package physics

import (
	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// OverlapsCapsule reports if a capsule from a to b touches any shape with a
// CollisionLayers category in mask.
func (w PhysicsWorld) OverlapsCapsule(a, b glm.Vec3f, radius float32, mask uint64) bool {
	filter := b3.DefaultQueryFilter()
	filter.MaskBits = mask

	points := []b3.Vec3{toB3Vec(a), toB3Vec(b)}

	var hit bool
	w.World.OverlapShape(b3.Vec3{}, b3.MakeProxy(points, radius), filter, func(b3.Shape) bool {
		hit = true
		return false
	})

	return hit
}

package physics

import (
	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

type RayHit struct {
	Entity byke.EntityId
	Point  glm.Vec3f
	Normal glm.Vec3f

	// Distance from the ray origin to Point, in units of the ray direction's length.
	Distance float32

	body b3.Body
}

// CastRay returns the closest hit along origin + t*direction for t in [0, maxDistance].
func (w PhysicsWorld) CastRay(origin, direction glm.Vec3f, maxDistance float32) (RayHit, bool) {
	return w.CastRayMask(origin, direction, maxDistance, b3.DefaultQueryFilter().MaskBits)
}

// CastRayMask works like CastRay, but only hits shapes with a CollisionLayers category in mask.
func (w PhysicsWorld) CastRayMask(origin, direction glm.Vec3f, maxDistance float32, mask uint64) (RayHit, bool) {
	filter := b3.DefaultQueryFilter()
	filter.MaskBits = mask
	result := w.World.CastRayClosest(toB3Vec(origin), toB3Vec(direction.Scale(maxDistance)), filter)
	if result.Hit == 0 {
		return RayHit{}, false
	}

	body := b3.Shape{Id: result.ShapeId}.GetBody()

	return RayHit{
		Entity:   byke.EntityId(body.GetUserData()),
		Point:    toGlmVec(result.Point),
		Normal:   toGlmVec(result.Normal),
		Distance: result.Fraction * maxDistance,
		body:     body,
	}, true
}

// IsDynamic reports if the hit body is a dynamic body.
func (h RayHit) IsDynamic() bool {
	return h.body.GetType() == b3.DynamicBody
}

// Mass of the hit body in kg.
func (h RayHit) Mass() float32 {
	return h.body.GetMass()
}

// ApplyImpulse applies an impulse in N*s at the hit point and wakes the body.
// Only dynamic bodies react to it.
func (h RayHit) ApplyImpulse(impulse glm.Vec3f) {
	h.body.ApplyLinearImpulse(toB3Vec(impulse), toB3Vec(h.Point), 1)
}

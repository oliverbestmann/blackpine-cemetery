package physics

import (
	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// Hit is sent when a dynamic body hits another body faster than 1 m/s.
type Hit struct {
	Point glm.Vec3f

	// Speed is the approach speed in m/s.
	Speed float32
}

func hitEventsSystem(world PhysicsWorld, hits *byke.MessageWriter[Hit]) {
	for _, event := range world.World.GetContactEvents().HitEvents {
		hits.Write(Hit{Point: toGlmVec(event.Point), Speed: event.ApproachSpeed})
	}
}

// Explode pushes the dynamic bodies around center away from it and wakes them. Bodies within
// radius get impulsePerArea in N*s per m² of their area facing the center, the impulse fades
// out linearly over falloff beyond radius.
func (w PhysicsWorld) Explode(center glm.Vec3f, radius, falloff, impulsePerArea float32) {
	def := b3.DefaultExplosionDef()
	def.Position = toB3Vec(center)
	def.Radius = radius
	def.Falloff = falloff
	def.ImpulsePerArea = impulsePerArea
	w.World.Explode(def)
}

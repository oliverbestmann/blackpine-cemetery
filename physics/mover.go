package physics

import (
	"log/slog"
	"math"

	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// This is a port of DynamicMover from the box3d samples (samples/dynamic_mover.cpp,
// MIT, Erin Catto). The capsule is a dynamic body with locked rotation, so it collides and
// is pushed like everything else. A mover joint drives it to the target velocity and a pogo
// joint holds it above the ground, so it can climb steps without touching them.
//
// Differences to the sample: the body is the entity's own body, created from RigidBody and a
// capsule Collider. The input is a world direction instead of forward, right and throttle. The
// mover joints of all movers share one static base body, and moving platforms are not supported.

// MoverSettings tunes a Mover. Start with DefaultMoverSettings.
type MoverSettings struct {
	JumpSpeed  float32
	MaxSpeed   float32
	MinSpeed   float32
	StopSpeed  float32
	Accelerate float32

	// AirSteer is the share of the ground acceleration available in the air.
	AirSteer float32

	// Friction on the ground in units of 1/time.
	Friction float32

	// GravityScale makes the mover fall faster than the rest of the world. It is read once,
	// when the body is created.
	GravityScale float32

	// Force budget of the mover joint for reaching the target velocity. The air value is
	// small so the mover keeps its momentum while jumping.
	MaxGroundForce float32
	MaxAirForce    float32

	// PogoRestLength is the distance from the center of the lower capsule sphere to the ground.
	PogoRestLength   float32
	PogoHertz        float32
	PogoDampingRatio float32

	// Pogo force limits as a multiple of the mover weight.
	PogoCompressionScale float32
	PogoTensionScale     float32

	// SkirtPogo casts an octagon of rays around the capsule when the central ray misses or
	// hits a surface too steep to walk, so the mover can stand on a ledge.
	SkirtPogo bool

	// Surfaces steeper than this are not ground.
	MinGroundNormalY float32
}

// DefaultMoverSettings returns the settings of the box3d sample.
func DefaultMoverSettings() MoverSettings {
	return MoverSettings{
		JumpSpeed:            7,
		MaxSpeed:             6,
		MinSpeed:             0.1,
		StopSpeed:            3,
		Accelerate:           20,
		AirSteer:             0.25,
		Friction:             8,
		GravityScale:         1.5,
		MaxGroundForce:       6000,
		MaxAirForce:          1500,
		PogoRestLength:       0.9,
		PogoHertz:            3,
		PogoDampingRatio:     0.8,
		PogoCompressionScale: 100,
		PogoTensionScale:     100,
		SkirtPogo:            true,
		MinGroundNormalY:     0.7,
	}
}

// Mover walks a dynamic body with a capsule Collider, like a character. The body needs
// RigidBody{Type: Dynamic, LockRotation: true}. Set Direction and Jump from game code, the
// mover applies them before each physics step.
type Mover struct {
	byke.Component[Mover]
	MoverSettings

	// Direction is the walk direction in world space. Its length scales the speed and is
	// clamped to one, the vertical part is ignored.
	Direction glm.Vec3f

	// Jump jumps in the next step if the mover stands on walkable ground. The step resets it.
	Jump bool

	// OnGround is true if the pogo found ground below the mover, written by the mover.
	OnGround bool

	// Walkable is true if the ground is flat enough to walk on, written by the mover.
	Walkable bool

	state moverState
}

type moverState struct {
	initialized bool

	// the lower sphere center of the capsule, in the body frame
	center1 glm.Vec3f
	radius  float32
	filter  b3.QueryFilter

	moverJoint b3.MoverJoint
	pogoJoint  b3.PogoJoint
	surface    b3.Body
	cast       moverCast

	velocity      glm.Vec3f
	timeSliding   float32
	slideDuration float32
	jumping       bool
	jumpTicks     int

	// pogo joint state, carried across the joint being recreated
	pogoImpulse  float32
	pogoVelocity float32
}

type moverCast struct {
	point          b3.Pos
	normal         glm.Vec3f
	body           b3.Body
	userMaterialId uint64
	fraction       float32
	hit            bool
}

const moverSkirtCount = 8

// moverSystem updates all movers before the physics step.
func moverSystem(
	fixed byke.FixedTime,
	world PhysicsWorld,
	movers byke.Query[struct {
		Mover    *Mover
		Body     physicsBody
		Collider Collider
		Layers   byke.Option[CollisionLayers]
	}],
) {
	dt := float32(fixed.StepInterval.Seconds())

	for item := range movers.Items() {
		mover, body := item.Mover, item.Body.Body
		if body.IsEnabled() == 0 {
			continue
		}

		if !mover.state.initialized {
			if item.Collider.kind != capsuleCollider {
				slog.Warn("Mover needs a capsule collider")
				continue
			}

			layers := item.Layers.Or(CollisionLayers{Category: 1, Mask: math.MaxUint64})

			mover.state = moverState{
				initialized:   true,
				center1:       glm.V3f(0, -item.Collider.halfHeight*item.Body.scale.Y, 0),
				radius:        item.Collider.radius * max(item.Body.scale.X, item.Body.scale.Z),
				filter:        b3.QueryFilter{CategoryBits: layers.Category, MaskBits: layers.Mask},
				slideDuration: 0.2,
			}

			mover.createMoverJoint(world, body)
		}

		if mover.Jump {
			mover.jump(body)
			mover.Jump = false
		}

		mover.update(world, body, dt)
	}
}

func (m *Mover) createMoverJoint(world PhysicsWorld, body b3.Body) {
	def := b3.DefaultMoverJointDef()
	def.LinearVelocity = toB3Vec(m.state.velocity)
	def.MaxForce = m.MaxGroundForce
	def.Base.BodyIdA = world.anchor.Id
	def.Base.BodyIdB = body.Id
	def.Base.CollideConnected = 1

	m.state.moverJoint = world.World.CreateMoverJoint(def)
}

// jump jumps if the mover stands on walkable ground.
func (m *Mover) jump(body b3.Body) {
	if !m.OnGround || !m.Walkable {
		return
	}

	var surfaceVelocity float32
	if m.state.cast.body.IsValid() != 0 {
		surfaceVelocity = m.state.cast.body.GetWorldPointVelocity(m.state.cast.point).Y
	}

	mass := body.GetMass()
	vy := body.GetLinearVelocity().Y

	// remove the pogo constraint velocity but add the surface velocity
	dv := max(0, m.JumpSpeed-vy) + surfaceVelocity
	body.ApplyLinearImpulseToCenter(b3.Vec3{Y: mass * dv}, 1)

	m.launch()
}

// launch releases the ground, so an impulse can carry the mover into the air. Otherwise
// the pogo pulls it back down.
func (m *Mover) launch() {
	m.OnGround = false
	m.Walkable = false
	m.state.jumping = true
	m.state.jumpTicks = 0
}

// castPogo casts a ray down from origin and returns the closest hit, ignoring the mover and sensors.
func castPogo(world PhysicsWorld, body b3.Body, origin glm.Vec3f, translation glm.Vec3f, filter b3.QueryFilter) moverCast {
	var result moverCast

	world.World.CastRay(toB3Vec(origin), toB3Vec(translation), filter, func(shape b3.Shape, point b3.Pos, normal b3.Vec3, fraction float32, userMaterialId uint64, _, _ int32) float32 {
		hitBody := shape.GetBody()

		// the probes start inside or on the capsule, sensors can't hold up the mover
		if hitBody.Id == body.Id || shape.IsSensor() != 0 {
			return -1
		}

		result = moverCast{
			point:          point,
			normal:         toGlmVec(normal),
			body:           hitBody,
			userMaterialId: userMaterialId,
			fraction:       fraction,
			hit:            true,
		}

		return fraction
	})

	return result
}

// update follows the Quake ground and air model:
// https://github.com/id-Software/Quake/blob/master/QW/client/pmove.c#L390
//
// OnGround controls friction, the mover force and the pogo step down. Walkable controls the
// air steer. jumping keeps the pogo from pulling down.
func (m *Mover) update(world PhysicsWorld, body b3.Body, dt float32) {
	s := &m.state

	// reach further while grounded, so the spring finds the ground over a step
	stepDownLength := m.PogoRestLength
	rayLength := m.PogoRestLength
	if m.OnGround {
		rayLength += stepDownLength
	}

	translation := glm.V3f(0, -rayLength, 0)
	origin := toGlmVec(body.GetPosition()).Add(s.center1)

	s.cast = castPogo(world, body, origin, translation, s.filter)

	var pogoOffset glm.Vec3f

	centerWalkable := s.cast.hit && s.cast.normal.Y >= m.MinGroundNormalY
	if !centerWalkable && m.SkirtPogo {
		// the skirt turns with the direction of travel, so ties go to the leading probe
		velocity := toGlmVec(body.GetLinearVelocity())
		heading := glm.V3f(velocity.X, 0, velocity.Z)
		if heading.Dot(heading) < 1e-7 {
			heading = glm.V3f(1, 0, 0)
		}

		heading = heading.Normalize()
		side := glm.V3f(-heading.Z, 0, heading.X)

		bestDisplacement := float32(math.MaxFloat32)
		for idx := range moverSkirtCount {
			angle := 2 * math.Pi * float64(idx) / moverSkirtCount
			offset := heading.Scale(float32(math.Cos(angle))).Add(side.Scale(float32(math.Sin(angle)))).Scale(s.radius)

			result := castPogo(world, body, origin.Add(offset), translation, s.filter)

			// use the walkable result that moves the mover the least. Without one the mover
			// keeps the central hit and slides off the steep surface
			if result.hit && result.normal.Y >= m.MinGroundNormalY {
				displacement := abs(result.fraction*rayLength - m.PogoRestLength)
				if displacement < bestDisplacement {
					bestDisplacement = displacement
					s.cast = result
					pogoOffset = offset
				}
			}
		}
	}

	// should jumping end?
	if s.jumping {
		s.jumpTicks++

		// the ticks give the jump time to leave the ground, otherwise jumping ends when
		// the mover approaches the current ground
		if s.jumpTicks > 2 && s.cast.hit && s.velocity.Dot(s.cast.normal) <= 0 {
			s.jumping = false
		}
	}

	if s.cast.hit {
		// is the ground too steep to walk?
		m.Walkable = s.cast.normal.Y >= m.MinGroundNormalY

		// the cast can still hit while jumping
		m.OnGround = !s.jumping

		if m.OnGround {
			if m.Walkable {
				s.timeSliding = max(s.timeSliding-dt, 0)
			} else {
				s.timeSliding = min(s.timeSliding+dt, s.slideDuration)
			}
		}
	} else {
		m.OnGround = false
		m.Walkable = false
		s.timeSliding = max(s.timeSliding-dt, 0)
	}

	// the mover joint is destroyed with the base body, recreate it
	if s.moverJoint.IsValid() == 0 {
		m.createMoverJoint(world, body)
	}

	// the base body is static, the velocity is relative to the world
	s.velocity = toGlmVec(body.GetLinearVelocity())

	// friction acts in the horizontal plane, the pogo owns the vertical motion
	if m.OnGround {
		speed := float32(math.Hypot(float64(s.velocity.X), float64(s.velocity.Z)))
		if speed < m.MinSpeed {
			s.velocity.X = 0
			s.velocity.Z = 0
		} else {
			// linear damping above StopSpeed and a fixed reduction below
			control := max(speed, m.StopSpeed)

			// friction has units of 1/time
			drop := control * m.Friction * dt
			ratio := max(0, speed-drop) / speed
			s.velocity.X *= ratio
			s.velocity.Z *= ratio
		}
	}

	// mover force
	maxForce := m.MaxAirForce
	if m.OnGround {
		if s.slideDuration > 0 {
			maxForce = clamp01(1-s.timeSliding/s.slideDuration) * m.MaxGroundForce
		} else if m.Walkable {
			maxForce = m.MaxGroundForce
		} else {
			maxForce = 0
		}
	}

	s.moverJoint.SetMaxVelocityForce(maxForce)

	desiredVelocity := glm.V3f(m.Direction.X, 0, m.Direction.Z).Scale(m.MaxSpeed)
	desiredSpeed := min(desiredVelocity.Length(), m.MaxSpeed)

	var desiredDirection glm.Vec3f
	if desiredSpeed > 0 {
		desiredDirection = desiredVelocity.Normalize()
	}

	// accelerate
	addSpeed := desiredSpeed - s.velocity.Dot(desiredDirection)
	if addSpeed > 0 {
		steer := m.AirSteer
		if m.Walkable {
			steer = 1
		}

		accelSpeed := min(steer*m.Accelerate*m.MaxSpeed*dt, addSpeed)
		s.velocity.X += accelSpeed * desiredDirection.X
		s.velocity.Z += accelSpeed * desiredDirection.Z
	}

	normalLength := s.cast.normal.Length()
	if s.cast.hit && abs(normalLength-1) < 1e-4 {
		m.updatePogo(world, body, pogoOffset)
	} else {
		if s.pogoJoint.IsValid() != 0 {
			s.pogoJoint.DestroyJoint(0)
		}

		s.pogoJoint = b3.PogoJoint{}
		s.surface = b3.Body{}
		s.pogoImpulse = 0
		s.pogoVelocity = 0
	}

	s.moverJoint.SetLinearVelocity(toB3Vec(s.velocity))
}

// updatePogo creates or moves the pogo joint to the ground found by the cast.
func (m *Mover) updatePogo(world PhysicsWorld, body b3.Body, pogoOffset glm.Vec3f) {
	s := &m.state

	gravity := toGlmVec(world.World.GetGravity()).Length()
	weight := m.GravityScale * gravity * body.GetMass()

	// the pogo can pull down at a multiple of the gravity force, but not while jumping
	maxTension := m.PogoTensionScale * weight
	if s.jumping {
		maxTension = 0
	}

	localA := b3.Transform{P: s.cast.body.GetLocalPoint(s.cast.point), Q: b3.Quat{S: 1}}
	localB := b3.Transform{P: toB3Vec(s.center1.Add(pogoOffset)), Q: b3.Quat{S: 1}}

	valid := s.pogoJoint.IsValid() != 0

	// keep the joint while the mover stands on the same body, that is faster
	if valid && s.cast.body.Id == s.surface.Id {
		s.pogoJoint.SetLocalFrameA(localA)
		s.pogoJoint.SetLocalFrameB(localB)
		s.pogoJoint.SetNormal(toB3Vec(s.cast.normal))
		s.pogoJoint.SetMaxTensionForce(maxTension)
		s.pogoJoint.SetMaxCompressionForce(m.PogoCompressionScale * weight)
		s.pogoJoint.SetSpringHertz(m.PogoHertz)
		s.pogoJoint.SetSpringDampingRatio(m.PogoDampingRatio)
		return
	}

	if valid {
		s.pogoImpulse = s.pogoJoint.GetImpulse()
		s.pogoVelocity = s.pogoJoint.GetCurrentSpeed()
		s.pogoJoint.DestroyJoint(0)
	}

	s.surface = s.cast.body

	def := b3.DefaultPogoJointDef()
	def.Base.LocalFrameA = localA
	def.Base.LocalFrameB = localB
	def.Base.BodyIdA = s.surface.Id
	def.Base.BodyIdB = body.Id
	def.Base.CollideConnected = 1
	def.Normal = toB3Vec(s.cast.normal)
	def.RestLength = m.PogoRestLength
	def.Hertz = m.PogoHertz
	def.DampingRatio = m.PogoDampingRatio
	def.MaxCompressionForce = m.PogoCompressionScale * weight
	def.MaxTensionForce = maxTension

	// warm start from the joint that was just destroyed
	def.Impulse = s.pogoImpulse
	def.Velocity = s.pogoVelocity

	s.pogoJoint = world.World.CreatePogoJoint(def)
}

func clamp01(v float32) float32 {
	return max(0, min(1, v))
}

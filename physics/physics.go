// Package physics runs box3d rigid body physics on byke entities, modelled after avian3d.
package physics

import (
	"log/slog"
	"math"

	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

type BodyType uint8

const (
	Static BodyType = iota
	Dynamic
	Kinematic
)

// RigidBody turns an entity into a physics body. The body is created in the
// fixed step after the entity's GlobalTransform was first computed.
type RigidBody struct {
	byke.ImmutableComponent[RigidBody]
	Type BodyType

	// LockRotation keeps the body upright, e.g. for characters.
	LockRotation bool

	// Density in kg/m³, zero uses the box3d default of 1000.
	Density float32

	// Friction of the shapes, zero uses the box3d default.
	Friction float32

	// LinearDamping slows the body down, zero means none.
	LinearDamping float32

	// Bullet enables continuous collision against dynamic bodies, for fast bodies.
	Bullet bool

	// Restitution makes the shapes bouncy, zero means no bounce.
	Restitution float32
}

// CollisionLayers sets the collision filter of all shapes of a body. Two shapes collide if
// the category of each is in the mask of the other. The zero value is not used, missing
// CollisionLayers collide with everything.
type CollisionLayers struct {
	byke.ImmutableComponent[CollisionLayers]
	Category uint64
	Mask     uint64
}

// ControlledVelocity marks a dynamic body whose LinearVelocity component is written into
// the body before every step, so game code can steer it, e.g. a character.
type ControlledVelocity struct {
	byke.ImmutableComponent[ControlledVelocity]

	// Angular also writes the AngularVelocity component into the body.
	Angular bool
}

func (RigidBody) RequireComponents() []byke.ErasedComponent {
	return []byke.ErasedComponent{byke2d.NewTransform(), LinearVelocity{}, AngularVelocity{}}
}

// LinearVelocity is used as the initial velocity of a body and then mirrors the
// velocity of dynamic bodies after each step.
type LinearVelocity struct {
	byke.ComparableComponent[LinearVelocity]
	glm.Vec3f
}

// AngularVelocity works like LinearVelocity, in radians per second.
type AngularVelocity struct {
	byke.ComparableComponent[AngularVelocity]
	glm.Vec3f
}

// Sensor turns all colliders of a body into sensors. SensorOverlaps lists what they touch.
type Sensor struct {
	byke.ImmutableComponent[Sensor]
}

func (Sensor) RequireComponents() []byke.ErasedComponent {
	return []byke.ErasedComponent{SensorOverlaps{}}
}

type colliderKind uint8

const (
	sphereCollider colliderKind = iota
	capsuleCollider
	cuboidCollider
	trimeshCollider
	hullCollider
	compoundCollider
)

// Collider is an explicit collision shape in the local space of the body entity.
// The entity's scale is applied to the shape.
type Collider struct {
	byke.Component[Collider]

	kind        colliderKind
	radius      float32
	halfHeight  float32
	halfExtents glm.Vec3f
	offset      glm.Vec3f
	vertices    []glm.Vec3f
	indices     []uint32
	parts       []Collider
}

func SphereCollider(radius float32) Collider {
	return Collider{kind: sphereCollider, radius: radius}
}

// CapsuleCollider creates a capsule along the y axis. length is the distance between the two sphere centers.
func CapsuleCollider(radius, length float32) Collider {
	return Collider{kind: capsuleCollider, radius: radius, halfHeight: length / 2}
}

func CuboidCollider(halfExtents glm.Vec3f) Collider {
	return Collider{kind: cuboidCollider, halfExtents: halfExtents}
}

// CylinderCollider creates a cylinder along the y axis, centered at the origin.
// It is a convex hull with cylinderSides sides.
func CylinderCollider(radius, height float32) Collider {
	const cylinderSides = 16

	var points []glm.Vec3f
	for idx := range cylinderSides {
		angle := 2 * math.Pi * float64(idx) / cylinderSides
		x, z := radius*float32(math.Cos(angle)), radius*float32(math.Sin(angle))
		points = append(points, glm.Vec3f{x, -height / 2, z}, glm.Vec3f{x, height / 2, z})
	}

	return HullCollider(points)
}

// HullCollider creates the convex hull of points. box3d reduces it to at most
// maxHullVertices vertices. If the hull fails, the bounding box is used instead.
func HullCollider(points []glm.Vec3f) Collider {
	return Collider{kind: hullCollider, vertices: points}
}

// CompoundCollider combines several colliders into one body, e.g. convex hulls around
// the parts of a concave model. Compounds can not be nested.
func CompoundCollider(parts ...Collider) Collider {
	return Collider{kind: compoundCollider, parts: parts}
}

// flatten returns the parts of a compound collider, moved by its offset, or the collider itself.
func (c Collider) flatten() []Collider {
	if c.kind != compoundCollider {
		return []Collider{c}
	}

	parts := make([]Collider, len(c.parts))
	for idx, part := range c.parts {
		part.offset = part.offset.Add(c.offset)
		parts[idx] = part
	}

	return parts
}

// WithOffset moves the collider by offset in the local space of the body entity.
// Spheres, capsules and triangle meshes ignore the offset.
func (c Collider) WithOffset(offset glm.Vec3f) Collider {
	c.offset = offset
	return c
}

// TriMeshCollider creates a triangle mesh with counter clockwise winding. If indices
// is nil, every three vertices form a triangle. Dynamic bodies do not support triangle
// meshes and get the bounding box instead.
func TriMeshCollider(vertices []glm.Vec3f, indices []uint32) Collider {
	return Collider{kind: trimeshCollider, vertices: vertices, indices: indices}
}

// boundingBoxCollider returns a cuboid around all vertices.
func boundingBoxCollider(vertices []glm.Vec3f) Collider {
	if len(vertices) == 0 {
		return CuboidCollider(glm.Vec3f{})
	}

	lo, hi := vertices[0], vertices[0]
	for _, v := range vertices {
		lo, hi = lo.Min(v), hi.Max(v)
	}

	c := CuboidCollider(hi.Sub(lo).Scale(0.5))
	c.offset = lo.Add(hi).Scale(0.5)
	return c
}

type ColliderConstructor uint8

const (
	// TriMesh creates one triangle mesh per mesh in the hierarchy.
	TriMesh ColliderConstructor = iota

	// BoundingBox creates one box around all meshes in the hierarchy, in the body's local frame.
	BoundingBox

	// ConvexHull creates one convex hull around all meshes in the hierarchy.
	ConvexHull
)

// ColliderConstructorHierarchy builds colliders from the MeshInstance entities of the
// body entity and its descendants. Descendants with their own RigidBody are skipped.
type ColliderConstructorHierarchy struct {
	byke.ImmutableComponent[ColliderConstructorHierarchy]
	Constructor ColliderConstructor
}

// PhysicsWorld holds the box3d world. Use it for queries or to change settings like gravity.
type PhysicsWorld struct {
	World b3.World

	// anchor is a static body without shapes, the base of the mover joints
	anchor b3.Body
}

type physicsBody struct {
	byke.Component[physicsBody]
	Body   b3.Body
	meshes []b3.OwnedMesh

	// the entity's world scale, for debug rendering
	scale glm.Vec3f
}

// scaledCollider is a collider in the body frame, scale is applied to it when creating the shape.
type scaledCollider struct {
	Collider
	scale glm.Vec3f
}

// SystemsWriteBack copies the body poses and velocities into the components, in FixedPostUpdate.
var SystemsWriteBack = &byke.SystemSet{Name: "PhysicsWriteBack"}

func Plugin(app *byke.App) {
	def := b3.DefaultWorldDef()
	def.Gravity = b3.Vec3{Y: -9.81}
	world := b3.CreateWorld(def)
	app.InsertResource(PhysicsWorld{World: world, anchor: world.CreateBody(b3.DefaultBodyDef())})

	// also fires on despawn
	app.World().RegisterComponentHooks[physicsBody]().OnRemove(destroyBodyHook)

	app.AddSystems(byke.FixedPreUpdate, byke.System(createBodiesSystem, teleportSystem, disableSystem, syncKinematicSystem, controlVelocitySystem, moverSystem).Chain())
	app.AddSystems(byke.FixedUpdate, stepSystem)
	app.AddMessage[Hit]()
	app.AddSystems(byke.FixedPostUpdate, byke.System(writeBackSystem).InSet(SystemsWriteBack), sensorOverlapsSystem, hitEventsSystem)
}

// constructorColliders builds the colliders of a ColliderConstructorHierarchy from the meshes below the entity.
// It returns false while some descendants have no GlobalTransform yet.
func constructorColliders(
	nodes byke.Query[nodeItem],
	entityId byke.EntityId,
	bodyType BodyType,
	kind ColliderConstructor,
	translation glm.Vec3f,
	rotation glm.Quat,
) ([]scaledCollider, bool) {
	if kind == TriMesh && bodyType == Dynamic {
		slog.Warn("TriMesh colliders need a static or kinematic body, using BoundingBox",
			slog.Int("entity", int(entityId)))
		kind = BoundingBox
	}

	// vertices go into the body frame, which has no scale
	bodyMatrix := glm.TranslationMat4f(translation.XYZ())
	bodyMatrix.RotateAssign(rotation)
	bodyInv, _ := bodyMatrix.TryInverse()

	meshes, ok := hierarchyMeshes(nodes, entityId, bodyInv, true)
	if !ok {
		return nil, false
	}

	if len(meshes) == 0 {
		slog.Warn("No meshes found for ColliderConstructorHierarchy", slog.Int("entity", int(entityId)))
	}

	one := glm.Vec3f{1, 1, 1}

	var colliders []scaledCollider

	switch kind {
	case TriMesh:
		for _, mesh := range meshes {
			colliders = append(colliders, scaledCollider{mesh, one})
		}

	case BoundingBox, ConvexHull:
		var vertices []glm.Vec3f
		for _, mesh := range meshes {
			vertices = append(vertices, mesh.vertices...)
		}

		c := boundingBoxCollider(vertices)
		if kind == ConvexHull {
			c = HullCollider(vertices)
		}

		colliders = append(colliders, scaledCollider{c, one})
	}

	return colliders, true
}

func destroyBodyHook(_ byke.DeferredWorld, entity byke.EntityRef, ty *byke.ComponentType) {
	body := entity.Get(ty).(*physicsBody)
	body.Body.DestroyBody()

	// shapes use the meshes, destroy them after the body
	for _, mesh := range body.meshes {
		mesh.Destroy()
	}
}

type nodeItem struct {
	Children        byke.Option[byke.Children]
	Mesh            byke.Option[byke2d.MeshInstance]
	GlobalTransform byke2d.GlobalTransform
	RigidBody       byke.Has[RigidBody]
}

func createBodiesSystem(
	commands *byke.Commands,
	world PhysicsWorld,
	bodies byke.Query[struct {
		_ byke.Without[physicsBody]

		EntityId        byke.EntityId
		RigidBody       RigidBody
		GlobalTransform byke2d.GlobalTransform
		LinearVelocity  LinearVelocity
		AngularVelocity AngularVelocity
		Collider        byke.Option[Collider]
		Constructor     byke.Option[ColliderConstructorHierarchy]
		Sensor          byke.Has[Sensor]
		Mover           byke.Option[Mover]
		Layers          byke.Option[CollisionLayers]
	}],
	nodes byke.Query[nodeItem],
) {
	for item := range bodies.Items() {
		if !propagated(item.GlobalTransform) {
			continue
		}

		translation, rotation, scale := decompose(item.GlobalTransform.Affine)

		var colliders []scaledCollider

		if c, ok := item.Collider.Get(); ok {
			for _, part := range c.flatten() {
				colliders = append(colliders, scaledCollider{part, scale})
			}
		}

		if ctor, ok := item.Constructor.Get(); ok {
			constructed, ok := constructorColliders(nodes, item.EntityId, item.RigidBody.Type, ctor.Constructor, translation, rotation)
			if !ok {
				// some descendants have no GlobalTransform yet, try again next step
				continue
			}

			colliders = append(colliders, constructed...)
		}

		def := b3.DefaultBodyDef()
		def.Type1 = item.RigidBody.Type.b3()
		def.Position = toB3Vec(translation)
		def.Rotation = toB3Quat(rotation)
		def.LinearVelocity = toB3Vec(item.LinearVelocity.Vec3f)
		def.AngularVelocity = toB3Vec(item.AngularVelocity.Vec3f)
		def.UserData = uint64(item.EntityId)
		def.LinearDamping = item.RigidBody.LinearDamping
		def.IsBullet = boolU8(item.RigidBody.Bullet)
		mover, isMover := item.Mover.Get()
		if isMover {
			def.GravityScale = mover.GravityScale
			def.EnableSleep = 0
		}
		if item.RigidBody.LockRotation {
			def.MotionLocks = b3.MotionLocks{AngularX: 1, AngularY: 1, AngularZ: 1}
		}

		body := physicsBody{Body: world.World.CreateBody(def), scale: scale}

		shapeDef := b3.DefaultShapeDef()
		if item.Sensor.Exists() {
			shapeDef.IsSensor = 1
		}

		// sensors only see shapes with sensor events, static bodies never enter a sensor
		shapeDef.EnableSensorEvents = boolU8(item.Sensor.Exists() || item.RigidBody.Type != Static)
		shapeDef.EnableHitEvents = boolU8(item.RigidBody.Type == Dynamic)

		if item.RigidBody.Density > 0 {
			shapeDef.Density = item.RigidBody.Density
		}

		if item.RigidBody.Friction > 0 {
			shapeDef.BaseMaterial.Friction = item.RigidBody.Friction
		}

		shapeDef.BaseMaterial.Restitution = item.RigidBody.Restitution

		// the mover joint and the pogo joint do all the movement, friction would fight them
		if isMover {
			shapeDef.BaseMaterial.Friction = 0
			shapeDef.BaseMaterial.Restitution = 0
		}

		if layers, ok := item.Layers.Get(); ok {
			shapeDef.Filter.CategoryBits = layers.Category
			shapeDef.Filter.MaskBits = layers.Mask
		}

		for _, c := range colliders {
			if mesh := addShape(body.Body, shapeDef, item.RigidBody.Type, c.Collider, c.scale); mesh.MeshData != nil {
				body.meshes = append(body.meshes, mesh)
			}
		}

		commands.Entity(item.EntityId).Insert(body)
	}
}

// propagated reports if the transform propagation has run for this entity. The zero value is not a valid affine matrix.
func propagated(t byke2d.GlobalTransform) bool {
	return t.Affine[3][3] != 0
}

// hierarchyMeshes collects the meshes of the entity and its descendants as triangle
// mesh colliders, with vertices transformed by bodyInv. ok is false if a descendant
// has no GlobalTransform yet.
func hierarchyMeshes(nodes byke.Query[nodeItem], entityId byke.EntityId, bodyInv glm.Mat4f, root bool) (colliders []Collider, ok bool) {
	node, found := nodes.Get(entityId)
	if !found || (!root && node.RigidBody.Exists()) {
		return nil, true
	}

	if mesh, ok := node.Mesh.Get(); ok && mesh.Mesh != nil {
		if !propagated(node.GlobalTransform) {
			return nil, false
		}

		colliders = append(colliders, meshCollider(mesh.Mesh, bodyInv.Mul(node.GlobalTransform.Affine)))
	}

	if children, ok := node.Children.Get(); ok {
		for _, child := range children.Children() {
			childColliders, ok := hierarchyMeshes(nodes, child, bodyInv, false)
			if !ok {
				return nil, false
			}

			colliders = append(colliders, childColliders...)
		}
	}

	return colliders, true
}

func meshCollider(mesh *byke2d.Mesh, transform glm.Mat4f) Collider {
	vertices := make([]glm.Vec3f, 0, len(mesh.Vertices()))
	for _, v := range mesh.Vertices() {
		vertices = append(vertices, transform.Transform3(v))
	}

	indices := mesh.Indices()
	if indices == nil {
		for idx := range uint32(len(vertices)) {
			indices = append(indices, idx)
		}
	}

	if determinant(transform) < 0 {
		// mirrored, keep the triangles counter clockwise
		indices = append([]uint32(nil), indices...)
		for i := 0; i+2 < len(indices); i += 3 {
			indices[i+1], indices[i+2] = indices[i+2], indices[i+1]
		}
	}

	return TriMeshCollider(vertices, indices)
}

func determinant(m glm.Mat4f) float32 {
	a := glm.Vec3f{m[0][0], m[0][1], m[0][2]}
	b := glm.Vec3f{m[1][0], m[1][1], m[1][2]}
	c := glm.Vec3f{m[2][0], m[2][1], m[2][2]}
	return a.Cross(b).Dot(c)
}

// addShape creates the shape of c on the body. It returns the mesh for trimesh shapes,
// which must live as long as the body.
func addShape(body b3.Body, def b3.ShapeDef, bodyType BodyType, c Collider, scale glm.Vec3f) b3.OwnedMesh {
	if c.kind == trimeshCollider && bodyType == Dynamic {
		slog.Warn("TriMesh colliders need a static or kinematic body, using its bounding box")
		c = boundingBoxCollider(c.vertices)
	}

	absScale := glm.Vec3f{abs(scale[0]), abs(scale[1]), abs(scale[2])}
	maxScale := max(absScale[0], absScale[1], absScale[2])

	switch c.kind {
	case sphereCollider:
		body.CreateSphereShape(def, b3.Sphere{Radius: c.radius * maxScale})

	case capsuleCollider:
		h := c.halfHeight * absScale[1]
		body.CreateCapsuleShape(def, b3.Capsule{
			Center1: b3.Vec3{Y: -h},
			Center2: b3.Vec3{Y: h},
			Radius:  c.radius * max(absScale[0], absScale[2]),
		})

	case cuboidCollider:
		half, center := cuboidBox(c, scale)
		box := b3.MakeOffsetBoxHull(half[0], half[1], half[2], toB3Vec(center))
		body.CreateHullShape(def, &box.Base)

	case hullCollider:
		points := make([]b3.Vec3, len(c.vertices))
		for idx, v := range c.vertices {
			points[idx] = toB3Vec(v.Add(c.offset).Mul(scale))
		}

		hull := b3.CreateHull(points, maxHullVertices)
		if hull.HullData == nil {
			slog.Warn("Failed to create hull collider, using its bounding box", slog.Int("points", len(points)))
			box := boundingBoxCollider(c.vertices)
			box.offset = box.offset.Add(c.offset)
			return addShape(body, def, bodyType, box, scale)
		}

		// the shape copies the hull
		body.CreateHullShape(def, hull.HullData)
		hull.Destroy()

	case trimeshCollider:
		vertices := make([]b3.Vec3, len(c.vertices))
		for idx, v := range c.vertices {
			vertices[idx] = toB3Vec(v)
		}

		indices := make([]int32, 0, len(c.indices))
		for _, idx := range c.indices {
			indices = append(indices, int32(idx))
		}

		if indices == nil {
			for idx := range int32(len(vertices)) {
				indices = append(indices, idx)
			}
		}

		mesh := b3.CreateMesh(b3.MeshDef{IdentifyEdges: 1}, vertices, indices, nil)
		if mesh.MeshData == nil {
			slog.Warn("Failed to create mesh collider", slog.Int("triangles", len(indices)/3))
			return mesh
		}

		body.CreateMeshShape(def, mesh.MeshData, toB3Vec(scale))
		return mesh
	}

	return b3.OwnedMesh{}
}

// maxHullVertices is the vertex limit of convex hulls. box3d allows 128 edges, a hull with
// v vertices has up to 3v-6 edges, so round shapes with more than 44 vertices fail.
const maxHullVertices = 44

// cuboidBox returns the half extents and center of a cuboid collider with scale applied.
func cuboidBox(c Collider, scale glm.Vec3f) (half, center glm.Vec3f) {
	absScale := glm.Vec3f{abs(scale[0]), abs(scale[1]), abs(scale[2])}

	// box3d can't handle flat boxes
	const minHalfExtent = 0.005
	half = c.halfExtents.Mul(absScale).Max(glm.Vec3f{minHalfExtent, minHalfExtent, minHalfExtent})
	return half, c.offset.Mul(scale)
}

func abs(v float32) float32 {
	return max(v, -v)
}

func (t BodyType) b3() b3.BodyType {
	switch t {
	case Dynamic:
		return b3.DynamicBody
	case Kinematic:
		return b3.KinematicBody
	default:
		return b3.StaticBody
	}
}

func syncKinematicSystem(fixed byke.FixedTime, bodies byke.Query[struct {
	RigidBody       RigidBody
	GlobalTransform byke2d.GlobalTransform
	Body            physicsBody
}],
) {
	dt := float32(fixed.StepInterval.Seconds())

	for item := range bodies.Items() {
		if item.RigidBody.Type != Kinematic {
			continue
		}

		translation, rotation, _ := decompose(item.GlobalTransform.Affine)
		item.Body.Body.SetTargetTransform(b3.Transform{P: toB3Vec(translation), Q: toB3Quat(rotation)}, dt, 1)
	}
}

func controlVelocitySystem(bodies byke.Query[struct {
	Control         ControlledVelocity
	Body            physicsBody
	LinearVelocity  LinearVelocity
	AngularVelocity AngularVelocity
}],
) {
	for item := range bodies.Items() {
		item.Body.Body.SetLinearVelocity(toB3Vec(item.LinearVelocity.Vec3f))

		if item.Control.Angular {
			item.Body.Body.SetAngularVelocity(toB3Vec(item.AngularVelocity.Vec3f))
		}
	}
}

func boolU8(b bool) uint8 {
	if b {
		return 1
	}

	return 0
}

func stepSystem(fixed byke.FixedTime, world PhysicsWorld) {
	world.World.Step(float32(fixed.StepInterval.Seconds()), 4)
}

func writeBackSystem(
	world PhysicsWorld,
	bodies byke.Query[struct {
		RigidBody       RigidBody
		Body            physicsBody
		ChildOf         byke.Option[byke.ChildOf]
		Transform       *byke2d.Transform
		LinearVelocity  *LinearVelocity
		AngularVelocity *AngularVelocity
	}],
	parents byke.Query[struct{ GlobalTransform byke2d.GlobalTransform }],
) {
	for _, event := range world.World.GetBodyEvents().MoveEvents {
		item, ok := bodies.Get(byke.EntityId(event.UserData))
		if !ok || item.RigidBody.Type != Dynamic {
			continue
		}

		pose := poseMatrix(event.Transform)
		if childOf, ok := item.ChildOf.Get(); ok {
			if parent, ok := parents.Get(childOf.Parent); ok {
				inv, _ := parent.GlobalTransform.Affine.TryInverse()
				pose = inv.Mul(pose)
			}
		}

		translation, rotation, _ := decompose(pose)
		item.Transform.Translation = translation
		item.Transform.Rotation = rotation

		item.LinearVelocity.Vec3f = toGlmVec(item.Body.Body.GetLinearVelocity())
		item.AngularVelocity.Vec3f = toGlmVec(item.Body.Body.GetAngularVelocity())
	}
}

package physics

import (
	"math"
	"slices"
	"testing"
	"time"

	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

func near(a, b glm.Vec3f) bool {
	return nearTol(a, b, 1e-4)
}

func nearTol(a, b glm.Vec3f, tolerance float32) bool {
	return a.Sub(b).Length() < tolerance
}

// newTestApp returns an app with the physics plugin. The box3d world is destroyed after the test,
// box3d only has a fixed number of world slots.
func newTestApp(t *testing.T) *byke.App {
	app := new(byke.App)
	app.AddPlugin(Plugin)
	app.InsertResource(byke.FixedTime{StepInterval: time.Second / 64})

	t.Cleanup(func() {
		physicsWorld, _ := app.World().ResourceOf[PhysicsWorld]()
		physicsWorld.World.DestroyWorld()
	})

	return app
}

func TestQuatConversion(t *testing.T) {
	q := glm.QuatFromAxisAngle(glm.Vec3f{1, 2, 3}.Normalize(), 1.2)
	v := glm.Vec3f{0.3, -1, 2}

	got := toGlmVec(b3.RotateVector(toB3Quat(q), toB3Vec(v)))
	if want := q.Transform(v); !near(got, want) {
		t.Fatalf("b3 rotates %v to %v, glm to %v", v, got, want)
	}

	if back := toGlmQuat(toB3Quat(q)); !near(back.Transform(v), q.Transform(v)) {
		t.Fatalf("round trip changed the rotation")
	}
}

func TestDecompose(t *testing.T) {
	transform := byke2d.NewTransform().
		WithTranslationXYZ(1, 2, 3).
		WithRotation(glm.QuatFromAxisAngle(glm.Vec3f{0, 1, 1}.Normalize(), 2.5)).
		WithScaleXYZ(2, 3, 0.5)

	translation, rotation, scale := decompose(transform.Affine3())

	if !near(translation, transform.Translation) || !near(scale, transform.Scale) {
		t.Fatalf("got %v %v", translation, scale)
	}

	v := glm.Vec3f{1, -2, 0.5}
	if !near(rotation.Transform(v), transform.Rotation.Transform(v)) {
		t.Fatalf("rotation differs")
	}
}

func TestBoxFallsOntoTriMesh(t *testing.T) {
	app := newTestApp(t)

	world := app.World()

	// 20x20 ground quad at y=1, counter clockwise seen from above
	ground := byke2d.NewTransform().WithTranslationXYZ(0, 1, 0)
	world.Spawn([]byke.ErasedComponent{
		ground,
		byke2d.GlobalTransform{Affine: ground.Affine3()},
		RigidBody{Type: Static},
		TriMeshCollider(
			[]glm.Vec3f{{-10, 0, -10}, {-10, 0, 10}, {10, 0, 10}, {10, 0, -10}},
			[]uint32{0, 1, 2, 0, 2, 3},
		),
	})

	box := byke2d.NewTransform().
		WithTranslationXYZ(0, 5, 0).
		WithRotation(glm.RotationYQuat(0.3))

	boxId := world.Spawn([]byke.ErasedComponent{
		box,
		byke2d.GlobalTransform{Affine: box.Affine3()},
		RigidBody{Type: Dynamic},
		CuboidCollider(glm.Vec3f{0.5, 0.5, 0.5}),
	})

	for range 5 * 64 {
		world.RunSchedule(byke.FixedPreUpdate)
		world.RunSchedule(byke.FixedUpdate)
		world.RunSchedule(byke.FixedPostUpdate)
	}

	query := world.Query[struct {
		Transform      byke2d.Transform
		LinearVelocity LinearVelocity
	}]()

	item, ok := query.Get(boxId)
	if !ok {
		t.Fatal("box not found")
	}

	if y := item.Transform.Translation[1]; math.Abs(float64(y-1.5)) > 0.02 {
		t.Fatalf("box should rest at y=1.5, is at %v", y)
	}

	if speed := item.LinearVelocity.Length(); speed > 0.01 {
		t.Fatalf("box should rest, has speed %v", speed)
	}

	// rotation around y must survive the round trip through box3d
	v := glm.Vec3f{1, 0, 0}
	if item.Transform.Rotation.Transform(v).Sub(box.Rotation.Transform(v)).Length() > 0.01 {
		t.Fatalf("rotation changed")
	}

	physicsWorld, _ := world.ResourceOf[PhysicsWorld]()

	hit, ok := physicsWorld.CastRay(glm.Vec3f{0, 10, 0}, glm.Vec3f{0, -1, 0}, 20)
	if !ok || hit.Entity != boxId || !hit.IsDynamic() || math.Abs(float64(hit.Distance-8)) > 0.02 {
		t.Fatalf("expected to hit the box top at distance 8, got %+v (ok=%v)", hit, ok)
	}

	hit.ApplyImpulse(glm.Vec3f{2 * hit.Mass(), 0, 0})
	world.RunSchedule(byke.FixedUpdate)
	world.RunSchedule(byke.FixedPostUpdate)

	item, _ = query.Get(boxId)
	if item.LinearVelocity.Vec3f[0] < 1 {
		t.Fatalf("impulse should push the box along +x, velocity is %v", item.LinearVelocity.Vec3f)
	}
}

func TestSensorOverlaps(t *testing.T) {
	app := newTestApp(t)

	world := app.World()

	// static sensor volume from y=0 to y=2, a box falls into it and rests on the floor below
	sensor := byke2d.NewTransform().WithTranslationXYZ(0, 1, 0)
	sensorId := world.Spawn([]byke.ErasedComponent{
		sensor,
		byke2d.GlobalTransform{Affine: sensor.Affine3()},
		RigidBody{Type: Static},
		Sensor{},
		CuboidCollider(glm.Vec3f{2, 1, 2}),
	})

	floor := byke2d.NewTransform().WithTranslationXYZ(0, -0.5, 0)
	world.Spawn([]byke.ErasedComponent{
		floor,
		byke2d.GlobalTransform{Affine: floor.Affine3()},
		RigidBody{Type: Static},
		CuboidCollider(glm.Vec3f{10, 0.5, 10}),
	})

	box := byke2d.NewTransform().WithTranslationXYZ(0, 5, 0)
	boxId := world.Spawn([]byke.ErasedComponent{
		box,
		byke2d.GlobalTransform{Affine: box.Affine3()},
		RigidBody{Type: Dynamic},
		CuboidCollider(glm.Vec3f{0.5, 0.5, 0.5}),
	})

	query := world.Query[struct{ Overlaps SensorOverlaps }]()

	step := func(n int) {
		for range n {
			world.RunSchedule(byke.FixedPreUpdate)
			world.RunSchedule(byke.FixedUpdate)
			world.RunSchedule(byke.FixedPostUpdate)
		}
	}

	step(2)
	if item, _ := query.Get(sensorId); len(item.Overlaps.Entities) != 0 {
		t.Fatalf("box is still above the sensor, got %v", item.Overlaps.Entities)
	}

	step(2 * 64)
	item, _ := query.Get(sensorId)
	if !slices.Equal(item.Overlaps.Entities, []byke.EntityId{boxId}) || math.Abs(float64(item.Overlaps.Mass-1000)) > 1 {
		t.Fatalf("expected the 1000 kg box in the sensor, got %+v", item.Overlaps)
	}
}

func TestCylinderAndHullColliders(t *testing.T) {
	app := newTestApp(t)

	world := app.World()

	floor := byke2d.NewTransform().WithTranslationXYZ(0, -0.5, 0)
	world.Spawn([]byke.ErasedComponent{
		floor,
		byke2d.GlobalTransform{Affine: floor.Affine3()},
		RigidBody{Type: Static},
		CuboidCollider(glm.Vec3f{10, 0.5, 10}),
	})

	// cylinder with its origin at the bottom, like the props in the scene
	cylinder := byke2d.NewTransform().WithTranslationXYZ(0, 1, 0)
	cylinderId := world.Spawn([]byke.ErasedComponent{
		cylinder,
		byke2d.GlobalTransform{Affine: cylinder.Affine3()},
		RigidBody{Type: Dynamic},
		CylinderCollider(0.3, 0.8).WithOffset(glm.Vec3f{0, 0.4, 0}),
	})

	// more points on a sphere than a hull can have vertices
	var points []glm.Vec3f
	for idx := range 300 {
		y := 1 - 2*(float32(idx)+0.5)/300
		r := float32(math.Sqrt(float64(1 - y*y)))
		angle := float64(idx) * 2.39996
		points = append(points, glm.Vec3f{r * float32(math.Cos(angle)), y, r * float32(math.Sin(angle))}.Scale(0.5))
	}

	ball := byke2d.NewTransform().WithTranslationXYZ(5, 2, 0)
	ballId := world.Spawn([]byke.ErasedComponent{
		ball,
		byke2d.GlobalTransform{Affine: ball.Affine3()},
		RigidBody{Type: Dynamic},
		HullCollider(points),
	})

	for range 3 * 64 {
		world.RunSchedule(byke.FixedPreUpdate)
		world.RunSchedule(byke.FixedUpdate)
		world.RunSchedule(byke.FixedPostUpdate)
	}

	query := world.Query[struct{ Transform byke2d.Transform }]()

	item, _ := query.Get(cylinderId)
	if y := item.Transform.Translation[1]; math.Abs(float64(y)) > 0.02 {
		t.Fatalf("cylinder bottom should rest on the floor, is at y=%v", y)
	}

	// round: a diagonal ray hits at the radius, not at the corner of a box
	physicsWorld, _ := world.ResourceOf[PhysicsWorld]()
	diagonal := glm.Vec3f{1, 0, 1}.Normalize()
	hit, ok := physicsWorld.CastRay(diagonal.Scale(2).Add(glm.Vec3f{0, 0.4, 0}), diagonal.Scale(-1), 2)
	if !ok || hit.Entity != cylinderId || math.Abs(float64(hit.Distance-1.7)) > 0.02 {
		t.Fatalf("expected to hit the cylinder side at distance 1.7, got %+v (ok=%v)", hit, ok)
	}

	// the hull has at most maxHullVertices vertices, its faces are a bit inside the sphere
	item, _ = query.Get(ballId)
	if y := item.Transform.Translation[1]; math.Abs(float64(y-0.5)) > 0.06 {
		t.Fatalf("hull ball should rest at y=0.5, is at y=%v", y)
	}
}

func TestHullMeshFacesOutward(t *testing.T) {
	box := b3.MakeBoxHull(1, 2, 3)
	mesh := hullMesh(&box.Base)

	vertices, indices := mesh.Vertices(), mesh.Indices()
	if len(indices) != 6*2*3 {
		t.Fatalf("a box has 12 triangles, got %d", len(indices)/3)
	}

	for i := 0; i < len(indices); i += 3 {
		a, b, c := vertices[indices[i]], vertices[indices[i+1]], vertices[indices[i+2]]
		center := a.Add(b).Add(c).Scale(1.0 / 3)
		if b.Sub(a).Cross(c.Sub(a)).Dot(center) <= 0 {
			t.Fatalf("triangle %d faces inwards", i/3)
		}
	}
}

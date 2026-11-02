package physics

import (
	"testing"

	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestHitsAndExplode(t *testing.T) {
	app := newTestApp(t)

	world := app.World()

	ground := byke2d.NewTransform()
	world.Spawn([]byke.ErasedComponent{
		ground,
		byke2d.GlobalTransform{Affine: ground.Affine3()},
		RigidBody{Type: Static},
		CuboidCollider(glm.V3f(20, 0.5, 20)).WithOffset(glm.V3f(0, -0.5, 0)),
	})

	// a 0.5 m crate of 125 kg, 5 m from the explosion
	box := byke2d.NewTransform().WithTranslationXYZ(5, 2, 0)
	boxId := world.Spawn([]byke.ErasedComponent{
		box,
		byke2d.GlobalTransform{Affine: box.Affine3()},
		RigidBody{Type: Dynamic},
		CuboidCollider(glm.V3f(0.25, 0.25, 0.25)),
	})

	messages, _ := world.ResourceOf[byke.Messages[Hit]]()
	hits := messages.Reader()

	step := func(n int) {
		for range n {
			world.RunSchedule(byke.FixedPreUpdate)
			world.RunSchedule(byke.FixedUpdate)
			world.RunSchedule(byke.FixedPostUpdate)
		}
	}

	step(64)

	landed := hits.Read()
	if len(landed) == 0 || landed[0].Speed < 4 || landed[0].Point.Y > 0.1 {
		t.Fatalf("expected a hit at the ground after a 1.75 m fall, got %+v", landed)
	}

	physicsWorld, _ := world.ResourceOf[PhysicsWorld]()
	physicsWorld.Explode(glm.Vec3f{}, 6, 6, 1500)
	step(1)

	query := world.Query[struct{ LinearVelocity LinearVelocity }]()
	item, _ := query.Get(boxId)

	if v := item.LinearVelocity.Vec3f; v.X < 1 || v.X > 20 {
		t.Fatalf("explosion should push the box away along +x, velocity is %v", v)
	}

	t.Logf("velocity after explosion: %v", item.LinearVelocity.Vec3f)
}

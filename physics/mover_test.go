package physics

import (
	"testing"

	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestMoverWalksUpStepAndJumps(t *testing.T) {
	app := newTestApp(t)
	world := app.World()

	spawnBox := func(center, half glm.Vec3f) {
		transform := byke2d.NewTransform().WithTranslationXYZ(center.XYZ())
		world.Spawn([]byke.ErasedComponent{
			transform,
			byke2d.GlobalTransform{Affine: transform.Affine3()},
			RigidBody{Type: Static},
			CuboidCollider(half),
		})
	}

	// the floor and a 0.3 m step at x=3
	spawnBox(glm.V3f(0, -0.5, 0), glm.V3f(50, 0.5, 50))
	spawnBox(glm.V3f(8, 0.15, 0), glm.V3f(5, 0.15, 5))

	settings := DefaultMoverSettings()
	settings.PogoRestLength = 0.65

	start := byke2d.NewTransform().WithTranslationXYZ(0, 1, 0)
	moverId := world.Spawn([]byke.ErasedComponent{
		start,
		byke2d.GlobalTransform{Affine: start.Affine3()},
		RigidBody{Type: Dynamic, LockRotation: true, Density: 200},
		CapsuleCollider(0.35, 0.5),
		Mover{MoverSettings: settings},
	})

	query := world.Query[struct {
		Transform byke2d.Transform
		Mover     *Mover
	}]()

	step := func(n int) {
		for range n {
			world.RunSchedule(byke.FixedPreUpdate)
			world.RunSchedule(byke.FixedUpdate)
			world.RunSchedule(byke.FixedPostUpdate)
		}
	}

	// settle: the pogo holds the capsule center at rest length plus half the capsule length
	step(64)

	item, _ := query.Get(moverId)
	if y := item.Transform.Translation.Y; abs(y-0.9) > 0.05 || !item.Mover.OnGround {
		t.Fatalf("mover should hover at y=0.9 on the ground, is at y=%v, on ground %v", y, item.Mover.OnGround)
	}

	// walk 2 s along +x and onto the step
	item.Mover.Direction = glm.V3f(1, 0, 0)
	step(128)

	item, _ = query.Get(moverId)
	if pos := item.Transform.Translation; pos.X < 8 || abs(pos.Y-1.2) > 0.1 {
		t.Fatalf("mover should have walked onto the step at y=1.2, is at %v", pos)
	}

	item.Mover.Direction = glm.Vec3f{}
	item.Mover.Jump = true

	var top float32
	for range 64 {
		step(1)
		item, _ = query.Get(moverId)
		top = max(top, item.Transform.Translation.Y)
	}

	// 7 m/s at 1.5 g reaches about 1.6 m
	if top < 2.4 {
		t.Fatalf("mover should have jumped, highest point y=%v", top)
	}

	if item.Mover.Jump {
		t.Fatalf("jump should be reset after the step")
	}
}

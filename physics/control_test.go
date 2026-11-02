package physics

import (
	"testing"

	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestControlledCharacter(t *testing.T) {
	app := newTestApp(t)
	world := app.World()

	ground := byke2d.NewTransform()
	world.Spawn([]byke.ErasedComponent{
		ground,
		byke2d.GlobalTransform{Affine: ground.Affine3()},
		RigidBody{Type: Static},
		CollisionLayers{Category: 1, Mask: ^uint64(0)},
		CuboidCollider(glm.V3f(50, 0.5, 50)).WithOffset(glm.V3f(0, -0.5, 0)),
	})

	// an upright capsule that walks along +x
	character := byke2d.NewTransform().WithTranslationXYZ(0, 1, 0)
	characterId := world.Spawn([]byke.ErasedComponent{
		character,
		byke2d.GlobalTransform{Affine: character.Affine3()},
		RigidBody{Type: Dynamic, LockRotation: true},
		ControlledVelocity{},
		CollisionLayers{Category: 2, Mask: 1},
		CapsuleCollider(0.3, 1),
	})

	// a ghost box that ignores the character, falling through the area the character walks
	ghost := byke2d.NewTransform().WithTranslationXYZ(3, 1, 0)
	world.Spawn([]byke.ErasedComponent{
		ghost,
		byke2d.GlobalTransform{Affine: ghost.Affine3()},
		RigidBody{Type: Dynamic},
		CollisionLayers{Category: 4, Mask: 1},
		CuboidCollider(glm.V3f(0.5, 0.5, 0.5)),
	})

	query := world.Query[struct {
		Transform      byke2d.Transform
		LinearVelocity *LinearVelocity
	}]()

	for range 128 {
		if item, ok := query.Get(characterId); ok {
			item.LinearVelocity.Vec3f = glm.V3f(3, item.LinearVelocity.Vec3f.Y, 0)
		}

		world.RunSchedule(byke.FixedPreUpdate)
		world.RunSchedule(byke.FixedUpdate)
		world.RunSchedule(byke.FixedPostUpdate)
	}

	item, _ := query.Get(characterId)

	// 2 s at 3 m/s, walking through the box, which does not collide with it
	if x := item.Transform.Translation.X; x < 5.5 || x > 6.5 {
		t.Fatalf("character should have walked to x=6, is at %v", item.Transform.Translation)
	}

	up := item.Transform.Rotation.Transform(glm.V3f(0, 1, 0))
	if !nearTol(up, glm.V3f(0, 1, 0), 1e-3) {
		t.Fatalf("character should stay upright, up is %v", up)
	}

	physicsWorld, _ := world.ResourceOf[PhysicsWorld]()
	if hit, ok := physicsWorld.CastRayMask(glm.V3f(3, 5, 0), glm.V3f(0, -1, 0), 10, 1); !ok || hit.Distance < 4.9 {
		t.Fatalf("masked ray should only hit the ground at distance 5, got %+v", hit)
	}
}

func TestTeleportAndDisable(t *testing.T) {
	app := newTestApp(t)
	world := app.World()

	start := byke2d.NewTransform().WithTranslationXYZ(0, 10, 0)
	bodyId := world.Spawn([]byke.ErasedComponent{
		start,
		byke2d.GlobalTransform{Affine: start.Affine3()},
		RigidBody{Type: Dynamic},
		CuboidCollider(glm.V3f(0.5, 0.5, 0.5)),
	})

	query := world.Query[byke2d.Transform]()

	step := func(n int) {
		for range n {
			world.RunSchedule(byke.FixedPreUpdate)
			world.RunSchedule(byke.FixedUpdate)
			world.RunSchedule(byke.FixedPostUpdate)
		}
	}

	step(2)

	world.RunSystem(func(commands *byke.Commands) {
		commands.Entity(bodyId).Insert(Disabled{}, Teleport{Position: glm.V3f(5, 20, 0)})
	})
	step(30)

	// disabled, so it does not fall
	if pos, _ := query.Get(bodyId); !nearTol(pos.Translation, glm.V3f(5, 20, 0), 1e-3) {
		t.Fatalf("disabled body should stay at the teleport target, is at %v", pos.Translation)
	}

	world.RunSystem(func(commands *byke.Commands) {
		commands.Entity(bodyId).Remove[Disabled]()
	})
	step(30)

	if pos, _ := query.Get(bodyId); pos.Translation.Y > 19 || abs(pos.Translation.X-5) > 1e-3 {
		t.Fatalf("enabled body should fall from the teleport target, is at %v", pos.Translation)
	}
}

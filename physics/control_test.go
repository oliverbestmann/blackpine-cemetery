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
		CuboidCollider(glm.Vec3f{50, 0.5, 50}).WithOffset(glm.Vec3f{0, -0.5, 0}),
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
		CuboidCollider(glm.Vec3f{0.5, 0.5, 0.5}),
	})

	query := world.Query[struct {
		Transform      byke2d.Transform
		LinearVelocity *LinearVelocity
	}]()

	for range 128 {
		if item, ok := query.Get(characterId); ok {
			item.LinearVelocity.Vec3f = glm.Vec3f{3, item.LinearVelocity.Vec3f[1], 0}
		}

		world.RunSchedule(byke.FixedPreUpdate)
		world.RunSchedule(byke.FixedUpdate)
		world.RunSchedule(byke.FixedPostUpdate)
	}

	item, _ := query.Get(characterId)

	// 2 s at 3 m/s, walking through the box, which does not collide with it
	if x := item.Transform.Translation[0]; x < 5.5 || x > 6.5 {
		t.Fatalf("character should have walked to x=6, is at %v", item.Transform.Translation)
	}

	up := item.Transform.Rotation.Transform(glm.Vec3f{0, 1, 0})
	if !nearTol(up, glm.Vec3f{0, 1, 0}, 1e-3) {
		t.Fatalf("character should stay upright, up is %v", up)
	}

	physicsWorld, _ := world.ResourceOf[PhysicsWorld]()
	if hit, ok := physicsWorld.CastRayMask(glm.Vec3f{3, 5, 0}, glm.Vec3f{0, -1, 0}, 10, 1); !ok || hit.Distance < 4.9 {
		t.Fatalf("masked ray should only hit the ground at distance 5, got %+v", hit)
	}
}

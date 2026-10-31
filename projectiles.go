package main

import (
	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

const (
	grenadeSpeed = 22
	grenadeFuse  = 1.6
	grenadeScale = 1.6

	rocketSpeed = 45
	rocketLife  = 6
)

// Grenade bounces around until its fuse runs out or it touches a zombie.
type Grenade struct {
	Component[Grenade]
	Fuse float32
}

// Rocket flies straight and explodes on the first thing it hits.
type Rocket struct {
	Component[Rocket]
	Velocity glm.Vec3f
	Age      float32
	smoke    float32
}

func projectilesPlugin(app *App) {
	app.AddSystems(Update, System(grenadeSystem, rocketSystem).RunIf(InState(GameStatePlaying)))
}

func spawnGrenade(commands *Commands, models *Models, muzzle, dir glm.Vec3f) {
	model := models.Get("blaster/grenade-a")
	center := model.Min.Add(model.Max).Scale(0.5)

	// lob it a bit upwards
	velocity := dir.Add(glm.Vec3f{0, 0.15, 0}).Normalize().Scale(grenadeSpeed)

	commands.Spawn(
		Round{},
		Grenade{Fuse: grenadeFuse},
		TransformFromXYZ(muzzle.XYZ()).WithScaleXYZ(grenadeScale, grenadeScale, grenadeScale),
		MeshInstance{Mesh: model.Merged},
		model.Material,
		physics.RigidBody{Type: physics.Dynamic, Density: 2000, Restitution: 0.45, Friction: 0.6, Bullet: true},
		physics.CollisionLayers{Category: layerProjectile, Mask: layerWorld | layerProp | layerCar},
		physics.SphereCollider(model.Size()[1]*0.5).WithOffset(center),
		physics.LinearVelocity{Vec3f: velocity},
		physics.AngularVelocity{Vec3f: glm.Vec3f{8, 2, 0}},
	)
}

func spawnRocket(commands *Commands, assets *GameAssets, muzzle, dir glm.Vec3f) {
	rotation := glm.QuatFromRotationArc(glm.Vec3f{0, 0, 1}, dir)

	rocket := commands.Spawn(
		Round{},
		Rocket{Velocity: dir.Scale(rocketSpeed)},
		NewTransform().WithTranslationXYZ(muzzle.XYZ()).WithRotation(rotation),
		Visibility{},
	).Id()

	commands.Spawn(
		ChildOf{Parent: rocket},
		NewTransform().WithScaleXYZ(0.14, 0.14, 0.6),
		MeshInstance{Mesh: assets.Cube},
		assets.RocketMaterial,
	)

	// the glowing exhaust
	commands.Spawn(
		ChildOf{Parent: rocket},
		NewTransform().WithTranslationXYZ(0, 0, -0.35).WithScaleXYZ(0.16, 0.16, 0.16),
		MeshInstance{Mesh: assets.Sphere},
		assets.FireMaterial,
		NotShadowCaster{},
	)

	commands.Spawn(
		ChildOf{Parent: rocket},
		TransformFromXYZ(0, 0, -0.6),
		PointLight{Color: ColorSRGB(1, 0.6, 0.25), Intensity: 150000, Range: 15},
	)
}

// grenadeSystem blows grenades up when the fuse runs out or a zombie is close.
func grenadeSystem(
	vt VirtualTime,
	commands *Commands,
	explosions *MessageWriter[Explosion],
	grenades Query[struct {
		EntityId  EntityId
		Grenade   *Grenade
		Transform Transform
	}],
	zombies Query[struct {
		Zombie    Zombie
		Transform Transform
	}],
) {
	for item := range grenades.Items() {
		g := item.Grenade
		if g.Fuse <= 0 {
			continue
		}

		g.Fuse -= vt.DeltaSecs

		pos := item.Transform.Translation

		touching := false
		if g.Fuse < grenadeFuse-0.1 {
			for z := range zombies.Items() {
				if z.Zombie.Rising <= 0 && flat(z.Transform.Translation.Sub(pos)).Length() < zombieRadius*z.Zombie.Scale/2.1+0.4 {
					touching = true
					break
				}
			}
		}

		if g.Fuse > 0 && !touching {
			continue
		}

		g.Fuse = 0
		explosions.Write(Explosion{Position: pos, Radius: 8, Damage: 160})
		commands.Entity(item.EntityId).Despawn()
	}
}

// rocketSystem moves rockets with a ray cast per frame, they explode on the first hit.
func rocketSystem(
	vt VirtualTime,
	commands *Commands,
	world physics.PhysicsWorld,
	rng *GameRng,
	assets *GameAssets,
	explosions *MessageWriter[Explosion],
	rockets Query[struct {
		EntityId  EntityId
		Rocket    *Rocket
		Transform *Transform
	}],
) {
	dt := vt.DeltaSecs

	for item := range rockets.Items() {
		r := item.Rocket
		if r.Age < 0 {
			// already exploded
			continue
		}

		r.Age += dt

		pos := item.Transform.Translation
		step := r.Velocity.Scale(dt)
		distance := step.Length()

		hit, ok := world.CastRayMask(pos, step.Scale(1/distance), distance+0.3, hitscanMask)
		if ok || r.Age > rocketLife || pos[1] < 0 {
			if ok {
				pos = hit.Point.Sub(step.Scale(0.3 / distance))
			}

			r.Age = -1
			explosions.Write(Explosion{Position: pos, Radius: 10, Damage: 220})
			commands.Entity(item.EntityId).Despawn()
			continue
		}

		item.Transform.Translation = pos.Add(step)

		// a smoke trail
		r.smoke -= dt
		if r.smoke <= 0 {
			r.smoke = 0.025
			s := 0.15 + rng.Float32()*0.1
			spawnParticle(commands, assets.Sphere, assets.SmokeMaterial, pos, Particle{
				Velocity:   randomInSphere(rng.Rand).Scale(0.6).Add(glm.Vec3f{0, 0.5, 0}),
				Drag:       1,
				Life:       0.9 + rng.Float32()*0.4,
				StartScale: glm.Vec3f{s, s, s},
				EndScale:   glm.Vec3f{s, s, s}.Scale(0.2),
			})
		}
	}
}

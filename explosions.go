package main

import (
	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

// Explosion blows things up at Position. Damage fades out linearly up to Radius.
type Explosion struct {
	Position glm.Vec3f
	Radius   float32
	Damage   float32
}

// Explosive props blow up when their health runs out.
type Explosive struct {
	Component[Explosive]
	Health float32

	// Fuse counts down after another explosion lit it, it explodes at zero.
	Fuse float32
	Lit  bool
}

const barrelHealth = 30

// FadingLight is a point light that fades out over its lifetime and despawns.
type FadingLight struct {
	Component[FadingLight]
	Intensity float32
	Age, Life float32
}

func explosionsPlugin(app *App) {
	app.AddMessage[Explosion]()
	app.AddMessage[PropHit]()

	app.AddSystems(Update, System(explosiveSystem, explosionSystem).Chain().RunIf(NotInState(GameStateLoading)))
	app.AddSystems(Update, fadingLightSystem)
}

// explosiveDamage returns the damage at distance from an explosion.
func explosiveDamage(damage, radius, distance float32) float32 {
	if distance >= radius {
		return 0
	}

	return damage * (1 - distance/radius)
}

// explosiveSystem damages explosive props on hits and blows them up.
func explosiveSystem(
	vt VirtualTime,
	commands *Commands,
	hits *MessageReader[PropHit],
	explosions *MessageWriter[Explosion],
	explosives Query[struct {
		EntityId  EntityId
		Explosive *Explosive
		Transform Transform
	}],
) {
	for _, hit := range hits.Read() {
		if item, ok := explosives.Get(hit.Entity); ok {
			item.Explosive.Health -= hit.Damage
		}
	}

	for item := range explosives.Items() {
		e := item.Explosive

		if e.Lit {
			e.Fuse -= vt.DeltaSecs
		}

		if e.Health > 0 && (!e.Lit || e.Fuse > 0) {
			continue
		}

		// mark it as done, it despawns at the end of the frame
		e.Health = 1e9
		e.Lit = false

		explosions.Write(Explosion{Position: item.Transform.Translation, Radius: 9, Damage: 150})
		commands.Entity(item.EntityId).Despawn()
	}
}

func explosionSystem(
	commands *Commands,
	world physics.PhysicsWorld,
	rng *GameRng,
	budget *SoundBudget,
	assets *GameAssets,
	reader *MessageReader[Explosion],
	damages *MessageWriter[ZombieDamage],
	zombies Query[struct {
		EntityId  EntityId
		_         With[Zombie]
		Transform Transform
	}],
	players Query[struct {
		Player    *Player
		Transform Transform
	}],
	cameras Query[*PlayerCamera],
	explosives Query[struct {
		Explosive *Explosive
		Transform Transform
	}],
	trees Query[struct {
		EntityId  EntityId
		Tree      Tree
		Transform Transform
	}],
) {
	for _, ex := range reader.Read() {
		world.Explode(ex.Position, ex.Radius*0.6, ex.Radius*0.6, 120)

		for z := range zombies.Items() {
			// aim at the center of the body, a bit above the ground
			offset := z.Transform.Translation.Sub(ex.Position)
			distance := offset.Length()

			amount := explosiveDamage(ex.Damage, ex.Radius, distance)
			if amount <= 0 {
				continue
			}

			dir := offset.Scale(1 / max(distance, 0.1)).Add(glm.Vec3f{0, 0.8, 0}).Normalize()

			damages.Write(ZombieDamage{
				Zombie:    z.EntityId,
				Amount:    amount,
				Push:      dir.Scale(5 + 10*amount/ex.Damage),
				Point:     z.Transform.Translation,
				Explosive: true,
				Source:    SourceExplosion,
			})
		}

		for item := range explosives.Items() {
			distance := item.Transform.Translation.Sub(ex.Position).Length()
			if distance < ex.Radius && !item.Explosive.Lit && item.Explosive.Health < 1e8 {
				// chain reactions go off one after another
				item.Explosive.Lit = true
				item.Explosive.Fuse = 0.15 + rng.Float32()*0.25
			}
		}

		for tree := range trees.Items() {
			offset := flat(tree.Transform.Translation.Sub(ex.Position))
			distance := offset.Length()
			if distance >= ex.Radius*0.6 {
				continue
			}

			// the static trimesh tree becomes a dynamic one with hulls that tips away from the blast.
			// the new body exists only after the next step, so it gets its push as start velocity.
			away := offset.Scale(1 / max(distance, 0.1))
			strength := 1 - distance/(ex.Radius*0.6)

			commands.Entity(tree.EntityId).Despawn()
			spawnProp(commands, tree.Tree.Model, tree.Transform, PropSettings{Dynamic: true, Density: 300, Slices: 4},
				physics.LinearVelocity{Vec3f: away.Scale(3 * strength)},
				physics.AngularVelocity{Vec3f: glm.Vec3f{0, 1, 0}.Cross(away).Scale(0.5 + 1.5*strength)},
			)
		}

		for player := range players.Items() {
			distance := player.Transform.Translation.Sub(ex.Position).Length()

			// own explosions hurt, but not too much
			if amount := explosiveDamage(ex.Damage*0.3, ex.Radius*0.7, distance); amount > 0 && player.Player.Driving == NoEntityId {
				player.Player.Health -= amount
				player.Player.Damaged = 0.4
			}

			for camera := range cameras.Items() {
				camera.Shake = max(camera.Shake, clamp(1.5-distance/30, 0, 1.2))
			}
		}

		spawnExplosionEffect(commands, assets, rng, ex.Position, ex.Radius)

		sound := assets.Sounds.Explosion
		if rng.IntN(2) == 0 {
			sound = assets.Sounds.ExplosionLow
		}

		playAt(commands, budget, sound, ex.Position, 1.5)
	}
}

// spawnExplosionEffect spawns the fireball, smoke, sparks and a bright flash of light.
func spawnExplosionEffect(commands *Commands, assets *GameAssets, rng *GameRng, pos glm.Vec3f, radius float32) {
	size := radius / 9

	commands.Spawn(
		Round{},
		TransformFromXYZ(pos.Add(glm.Vec3f{0, 1, 0}).XYZ()),
		PointLight{Color: ColorSRGB(1, 0.55, 0.2), Intensity: 0, Range: 40},
		FadingLight{Intensity: 4e6 * size, Life: 0.6},
	)

	// the fireball: a few glowing balls that grow and shrink
	for range 6 {
		s := (1.5 + rng.Float32()*1.5) * size
		spawnParticle(commands, assets.Sphere, assets.FireMaterial, pos.Add(randomInSphere(rng.Rand).Scale(size)), Particle{
			Velocity:   randomInSphere(rng.Rand).Scale(3).Add(glm.Vec3f{0, 3, 0}),
			Drag:       3,
			Life:       0.25 + rng.Float32()*0.2,
			StartScale: glm.Vec3f{s, s, s}.Scale(0.5),
			EndScale:   glm.Vec3f{s, s, s}.Scale(1.4),
		})
	}

	// smoke rises slowly
	for range 5 {
		s := (0.5 + 0.6*rng.Float32()) * size
		spawnParticle(commands, assets.Sphere, assets.SmokeMaterial, pos.Add(randomInSphere(rng.Rand).Scale(size*1.5)), Particle{
			Velocity:   randomInSphere(rng.Rand).Scale(2).Add(glm.Vec3f{0, 3, 0}),
			Gravity:    -1,
			Drag:       1.5,
			Life:       1 + rng.Float32()*0.6,
			StartScale: glm.Vec3f{s, s, s},
			EndScale:   glm.Vec3f{s, s, s}.Scale(0.1),
		})
	}

	// sparks and burning debris
	for range 24 {
		spawnParticle(commands, assets.Cube, assets.FireMaterial, pos.Add(glm.Vec3f{0, 0.3, 0}), Particle{
			Velocity:   randomInSphere(rng.Rand).Scale(18).Add(glm.Vec3f{0, 8, 0}),
			Gravity:    15,
			Drag:       0.5,
			Life:       0.6 + rng.Float32()*0.6,
			StartScale: glm.Vec3f{0.12, 0.12, 0.12},
			EndScale:   glm.Vec3f{0.03, 0.03, 0.03},
			Bounce:     true,
		})
	}
}

func fadingLightSystem(vt VirtualTime, commands *Commands, lights Query[struct {
	EntityId EntityId
	Fade     *FadingLight
	Light    *PointLight
}],
) {
	for item := range lights.Items() {
		item.Fade.Age += vt.DeltaSecs
		t := item.Fade.Age / item.Fade.Life
		if t >= 1 {
			commands.Entity(item.EntityId).Despawn()
			continue
		}

		item.Light.Intensity = item.Fade.Intensity * (1 - t) * (1 - t)
	}
}

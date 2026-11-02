package main

import (
	"math/rand/v2"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

const (
	// gibs lie around this long, shorter when there are more than maxGibs
	gibLifetime = 15
	maxGibs     = 300

	// blood splats stay this long, shorter when there are more than maxBlood
	bloodLifetime = 25
	maxBlood      = 800
)

// Gib is a body part flying around after a zombie died.
type Gib struct {
	Component[Gib]
	Age float32
}

// Blood is a drop of blood. It flies until it hits the ground and stays there as a splat.
type Blood struct {
	Component[Blood]
	Velocity glm.Vec3f
	Age      float32
	Landed   bool
	Size     float32
}

func gibsPlugin(app *App) {
	app.AddSystems(Update, gibAgeSystem, bloodSystem)
}

// spawnGibs tears a model apart: every part becomes a physics body that flies away.
// transform places the model in the world, velocity is the common velocity and spread
// the random velocity of each part in m/s.
func spawnGibs(commands *Commands, rng *GameRng, model *Model, transform Transform, velocity glm.Vec3f, spread float32) {
	root := transform.Affine3()

	for _, part := range model.Parts {
		if part.Mesh == nil {
			continue
		}

		world := root.Mul(part.Model)
		translation, rotation, scale := decomposeAffine(world)

		half := part.Max.Sub(part.Min).Scale(0.5)
		center := part.Min.Add(part.Max).Scale(0.5)

		v := velocity.Add(randomInSphere(rng.Rand).Scale(spread)).Add(glm.V3f(0, spread*0.6, 0))
		spin := randomInSphere(rng.Rand).Scale(8 + spread)

		commands.Spawn(
			Round{},
			Gib{},
			Transform{Translation: translation, Rotation: rotation, Scale: scale},
			MeshInstance{Mesh: part.Mesh},
			model.Material,
			physics.RigidBody{Type: physics.Dynamic, Density: 300, LinearDamping: 0.1},
			physics.CollisionLayers{Category: layerGib, Mask: layerWorld | layerProp | layerGib | layerCar},
			physics.CuboidCollider(half).WithOffset(center),
			physics.LinearVelocity{Vec3f: v},
			physics.AngularVelocity{Vec3f: spin},
		)
	}
}

// decomposeAffine splits a matrix without shear into translation, rotation and scale.
func decomposeAffine(m glm.Mat4f) (glm.Vec3f, glm.Quat, glm.Vec3f) {
	var rot glm.Mat3f
	var lengths [3]float32
	for col := range 3 {
		axis := glm.V3f(m[col][0], m[col][1], m[col][2])
		lengths[col] = axis.Length()
		rot[col] = axis.Scale(1 / lengths[col]).Array()
	}

	return m.Translation(), glm.QuatFromMat3(rot), glm.V3f(lengths[0], lengths[1], lengths[2])
}

// lifetimeFor shortens a lifetime when there are more than max things alive.
func lifetimeFor(lifetime float32, count, maxCount int) float32 {
	if count <= maxCount {
		return lifetime
	}

	return lifetime * float32(maxCount) / float32(count)
}

func gibAgeSystem(vt VirtualTime, commands *Commands, gibs Query[struct {
	EntityId EntityId
	Gib      *Gib
}],
) {
	lifetime := lifetimeFor(gibLifetime, gibs.Count(), maxGibs)

	for item := range gibs.Items() {
		item.Gib.Age += vt.DeltaSecs
		if item.Gib.Age > lifetime {
			commands.Entity(item.EntityId).Despawn()
		}
	}
}

// spawnBlood sprays count drops of blood from a point.
func spawnBlood(commands *Commands, assets *GameAssets, rng *GameRng, point, velocity glm.Vec3f, count int) {
	for range count {
		size := 0.06 + rng.Float32()*0.1
		v := velocity.Add(randomInSphere(rng.Rand).Scale(4)).Add(glm.V3f(0, 2, 0))

		commands.Spawn(
			Round{},
			Blood{Velocity: v, Size: size},
			TransformFromXYZ(point.XYZ()).WithScaleXYZ(size, size, size),
			MeshInstance{Mesh: assets.Cube},
			assets.BloodMaterial,
			NotShadowCaster{},
		)
	}
}

// bloodSystem moves the drops and turns them into splats on the ground.
func bloodSystem(vt VirtualTime, commands *Commands, drops Query[struct {
	EntityId  EntityId
	Blood     *Blood
	Transform *Transform
}],
) {
	dt := vt.DeltaSecs
	lifetime := lifetimeFor(bloodLifetime, drops.Count(), maxBlood)

	for item := range drops.Items() {
		b := item.Blood
		b.Age += dt

		if b.Age > lifetime {
			commands.Entity(item.EntityId).Despawn()
			continue
		}

		if b.Landed {
			// shrink away at the end of the lifetime
			if left := lifetime - b.Age; left < 2 {
				s := b.Size * 4 * left / 2
				item.Transform.Scale = glm.V3f(s, 0.01, s)
			}

			continue
		}

		b.Velocity.Y -= 9.81 * dt
		item.Transform.Translation = item.Transform.Translation.Add(b.Velocity.Scale(dt))

		if item.Transform.Translation.Y < 0.01 {
			// a flat splat on the ground, a bit above it to avoid z fighting
			b.Landed = true
			item.Transform.Translation.Y = 0.005 + rand.Float32()*0.01
			item.Transform.Scale = glm.V3f(b.Size*4, 0.01, b.Size*4)
			item.Transform.Rotation = glm.RotationYQuat(glm.Rad(rand.Float32() * 3))
		}
	}
}

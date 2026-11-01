package main

import (
	"math"
	"math/rand/v2"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// spawnDecor spawns a model without physics, for things nobody can reach or bump into.
func spawnDecor(commands *Commands, model *Model, transform Transform, extra ...ErasedComponent) {
	components := []ErasedComponent{Round{}, transform, MeshInstance{Mesh: model.Merged}, model.Material}
	commands.Spawn(append(components, extra...)...)
}

// spawnFlame spawns a small glowing ball, the Kenney flames are only texture colors.
// With a light intensity above zero it also lights its surroundings.
func spawnFlame(commands *Commands, assets *GameAssets, pos glm.Vec3f, size, intensity, lightRange float32) {
	components := []ErasedComponent{
		Round{},
		TransformFromXYZ(pos.XYZ()).WithScaleXYZ(size, size*1.4, size),
		MeshInstance{Mesh: assets.Sphere},
		glowing(6, 1.6, 0.12),
		NotShadowCaster{},
	}

	if intensity > 0 {
		components = append(components, PointLight{Color: ColorSRGB(1, 0.6, 0.25), Intensity: intensity, Range: lightRange})
	}

	commands.Spawn(components...)
}

// spawnGraveyardLights spawns fire baskets at the gate, lanterns at the crypts and
// lightposts along the path from the start.
func spawnGraveyardLights(commands *Commands, models *Models, assets *GameAssets) {
	s := float32(scaleGraveyard)
	front := graveyardCenter[2] + graveyardHalfDepth

	basket := models.Get("graveyard/fire-basket")
	for _, x := range []float32{-5.5, 5.5} {
		pos := glm.Vec3f{x, 0, front + 1.5}
		spawnProp(commands, basket, TransformFromXYZ(pos.XYZ()).WithScaleXYZ(s*1.5, s*1.5, s*1.5), PropSettings{})
		spawnFlame(commands, assets, pos.Add(glm.Vec3f{0, basket.Max[1] * s * 1.5, 0}), 0.45, 60000, 16)
	}

	lantern := models.Get("graveyard/lantern-candle")
	// at most 3-4 static point lights, every pixel loops over all of them.
	// only the lantern at the big crypt lights the ground.
	for _, x := range []float32{-18 + 3, 0 + 4, 18 + 3} {
		pos := glm.Vec3f{x, 0, graveyardCenter[2] - graveyardHalfDepth + 2}
		spawnProp(commands, lantern, TransformFromXYZ(pos.XYZ()).WithScaleXYZ(s, s, s), PropSettings{})

		intensity := float32(0)
		if x == 0+4 {
			intensity = 15000
		}

		spawnFlame(commands, assets, pos.Add(glm.Vec3f{0, lantern.Max[1] * s * 0.45, 0}), 0.12, intensity, 9)
	}

	post := models.Get("graveyard/lightpost-double")
	for _, z := range []float32{front + 8, front + 22} {
		for _, x := range []float32{-6, 6} {
			pos := glm.Vec3f{x, 0, z}
			ps := s * 1.6
			spawnProp(commands, post, TransformFromXYZ(pos.XYZ()).WithScaleXYZ(ps, ps, ps), PropSettings{})

			// glowing lamps only, no point light
			for _, arm := range []float32{post.Min[2] * 0.8, post.Max[2] * 0.8} {
				spawnFlame(commands, assets, pos.Add(glm.Vec3f{0, post.Max[1]*ps - 0.45, arm * ps}), 0.14, 0, 0)
			}
		}
	}
}

// spawnVegetation scatters grass tufts and grass patches over the arena, without physics.
func spawnVegetation(commands *Commands, models *Models, rng *rand.Rand) {
	patches := []*Model{models.Get("survival/patch-grass"), models.Get("survival/patch-grass-large")}
	tufts := []*Model{models.Get("survival/grass"), models.Get("survival/grass-large")}

	random := func() glm.Vec3f {
		return glm.Vec3f{(rng.Float32()*2 - 1) * (arenaHalfSize - 2), 0, (rng.Float32()*2 - 1) * (arenaHalfSize - 2)}
	}

	for range 80 {
		center := random()

		// slightly above the ground, the patches are flat
		s := scaleSurvival * (0.6 + rng.Float32()*0.5)
		transform := TransformFromXYZ(center[0], 0.02+rng.Float32()*0.01, center[2]).WithRotation(randomYaw(rng)).WithScaleXYZ(s, 1, s)
		spawnDecor(commands, patches[rng.IntN(len(patches))], transform, NotShadowCaster{})

		// tufts around the patch
		for range 1 + rng.IntN(4) {
			pos := center.Add(glm.Vec3f{(rng.Float32()*2 - 1) * s, 0, (rng.Float32()*2 - 1) * s})
			ts := scaleSurvival * (0.8 + rng.Float32()*0.6)
			spawnDecor(commands, tufts[rng.IntN(len(tufts))], TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(ts, ts, ts), NotShadowCaster{})
		}
	}

	// single tufts everywhere
	for range 250 {
		ts := scaleSurvival * (0.8 + rng.Float32()*0.6)
		spawnDecor(commands, tufts[rng.IntN(len(tufts))], TransformFromXYZ(random().XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(ts, ts, ts), NotShadowCaster{})
	}

	// flat rocks with grass, low enough to drive over, so no collider
	rock := models.Get("survival/rock-flat-grass")
	for range 15 {
		s := scaleSurvival * (0.6 + rng.Float32()*0.4)
		spawnDecor(commands, rock, TransformFromXYZ(randomArenaPosition(rng, 4).XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s*0.6, s))
	}
}

// spawnRoad lays a road from the graveyard gate to the south edge of the arena, with cones
// along the sides and two crashed cars on it.
func spawnRoad(commands *Commands, models *Models, rng *rand.Rand) {
	const (
		tile = 4.0
		// the model tile is 0.8 m wide
		size = tile * 0.8
	)

	road := models.Get("graveyard/road")
	front := graveyardCenter[2] + graveyardHalfDepth

	// flat, so nobody trips over the edge. render only, the ground below does the physics
	for z := float32(front + size/2); z < arenaHalfSize; z += size {
		for _, x := range []float32{-size / 2, size / 2} {
			spawnDecor(commands, road, TransformFromXYZ(x, 0.01, z).WithScaleXYZ(tile, 0.5, tile), NotShadowCaster{})
		}
	}

	cones := []*Model{models.Get("car/cone"), models.Get("car/cone-flat")}
	cone := func(x, z float32) {
		s := float32(scaleCar)
		spawnProp(commands, cones[rng.IntN(len(cones))], TransformFromXYZ(x, 0, z).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 200})
	}

	// rows of cones on both sides
	for z := float32(front + 4); z < arenaHalfSize-2; z += 6 + rng.Float32()*6 {
		cone(-size-0.6, z)
		cone(size+0.6, z)
	}

	// a crash scene: two cars and a ring of cones
	crash := glm.Vec3f{0, 0, 45}
	s := float32(scaleCar)
	spawnCar(commands, models, "car/sedan", TransformFromXYZ(crash[0]-1.8, 0, crash[2]).WithRotation(glm.RotationYQuat(glm.DegToRad(70))).WithScaleXYZ(s, s, s))
	spawnCar(commands, models, "car/taxi", TransformFromXYZ(crash[0]+2.2, 0, crash[2]+3).WithRotation(glm.RotationYQuat(glm.DegToRad(-160))).WithScaleXYZ(s, s, s))

	for i := range 10 {
		angle := float32(i) * 2 * math.Pi / 10
		cone(crash[0]+cos32(angle)*7, crash[2]+1.5+sin32(angle)*7)
	}
}

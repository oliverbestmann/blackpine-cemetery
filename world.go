package main

import (
	"math"
	"math/rand/v2"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

// collision categories
const (
	layerWorld uint64 = 1 << iota
	layerProp
	layerPlayer
	layerZombie
	layerGib
	layerProjectile
	layerCar
	layerFlyer
)

const layerAll = ^uint64(0)

// half the side length of the square arena, walls stand at this distance from the center
const arenaHalfSize = 90

// scale of the Kenney kits, so the characters are about 1.8 m tall
const (
	scaleGraveyard = 2.1
	scaleSurvival  = 3.0
	scaleCar       = 1.4
	scaleUrban     = 4.0
)

var (
	// towards the sun of the sky texture, about 8° above the horizon
	sunDirection = glm.Vec3f{0.777, 0.139, -0.614}.Normalize()

	// the shadow map covers this box around the player
	sunShadowVolume = glm.Vec3f{60, 60, 300}
)

// Sun is the directional light, its shadow volume follows the player.
type Sun struct {
	ImmutableComponent[Sun]
}

// beforeTransforms runs in PostUpdate before the transforms propagate.
var beforeTransforms = (&SystemSet{Name: "BeforeTransforms"}).Before(TransformSystems)

func worldPlugin(app *App) {
	app.AddSystems(PostUpdate, System(followSunSystem).InSet(beforeTransforms))
}

func spawnWorldSystem(commands *Commands, models *Models, assets *GameAssets) {
	rng := rand.New(rand.NewPCG(7, 13))

	spawnGround(commands, assets)
	spawnSun(commands)
	spawnArenaWalls(commands, models)
	spawnGraveyard(commands, models, rng)
	spawnScenery(commands, models, rng)
}

func spawnGround(commands *Commands, assets *GameAssets) {
	const size = 2 * (arenaHalfSize + 60)

	// the texture repeats every 6 m
	mesh := PlaneWithSize(glm.Vec2f{size, size})
	mesh.ComputeUV(func(p glm.Vec3f) glm.Vec2f { return glm.Vec2f{p[0] / 6, p[2] / 6} })
	mesh.ComputeTangents()

	commands.Spawn(
		Round{},
		NewTransform(),
		MeshInstance{Mesh: mesh},
		StandardMaterial{
			BaseColor:                 ColorSRGB(0.85, 0.75, 0.7),
			PerceptualRoughness:       0.95,
			StandardMaterialBindGroup: StandardMaterialBindGroup{Texture: assets.Ground},
		},
		physics.RigidBody{Type: physics.Static},
		physics.CollisionLayers{Category: layerWorld, Mask: layerAll},
		physics.CuboidCollider(glm.Vec3f{size / 2, 1, size / 2}).WithOffset(glm.Vec3f{0, -1, 0}),
		physics.NoDebugCollider{},
	)
}

func spawnSun(commands *Commands) {
	light := DefaultDirectionalLight
	light.Color = ColorSRGB(1, 0.75, 0.55)
	light.Illuminance = 2500
	light.ShadowsEnabled = true
	light.ShadowVolume = sunShadowVolume
	light.EnvironmentShadow = 0.6

	commands.Spawn(
		Round{},
		Sun{},
		light,
		// lights shine along their local -z
		NewTransform().WithRotation(glm.QuatFromRotationArc(glm.Vec3f{0, 0, -1}, sunDirection.Scale(-1))),
	)
}

// followSunSystem keeps the shadow volume centered on the camera.
func followSunSystem(
	cameras Query[struct {
		_         With[PlayerCamera]
		Transform Transform
	}],
	suns Query[struct {
		_         With[Sun]
		Transform *Transform
	}],
) {
	camera, ok := cameras.Single()
	if !ok {
		return
	}

	center := camera.Transform.Translation

	// snap to a grid of shadow texels, so the shadows do not shimmer while moving
	const step = 0.5
	center = glm.Vec3f{
		float32(math.Round(float64(center[0]/step))) * step,
		0,
		float32(math.Round(float64(center[2]/step))) * step,
	}

	for sun := range suns.Items() {
		// the volume starts at the light's position and reaches along the light's direction
		sun.Transform.Translation = center.Add(sunDirection.Scale(sunShadowVolume[2] / 2))
	}
}

// PropSettings describes how a prop takes part in the physics.
type PropSettings struct {
	Dynamic bool

	// Density in kg/m³ of the collider, zero for the default of 1000
	Density float32

	// Shrink the box collider by this factor, e.g. for round things like trees
	ColliderScale glm.Vec3f

	// HullCollider uses the convex hull of the model instead of its bounding box.
	HullCollider bool
}

// spawnProp spawns a model as a single entity with a box collider around it.
func spawnProp(commands *Commands, model *Model, transform Transform, settings PropSettings, extra ...ErasedComponent) EntityId {
	half := model.Size().Scale(0.5)
	if settings.ColliderScale != (glm.Vec3f{}) {
		half = half.Mul(settings.ColliderScale)
	}

	center := model.Min.Add(model.Max).Scale(0.5)

	collider := physics.CuboidCollider(half).WithOffset(center)
	if settings.HullCollider {
		collider = physics.HullCollider(model.Merged.Vertices())
	}

	body := physics.RigidBody{Type: physics.Static, Density: settings.Density}
	layers := physics.CollisionLayers{Category: layerWorld, Mask: layerAll}

	if settings.Dynamic {
		body.Type = physics.Dynamic
		layers.Category = layerProp
	}

	components := []ErasedComponent{
		Round{},
		transform,
		MeshInstance{Mesh: model.Merged},
		model.Material,
		body,
		layers,
		collider,
	}

	return commands.Spawn(append(components, extra...)...).Id()
}

// arenaWalls are brick walls around the arena. They are static, nothing leaves the arena.
func spawnArenaWalls(commands *Commands, models *Models) {
	wall := models.Get("graveyard/brick-wall")

	const wallScale = 4.0
	length := wall.Size()[0] * wallScale

	for side := range 4 {
		rotation := glm.RotationYQuat(glm.Rad(float64(side) * math.Pi / 2))

		for offset := -arenaHalfSize + length/2; offset < arenaHalfSize; offset += length {
			position := rotation.Transform(glm.Vec3f{offset, 0, -arenaHalfSize})
			transform := TransformFromXYZ(position.XYZ()).WithRotation(rotation).WithScaleXYZ(wallScale, wallScale, wallScale)
			spawnProp(commands, wall, transform, PropSettings{})
		}
	}
}

// graveyardCenter is the center of the graveyard, the dead rise there.
var graveyardCenter = glm.Vec3f{0, 0, -45}

const (
	graveyardHalfWidth = 30
	graveyardHalfDepth = 20
)

func spawnGraveyard(commands *Commands, models *Models, rng *rand.Rand) {
	stones := []string{
		"graveyard/gravestone-bevel",
		"graveyard/gravestone-broken",
		"graveyard/gravestone-cross",
		"graveyard/gravestone-round",
		"graveyard/gravestone-wide",
		"graveyard/gravestone-decorative",
	}

	s := float32(scaleGraveyard)

	// rows of graves, facing the town
	for row := float32(-graveyardHalfDepth + 4); row < graveyardHalfDepth-2; row += 4 {
		for col := float32(-graveyardHalfWidth + 3); col < graveyardHalfWidth-2; col += 3 {
			if rng.Float32() < 0.25 {
				continue
			}

			model := models.Get(stones[rng.IntN(len(stones))])
			pos := graveyardCenter.Add(glm.Vec3f{col + rng.Float32()*0.6 - 0.3, 0, row + rng.Float32()*0.6 - 0.3})
			tilt := glm.RotationYQuat(glm.Rad(rng.Float32()*0.4 - 0.2)).Mul(glm.RotationXQuat(glm.Rad(rng.Float32()*0.2 - 0.1)))

			// light and loose, explosions toss them around
			spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(tilt).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 400})
		}
	}

	// crypts at the back
	for _, x := range []float32{-18, 0, 18} {
		model := models.Get("graveyard/crypt-small")
		pos := graveyardCenter.Add(glm.Vec3f{x, 0, -graveyardHalfDepth - 3})
		spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithScaleXYZ(3, 3, 3), PropSettings{})
	}

	// iron fence around the graveyard, with gaps for the dead to walk through
	fence := models.Get("graveyard/iron-fence")
	damaged := models.Get("graveyard/iron-fence-damaged")
	fenceLength := fence.Size()[0] * s

	spawnFenceLine := func(from, to glm.Vec3f) {
		dir := to.Sub(from)
		length := dir.Length()
		dir = dir.Normalize()
		rotation := glm.QuatFromRotationArc(glm.Vec3f{1, 0, 0}, dir)

		for d := fenceLength / 2; d < length; d += fenceLength {
			// every few segments a gap
			if rng.Float32() < 0.2 {
				continue
			}

			model := fence
			if rng.Float32() < 0.3 {
				model = damaged
			}

			pos := from.Add(dir.Scale(d))
			spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(rotation).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 300})
		}
	}

	c := graveyardCenter
	w, d := float32(graveyardHalfWidth), float32(graveyardHalfDepth)
	spawnFenceLine(c.Add(glm.Vec3f{-w, 0, d}), c.Add(glm.Vec3f{-4, 0, d}))
	spawnFenceLine(c.Add(glm.Vec3f{4, 0, d}), c.Add(glm.Vec3f{w, 0, d}))
	spawnFenceLine(c.Add(glm.Vec3f{-w, 0, -d}), c.Add(glm.Vec3f{-w, 0, d}))
	spawnFenceLine(c.Add(glm.Vec3f{w, 0, d}), c.Add(glm.Vec3f{w, 0, -d}))
}

// keepClear reports if a position is in an area that should stay free of scenery:
// the player start and the graveyard.
func keepClear(pos glm.Vec3f) bool {
	if pos.Length() < 12 {
		return true
	}

	// an open field between the start and the graveyard
	if math.Abs(float64(pos[0])) < 10 && pos[2] > -25 && pos[2] < 20 {
		return true
	}

	rel := pos.Sub(graveyardCenter)
	return math.Abs(float64(rel[0])) < graveyardHalfWidth+3 && math.Abs(float64(rel[2])) < graveyardHalfDepth+6
}

func randomArenaPosition(rng *rand.Rand, margin float32) glm.Vec3f {
	for {
		pos := glm.Vec3f{
			(rng.Float32()*2 - 1) * (arenaHalfSize - margin),
			0,
			(rng.Float32()*2 - 1) * (arenaHalfSize - margin),
		}

		if !keepClear(pos) {
			return pos
		}
	}
}

func randomYaw(rng *rand.Rand) glm.Quat {
	return glm.RotationYQuat(glm.Rad(rng.Float32() * 2 * math.Pi))
}

func spawnScenery(commands *Commands, models *Models, rng *rand.Rand) {
	// trees: tall and static, the trunk is much thinner than the crown
	trees := []string{"graveyard/pine", "graveyard/pine-crooked", "graveyard/pine-fall", "graveyard/pine-fall-crooked"}
	for range 70 {
		pos := randomArenaPosition(rng, 4)
		scale := 3.5 + rng.Float32()*2
		model := models.Get(trees[rng.IntN(len(trees))])
		spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(scale, scale, scale),
			PropSettings{ColliderScale: glm.Vec3f{0.15, 1, 0.15}})
	}

	// clusters of loose props
	type propKind struct {
		name    string
		scale   float32
		density float32
		count   int
	}

	kinds := []propKind{
		{"survival/barrel", scaleSurvival, 300, 30},
		{"survival/box", scaleSurvival, 250, 40},
		{"survival/box-large", scaleSurvival, 250, 20},
		{"car/box", scaleCar, 150, 30},
		{"car/cone", scaleCar, 200, 30},
		{"car/debris-tire", scaleCar, 300, 20},
		{"graveyard/hay-bale", scaleGraveyard * 1.5, 150, 25},
		{"graveyard/pumpkin", scaleGraveyard, 200, 30},
		{"graveyard/pumpkin-carved", scaleGraveyard, 200, 15},
		{"graveyard/coffin", scaleGraveyard * 1.2, 250, 12},
		{"survival/tree-log", scaleSurvival * 1.5, 400, 15},
	}

	for _, kind := range kinds {
		model := models.Get(kind.name)

		for range kind.count {
			// stacks and piles around a random spot
			pos := randomArenaPosition(rng, 6)
			s := kind.scale
			transform := TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s, s)
			var extra []ErasedComponent
			if kind.name == "survival/barrel" {
				extra = append(extra, Explosive{Health: barrelHealth})
			}

			spawnProp(commands, model, transform, PropSettings{Dynamic: true, Density: kind.density}, extra...)
		}
	}

	// rocks, static
	rocks := []string{"survival/rock-a", "survival/rock-b", "survival/rock-c", "graveyard/rocks"}
	for range 25 {
		pos := randomArenaPosition(rng, 4)
		s := 3 + rng.Float32()*3
		spawnProp(commands, models.Get(rocks[rng.IntN(len(rocks))]), TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s, s), PropSettings{HullCollider: true})
	}

	// abandoned cars, heavy but explosions still flip them. The player can drive them too
	cars := []string{"car/taxi", "car/suv", "car/van", "car/truck", "car/police"}
	for range 10 {
		pos := randomArenaPosition(rng, 8)
		model := models.Get(cars[rng.IntN(len(cars))])
		s := float32(scaleCar)
		spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 150}, Car{})
	}
}

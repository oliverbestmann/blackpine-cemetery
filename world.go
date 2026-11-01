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
	// towards the sun of the sky texture, about 74° above the horizon
	sunDirection = glm.Vec3f{0.218, 0.963, -0.159}.Normalize()

	// the shadow map covers this box around the player
	sunShadowVolumes = [3]glm.Vec3f{
		{60, 60, 300},
		{240, 240, 300},
		{480, 480, 300},
	}
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
	spawnGraveyard(commands, models, assets, rng)
	spawnGraveyardLights(commands, models, assets)
	spawnScenery(commands, models, rng)
	spawnVegetation(commands, models, rng)
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
	light.ShadowVolumes = sunShadowVolumes[:]
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
		sunShadowVolumeHeight := sunShadowVolumes[0][2]

		// the volume starts at the light's position and reaches along the light's direction
		sun.Transform.Translation = center.Add(sunDirection.Scale(sunShadowVolumeHeight / 2))
	}
}

// PropSettings describes how a prop takes part in the physics.
type PropSettings struct {
	Dynamic bool

	// Density in kg/m³ of the collider, zero for the default of 1000
	Density float32

	// Slices cuts the model into this many horizontal slabs with one convex hull each,
	// for models that get narrower or wider with the height, like trees. Zero means one hull.
	Slices int

	// TriMesh uses the triangles of the model as collider. Static props only.
	TriMesh bool
}

// Tree is a static tree. An explosion close by knocks it over.
type Tree struct {
	Component[Tree]
	Model *Model
}

// spawnProp spawns a model as a single entity with convex hull colliders around it.
func spawnProp(commands *Commands, model *Model, transform Transform, settings PropSettings, extra ...ErasedComponent) EntityId {
	collider := slicedHullCollider(model.Merged, settings.Slices)
	if settings.TriMesh && !settings.Dynamic {
		collider = physics.TriMeshCollider(model.Merged.Vertices(), model.Merged.Indices())
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

	// the wall model sits behind its origin, so the walls miss the corners. A pillar on the
	// crossing of the wall center lines closes them.
	pillar := models.Get("graveyard/pillar-square")
	centerOffset := -(wall.Min[2] + wall.Max[2]) / 2 * wallScale
	corner := arenaHalfSize + centerOffset

	// twice the center offset wide, so it reaches the end of the walls, and a bit taller than the walls
	scaleXZ := 2 * centerOffset / pillar.Size()[0]
	scaleY := 1.3 * wall.Size()[1] * wallScale / pillar.Size()[1]

	for _, sign := range [4][2]float32{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		transform := TransformFromXYZ(sign[0]*corner, 0, sign[1]*corner).WithScaleXYZ(scaleXZ, scaleY, scaleXZ)
		spawnProp(commands, pillar, transform, PropSettings{})
	}
}

// graveyardCenter is the center of the graveyard, the dead rise there.
var graveyardCenter = glm.Vec3f{0, 0, -45}

const (
	graveyardHalfWidth = 30
	graveyardHalfDepth = 20
)

func spawnGraveyard(commands *Commands, models *Models, assets *GameAssets, rng *rand.Rand) {
	stones := []string{
		"graveyard/gravestone-bevel",
		"graveyard/gravestone-broken",
		"graveyard/gravestone-cross",
		"graveyard/gravestone-round",
		"graveyard/gravestone-wide",
		"graveyard/gravestone-decorative",
		"graveyard/gravestone-cross-large",
		"graveyard/gravestone-roof",
		"graveyard/cross",
		"graveyard/cross-wood",
		"graveyard/cross-column",
		"graveyard/pillar-obelisk",
	}

	plots := []string{"graveyard/grave", "graveyard/grave-border"}
	candles := []string{"graveyard/candle", "graveyard/candle-multiple"}
	urns := []string{"graveyard/urn-round", "graveyard/urn-square"}
	tombs := []string{"graveyard/crypt-a", "graveyard/crypt-b"}

	s := float32(scaleGraveyard)

	// rows of graves, facing the town
	for row := float32(-graveyardHalfDepth + 4); row < graveyardHalfDepth-2; row += 4 {
		for col := float32(-graveyardHalfWidth + 3); col < graveyardHalfWidth-2; col += 3 {
			if rng.Float32() < 0.25 {
				continue
			}

			pos := graveyardCenter.Add(glm.Vec3f{col + rng.Float32()*0.6 - 0.3, 0, row + rng.Float32()*0.6 - 0.3})

			// now and then a heavy stone tomb instead of a grave
			if rng.Float32() < 0.08 {
				spawnProp(commands, models.Get(tombs[rng.IntN(len(tombs))]), TransformFromXYZ(pos.Add(glm.Vec3f{0, 0, 1}).XYZ()).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 1500})
				continue
			}

			// the plot in front of the stone, flat, the dead climb through it
			if rng.Float32() < 0.6 {
				plot := TransformFromXYZ(pos.Add(glm.Vec3f{0, 0.01, 1.6}).XYZ()).WithScaleXYZ(s, s, s)
				spawnDecor(commands, models.Get(plots[rng.IntN(len(plots))]), plot)
			}

			if rng.Float32() < 0.2 {
				candle := models.Get(candles[rng.IntN(len(candles))])
				at := pos.Add(glm.Vec3f{0.5, 0, 0.5})
				spawnDecor(commands, candle, TransformFromXYZ(at.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s, s))
				spawnFlame(commands, assets, at.Add(glm.Vec3f{0, candle.Max[1]*s + 0.05, 0}), 0.06, 0, 0)
			}

			if rng.Float32() < 0.1 {
				spawnProp(commands, models.Get(urns[rng.IntN(len(urns))]), TransformFromXYZ(pos.Add(glm.Vec3f{-0.6, 0, 0.4}).XYZ()).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 600})
			}

			model := models.Get(stones[rng.IntN(len(stones))])
			tilt := glm.RotationYQuat(glm.Rad(rng.Float32()*0.4 - 0.2)).Mul(glm.RotationXQuat(glm.Rad(rng.Float32()*0.2 - 0.1)))

			// light and loose, explosions toss them around
			spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(tilt).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 400, Slices: 3})
		}
	}

	// crypts at the back, a large one with an altar in the middle
	for _, x := range []float32{-18, 18} {
		model := models.Get("graveyard/crypt-small")
		pos := graveyardCenter.Add(glm.Vec3f{x, 0, -graveyardHalfDepth - 3})
		spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithScaleXYZ(3, 3, 3), PropSettings{})
	}

	large := models.Get("graveyard/crypt-large")
	largePos := graveyardCenter.Add(glm.Vec3f{0, 0, -graveyardHalfDepth - 4})
	spawnProp(commands, large, TransformFromXYZ(largePos.XYZ()).WithScaleXYZ(3, 3, 3), PropSettings{})
	roofPos := largePos.Add(glm.Vec3f{0, large.Max[1] * 3, 0})
	spawnProp(commands, models.Get("graveyard/crypt-large-roof"), TransformFromXYZ(roofPos.XYZ()).WithScaleXYZ(3, 3, 3), PropSettings{})

	altar := graveyardCenter.Add(glm.Vec3f{0, 0, -graveyardHalfDepth + 2.5})
	spawnProp(commands, models.Get("graveyard/altar-stone"), TransformFromXYZ(altar.XYZ()).WithScaleXYZ(s, s, s), PropSettings{})

	// benches outside the gate
	bench := models.Get("graveyard/bench")
	for _, x := range []float32{-10, 10} {
		pos := glm.Vec3f{x, 0, graveyardCenter[2] + graveyardHalfDepth + 4}
		spawnProp(commands, bench, TransformFromXYZ(pos.XYZ()).WithRotation(glm.RotationYQuat(glm.Rad(math.Pi))).WithScaleXYZ(s, s, s), PropSettings{Dynamic: true, Density: 500})
	}

	// iron fence around the graveyard, with gaps for the dead to walk through
	fence := models.Get("graveyard/iron-fence")
	damaged := models.Get("graveyard/iron-fence-damaged")
	fenceLength := fence.Size()[0] * s

	// static pillars at the corners, the lines stop short of them. Lines that meet in a
	// corner would overlap, and the physics would push the dynamic segments over.
	pillar := models.Get("graveyard/pillar-square")
	inset := pillar.Size()[0]*s/2 + 0.05

	spawnFenceLine := func(from, to glm.Vec3f) {
		dir := to.Sub(from).Normalize()
		from, to = from.Add(dir.Scale(inset)), to.Sub(dir.Scale(inset))
		length := to.Sub(from).Length()
		rotation := glm.QuatFromRotationArc(glm.Vec3f{1, 0, 0}, dir)

		// stretch the segments a little, so the line ends exactly at the corners
		count := int(math.Ceil(float64(length / fenceLength)))
		step := length / float32(count)
		stretch := s * step / fenceLength
		depthOffset := (fence.Min[2] + fence.Max[2]) / 2 * s

		for idx := range count {
			// every few segments a gap, but the corners stay closed
			if idx > 0 && idx < count-1 && rng.Float32() < 0.2 {
				continue
			}

			model := fence
			if rng.Float32() < 0.3 {
				model = damaged
			}

			// the model is off center in depth, put it on the line so the corners meet
			pos := from.Add(dir.Scale((float32(idx) + 0.5) * step)).Sub(rotation.Transform(glm.Vec3f{0, 0, depthOffset}))
			spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(rotation).WithScaleXYZ(stretch, s, s), PropSettings{Dynamic: true, Density: 300})
		}
	}

	c := graveyardCenter
	w, d := float32(graveyardHalfWidth), float32(graveyardHalfDepth)
	spawnFenceLine(c.Add(glm.Vec3f{-w, 0, d}), c.Add(glm.Vec3f{-4, 0, d}))
	spawnFenceLine(c.Add(glm.Vec3f{4, 0, d}), c.Add(glm.Vec3f{w, 0, d}))
	spawnFenceLine(c.Add(glm.Vec3f{-w, 0, -d}), c.Add(glm.Vec3f{-w, 0, d}))
	spawnFenceLine(c.Add(glm.Vec3f{w, 0, d}), c.Add(glm.Vec3f{w, 0, -d}))
	spawnFenceLine(c.Add(glm.Vec3f{-w, 0, -d}), c.Add(glm.Vec3f{w, 0, -d}))

	for _, corner := range []glm.Vec3f{{-w, 0, -d}, {w, 0, -d}, {-w, 0, d}, {w, 0, d}} {
		spawnProp(commands, pillar, TransformFromXYZ(c.Add(corner).XYZ()).WithScaleXYZ(s, s, s), PropSettings{})
	}
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
	// trees: tall and static until an explosion knocks them over
	trees := []string{"graveyard/pine", "graveyard/pine-crooked", "graveyard/pine-fall", "graveyard/pine-fall-crooked"}
	for range 70 {
		pos := randomArenaPosition(rng, 4)
		scale := 3.5 + rng.Float32()*2
		model := models.Get(trees[rng.IntN(len(trees))])
		spawnProp(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(scale, scale, scale),
			PropSettings{TriMesh: true}, Tree{Model: model})
	}

	// rows of trees behind the walls, out of reach, so they need no physics
	for row := range 3 {
		distance := arenaHalfSize + 5 + float32(row)*5.5

		for side := range 4 {
			rotation := glm.RotationYQuat(glm.Rad(float64(side) * math.Pi / 2))

			for offset := -distance; offset < distance; offset += 6 + rng.Float32()*3 {
				jitter := (rng.Float32()*2 - 1) * 2
				pos := rotation.Transform(glm.Vec3f{offset, 0, -distance - jitter})
				scale := 3.5 + rng.Float32()*2.5
				model := models.Get(trees[rng.IntN(len(trees))])

				spawnDecor(commands, model, TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(scale, scale, scale))
			}
		}
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
		{"graveyard/coffin-old", scaleGraveyard * 1.2, 250, 8},
		{"graveyard/pumpkin-tall-carved", scaleGraveyard, 200, 12},
		{"graveyard/trunk", scaleGraveyard * 1.2, 300, 8},
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
	rocks := []string{"survival/rock-a", "survival/rock-b", "survival/rock-c", "graveyard/rocks", "graveyard/rocks-tall"}
	for range 25 {
		pos := randomArenaPosition(rng, 4)
		s := 3 + rng.Float32()*3
		spawnProp(commands, models.Get(rocks[rng.IntN(len(rocks))]), TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s, s), PropSettings{})
	}

	// abandoned cars, heavy but explosions still flip them. The player can drive them too
	cars := []string{"car/taxi", "car/suv", "car/van", "car/truck", "car/police", "car/ambulance", "car/garbage-truck", "car/sedan-sports", "car/hatchback-sports"}
	for range 10 {
		pos := randomArenaPosition(rng, 8)
		s := float32(scaleCar)
		spawnCar(commands, models, cars[rng.IntN(len(cars))], TransformFromXYZ(pos.XYZ()).WithRotation(randomYaw(rng)).WithScaleXYZ(s, s, s))
	}
}

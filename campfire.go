package main

import (
	_ "embed"
	"math"
	"math/rand/v2"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

//go:embed assets/shaders/fire.wgsl
var fireShaderCode string

var fireShader = &ShaderDef{Label: "fire", Source: fireShaderCode, FragmentEntry: "fire"}

// campCenter is the survivor camp next to the road, where the player held out so far.
var campCenter = glm.Vec3f{17, 0, 27}

// FireSprite is a flame sprite that turns to the camera around the vertical axis.
type FireSprite struct {
	Component[FireSprite]
}

// FireLight flickers a point light.
type FireLight struct {
	Component[FireLight]
	Intensity float32
	Phase     float32
}

func campfirePlugin(app *App) {
	app.AddSystems(PostUpdate, System(fireBillboardSystem).InSet(beforeTransforms))
	app.AddSystems(Update, fireLightSystem)
}

// spawnCamp builds the survivor camp: tents, bedrolls, a workbench, barricades and a campfire.
func spawnCamp(commands *Commands, models *Models, assets *GameAssets, rng *rand.Rand) {
	const s = scaleSurvival * 1.6

	at := func(name string, x, z float32, yaw glm.Rad, settings PropSettings) {
		scale := float32(s)
		if name == "survival/tree-log-small" {
			// the logs are seats, not trunks
			scale = scaleSurvival
		}

		transform := TransformFromXYZ(campCenter[0]+x, 0, campCenter[2]+z).WithRotation(glm.RotationYQuat(yaw)).WithScaleXYZ(scale, scale, scale)
		spawnProp(commands, models.Get(name), transform, settings)
	}

	at("survival/tent-canvas", -5, -4, glm.DegToRad(30), PropSettings{})
	at("survival/tent", 4, -5, glm.DegToRad(-20), PropSettings{})
	at("survival/workbench", 6, 2, glm.DegToRad(-80), PropSettings{})
	at("survival/bedroll", -2.5, 2.5, glm.DegToRad(100), PropSettings{Dynamic: true, Density: 100})
	at("survival/bedroll-packed", 2.5, 3.5, glm.DegToRad(10), PropSettings{Dynamic: true, Density: 100})
	at("survival/chest", -6, 1, glm.DegToRad(70), PropSettings{Dynamic: true, Density: 400})
	at("survival/bucket", -4, 4.5, 0, PropSettings{Dynamic: true, Density: 200})
	at("survival/resource-wood", 3.5, -1.5, glm.DegToRad(40), PropSettings{Dynamic: true, Density: 300})

	// logs to sit on around the fire
	for i := range 3 {
		angle := float32(i)*2*math.Pi/3 + 0.4
		at("survival/tree-log-small", cos32(angle)*2.6, sin32(angle)*2.6, glm.Rad(-angle), PropSettings{Dynamic: true, Density: 400})
	}

	// barricades on the side facing the graveyard
	for i := range 6 {
		x := float32(i)*2.4 - 6
		at("survival/fence-fortified", x, -9+rng.Float32()*0.6, glm.Rad(rng.Float32()*0.2-0.1), PropSettings{Dynamic: true, Density: 300})
	}

	at("survival/campfire-pit", 0, 0, 0, PropSettings{})
	at("survival/campfire-stand", 0, 0, glm.DegToRad(20), PropSettings{})

	spawnFire(commands, assets, campCenter.Add(glm.Vec3f{0, 0.15, 0}), 1.6, 2.6, 45000, rng.Float32())
}

// spawnFire spawns a flame sprite with a flickering point light. phase in [0, 1] shifts the flicker.
func spawnFire(commands *Commands, assets *GameAssets, pos glm.Vec3f, width, height, intensity, phase float32) {
	commands.Spawn(
		Round{},
		TransformFromXYZ(pos.XYZ()),
		// the shader does not read the texture, the color alpha shifts the noise
		Sprite{Texture: assets.WhiteTexture, CustomSize: Some(glm.Vec2f{width, height}), Color: ColorLinearRGBA(1, 1, 1, max(0.01, phase))},
		AnchorBottomCenter,
		CustomShader{Shader: fireShader},
		FireSprite{},
	)

	commands.Spawn(
		Round{},
		TransformFromXYZ(pos.Add(glm.Vec3f{0, height * 0.5, 0}).XYZ()),
		PointLight{Color: ColorSRGB(1, 0.55, 0.2), Intensity: intensity, Range: 16},
		FireLight{Intensity: intensity, Phase: phase * 100},
	)
}

// fireBillboardSystem turns the flames to the camera.
func fireBillboardSystem(
	cameras Query[struct {
		_         With[PlayerCamera]
		Transform Transform
	}],
	fires Query[struct {
		_         With[FireSprite]
		Transform *Transform
	}],
) {
	camera, ok := cameras.Single()
	if !ok {
		return
	}

	for fire := range fires.Items() {
		// the sprite faces +z
		toCamera := flat(camera.Transform.Translation.Sub(fire.Transform.Translation))
		fire.Transform.Rotation = glm.RotationYQuat(yawTowards(toCamera))
	}
}

// fireLightSystem flickers the fire lights with a few sines, cheap and never in sync.
func fireLightSystem(vt VirtualTime, lights Query[struct {
	Fire  FireLight
	Light *PointLight
}],
) {
	t := float32(vt.Elapsed.Seconds())

	for item := range lights.Items() {
		p := item.Fire.Phase + t
		flicker := 0.85 + 0.08*sin32(p*7.3) + 0.05*sin32(p*13.1+1.7) + 0.04*sin32(p*23.9+0.3)
		item.Light.Intensity = item.Fire.Intensity * flicker
	}
}

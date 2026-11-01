package main

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// the hud is rendered on this layer by its own camera
const hudLayer = 1

// the shorter screen side is this many hud units
const hudUnits = 1000

//go:embed assets/fonts/Creepster-Regular.ttf
var hudFontData []byte

// hudFont is a horror font for all texts of the game.
var hudFont = sync.OnceValue(func() Font {
	return Font{Faces: ParseFontFaces(hudFontData)}
})

// HudPin places a hud element relative to a screen corner or edge.
type HudPin struct {
	ComparableComponent[HudPin]

	// Screen is the pinned point in [-1, 1], (-1, -1) is the bottom left corner.
	Screen glm.Vec2f

	// Offset in hud units from the pinned point.
	Offset glm.Vec2f
}

// HudText is a text the hud updates every frame.
type HudText struct {
	ComparableComponent[HudText]
	Kind hudTextKind
}

type hudTextKind int

const (
	hudHealth hudTextKind = iota
	hudAmmo
	hudScore
	hudWave
	hudMessage
	hudEnemies
)

// EnemyMarker is an arrow around the crosshair that points to one of the last zombies.
type EnemyMarker struct {
	ImmutableComponent[EnemyMarker]
}

const (
	// markers show up when this many enemies or fewer are left
	enemyMarkerCount = 5

	// distance in meters of the markers on the floor from the player
	enemyMarkerDistance = 4
)

//go:embed assets/shaders/overlay.wgsl
var overlayShaderCode string

var overlayShader = &ShaderDef{Label: "overlay", Source: overlayShaderCode, FragmentEntry: "overlay"}

// health below this is low: red text and a pulsing red vignette
const lowHealth = 30

// DamageFlash is the full screen overlay: vignette, grain and the red flash when the player takes damage.
type DamageFlash struct {
	ImmutableComponent[DamageFlash]
}

func hudPlugin(app *App) {
	app.AddSystems(Update, hudPinSystem, System(hudTextSystem, damageFlashSystem, enemyMarkerSystem).RunIf(NotInState(GameStateLoading)))
}

func spawnHudSystem(commands *Commands, assets *GameAssets) {
	commands.Spawn(
		Round{},
		Camera{Order: 2},
		Camera2d,
		ClearColor{Color: ColorSRGBA(0, 0, 0, 0)},
		OrthographicProjection{
			ViewportOrigin: glm.Vec2f{0.5, 0.5},
			ScalingMode:    ScalingModeAutoMin{MinWidth: hudUnits, MinHeight: hudUnits},
		},
		RenderLayersOf(hudLayer),
	)

	commands.Spawn(
		Round{},
		TransformFromXYZ(0, 0, 0.5),
		Sprite{Texture: assets.CrosshairTexture, CustomSize: Some(glm.Vec2f{24, 24})},
		RenderLayersOf(hudLayer),
	)

	// full screen overlay below the hud texts, the shader does not read the texture
	commands.Spawn(
		Round{},
		TransformFromXYZ(0, 0, -0.1),
		Sprite{Texture: assets.WhiteTexture, CustomSize: Some(glm.Vec2f{4000, 4000}), Color: overlayColor(0, playerMaxHealth)},
		CustomShader{Shader: overlayShader},
		RenderLayersOf(hudLayer),
		DamageFlash{},
	)

	text := func(kind hudTextKind, pin HudPin, anchor *Anchor, size float32) {
		commands.Spawn(
			Round{},
			NewTransform(),
			pin,
			anchor,
			Text{Size: size, Color: ColorSRGB(1, 0.95, 0.85)},
			hudFont(),
			HudText{Kind: kind},
			RenderLayersOf(hudLayer),
		)
	}

	text(hudHealth, HudPin{Screen: glm.Vec2f{-1, -1}, Offset: glm.Vec2f{30, 30}}, AnchorBottomLeft, 48)
	text(hudAmmo, HudPin{Screen: glm.Vec2f{1, -1}, Offset: glm.Vec2f{-30, 30}}, AnchorBottomRight, 48)
	text(hudScore, HudPin{Screen: glm.Vec2f{1, 1}, Offset: glm.Vec2f{-30, -30}}, AnchorTopRight, 36)
	text(hudWave, HudPin{Screen: glm.Vec2f{-1, 1}, Offset: glm.Vec2f{30, -30}}, AnchorTopLeft, 36)
	text(hudMessage, HudPin{Screen: glm.Vec2f{0, 0}, Offset: glm.Vec2f{0, 180}}, AnchorCenter, 48)

	// enemies left in the wave, a skull and a number at the top
	text(hudEnemies, HudPin{Screen: glm.Vec2f{0, 1}, Offset: glm.Vec2f{6, -55}}, AnchorCenterLeft, 44)
	commands.Spawn(
		Round{},
		NewTransform(),
		HudPin{Screen: glm.Vec2f{0, 1}, Offset: glm.Vec2f{-28, -55}},
		Sprite{Texture: assets.SkullTexture, CustomSize: Some(glm.Vec2f{52, 52}), Color: ColorSRGB(1, 0.95, 0.85)},
		RenderLayersOf(hudLayer),
	)

	for range enemyMarkerCount {
		commands.Spawn(
			Round{},
			NewTransform(),
			Sprite{Texture: assets.ArrowTexture, CustomSize: Some(glm.Vec2f{32, 32}), Color: ColorSRGBA(1, 0.15, 0.1, 0)},
			RenderLayersOf(hudLayer),
			EnemyMarker{},
		)
	}
}

// enemyMarkerSystem points the markers to the living zombies once few enemies are left.
func enemyMarkerSystem(
	waves *Waves,
	screen ScreenSize,
	cameras Query[struct {
		_          With[PlayerCamera]
		Transform  GlobalTransform
		Projection PerspectiveProjection
	}],
	zombies Query[struct {
		Zombie    Zombie
		Transform GlobalTransform
	}],
	markers Query[struct {
		_         With[EnemyMarker]
		Transform *Transform
		Sprite    *Sprite
	}],
) {
	camera, ok := cameras.Single()

	var targets []glm.Vec3f
	left := waves.pending
	for z := range zombies.Items() {
		if !z.Zombie.Dead {
			left++
			targets = append(targets, z.Transform.Affine.Translation())
		}
	}

	if !ok || left > enemyMarkerCount {
		targets = nil
	}

	half := hudSize(screen).Scale(0.5)
	worldToClip := glm.IdentityMat4f()
	if ok {
		cameraToWorld := camera.Transform.Affine
		worldToCamera, _ := cameraToWorld.TryInverse()
		worldToClip = camera.Projection.ToMat4f(half).Mul(worldToCamera)
	}

	for marker := range markers.Items() {
		if len(targets) == 0 {
			marker.Sprite.Color = ColorSRGBA(1, 0.15, 0.1, 0)
			continue
		}

		pos, angle := markerPlacement(camera.Transform.Affine, worldToClip, half, targets[0])
		targets = targets[1:]

		marker.Sprite.Color = ColorSRGBA(1, 0.15, 0.1, 0.9)
		marker.Transform.Translation = glm.Vec3f{pos[0], pos[1], 0.4}
		marker.Transform.Rotation = glm.RotationZQuat(-angle)
	}
}

// markerPlacement returns the hud position of the marker for target and the direction
// it points to, clockwise from up. The marker lies on the floor around the player.
// If that spot is out of view, it sits at the bottom of the screen instead.
func markerPlacement(camera, worldToClip glm.Mat4f, half glm.Vec2f, target glm.Vec3f) (glm.Vec2f, glm.Rad) {
	center := camera.Translation()
	dir := target.Sub(center)
	dir[1] = 0
	dir = dir.Normalize()

	toHud := func(p glm.Vec3f) (glm.Vec2f, bool) {
		clip := worldToClip.Transform(glm.Vec4f{p[0], 0.02, p[2], 1})
		if clip[3] <= 0 {
			return glm.Vec2f{}, false
		}

		ndc := glm.Vec2f{clip[0] / clip[3], clip[1] / clip[3]}
		return ndc.Mul(half), max(math.Abs(float64(ndc[0])), math.Abs(float64(ndc[1]))) <= 0.95
	}

	from, okFrom := toHud(center.Add(dir.Scale(enemyMarkerDistance)))
	to, okTo := toHud(center.Add(dir.Scale(enemyMarkerDistance + 0.5)))
	if okFrom && okTo {
		d := to.Sub(from)
		return from, glm.Rad(math.Atan2(float64(d[0]), float64(d[1])))
	}

	// out of view: at the bottom, ahead in the middle and behind at the edges
	angle := markerAngle(camera, target)
	x := float32(angle) / math.Pi * (half[0] - 60)
	return glm.Vec2f{x, -half[1] + 60}, angle
}

// markerAngle is the bearing of target seen from the camera on the ground plane,
// 0 is straight ahead and positive angles turn right.
func markerAngle(camera glm.Mat4f, target glm.Vec3f) glm.Rad {
	d := target.Sub(camera.Translation())
	// columns of the camera matrix: x points right, z points backwards
	right, back := camera[0], camera[2]

	// project onto the ground, so looking up or down does not matter
	x := d[0]*right[0] + d[2]*right[2]
	y := -(d[0]*back[0] + d[2]*back[2])
	return glm.Rad(math.Atan2(float64(x), float64(y)))
}

// hudSize returns the visible hud area in hud units.
func hudSize(screen ScreenSize) glm.Vec2f {
	if screen.Vec2f[0] <= 0 || screen.Vec2f[1] <= 0 {
		return glm.Vec2f{hudUnits, hudUnits}
	}

	scale := hudUnits / min(screen.Vec2f[0], screen.Vec2f[1])
	return screen.Vec2f.Scale(scale)
}

func hudPinSystem(screen ScreenSize, pins Query[struct {
	Pin       HudPin
	Transform *Transform
}],
) {
	half := hudSize(screen).Scale(0.5)

	for item := range pins.Items() {
		pos := item.Pin.Screen.Mul(half).Add(item.Pin.Offset)
		item.Transform.Translation[0] = pos[0]
		item.Transform.Translation[1] = pos[1]
	}
}

func hudTextSystem(
	vt VirtualTime,
	stats *Stats,
	players Query[Player],
	arsenal *Arsenal,
	waves *Waves,
	zombies Query[Zombie],
	texts Query[struct {
		HudText HudText
		Text    *Text
	}],
) {
	player, _ := players.Single()

	for item := range texts.Items() {
		var value string

		switch item.HudText.Kind {
		case hudHealth:
			value = fmt.Sprintf("+ %d", int(math.Ceil(float64(max(0, player.Health)))))
			item.Text.Color = ColorSRGB(1, 0.95, 0.85)
			if player.Health < lowHealth {
				item.Text.Color = ColorSRGB(1, 0.2, 0.1)
			}

		case hudAmmo:
			value = arsenal.hudText()

		case hudScore:
			value = fmt.Sprintf("KILLS %d", stats.Kills)

		case hudWave:
			value = fmt.Sprintf("WAVE %d", stats.Wave)

		case hudMessage:
			value = stats.message(vt)

		case hudEnemies:
			value = fmt.Sprint(enemiesLeft(waves, zombies))
		}

		if item.Text.Text != value {
			item.Text.Text = value
		}
	}
}

// enemiesLeft counts the living zombies and those of the wave still to come.
func enemiesLeft(waves *Waves, zombies Query[Zombie]) int {
	left := waves.pending
	for z := range zombies.Items() {
		if !z.Dead {
			left++
		}
	}

	return left
}

func damageFlashSystem(vt VirtualTime, players Query[*Player], flashes Query[struct {
	_      With[DamageFlash]
	Sprite *Sprite
}],
) {
	player, ok := players.Single()
	if !ok {
		return
	}

	player.Damaged = max(0, player.Damaged-vt.DeltaSecs)

	for flash := range flashes.Items() {
		flash.Sprite.Color = overlayColor(player.Damaged, player.Health)
	}
}

// overlayColor packs the overlay shader parameters into the sprite color:
// r is the damage flash, g how low the health is, both in [0, 1].
func overlayColor(damaged, health float32) Color {
	damage := min(1, damaged*2.5)
	low := min(1, max(0, (lowHealth-health)/lowHealth))
	if health <= 0 {
		damage, low = 1, 1
	}

	return ColorLinearRGBA(damage, low, 0, 1)
}

func whiteTexture(ctx *RenderContext) *Texture {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for idx := range img.Pix {
		img.Pix[idx] = 255
	}

	return NewTextureFromImage(ctx, img, TextureFromImageOptions{Label: "White"})
}

// arrowTexture is a white triangle pointing up with a dark outline.
func arrowTexture(ctx *RenderContext) *Texture {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	// signed distance to a triangle with the tip at the top, negative inside
	dist := func(x, y float64) float64 {
		y = size - y // image rows grow downwards
		base := 8 - y
		// edges from the tip (32, 60) to the base corners (14, 8) and (50, 8)
		left := (-(x-32)*52 + (y-60)*18) / math.Hypot(52, 18)
		right := ((x-32)*52 + (y-60)*18) / math.Hypot(52, 18)
		return max(base, left, right)
	}

	for y := range size {
		for x := range size {
			d := dist(float64(x)+0.5, float64(y)+0.5)
			white := min(1, max(0, -d+0.5))
			alpha := min(1, max(0, -d+3.5))

			gray := uint8(255 * white)
			img.SetNRGBA(x, y, color.NRGBA{R: gray, G: gray, B: gray, A: uint8(255 * alpha)})
		}
	}

	return NewTextureFromImage(ctx, img, TextureFromImageOptions{Label: "Arrow"})
}

// crosshairTexture is a ring with a dot and a dark outline, visible on any background.
func crosshairTexture(ctx *RenderContext) *Texture {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	for y := range size {
		for x := range size {
			d := math.Hypot(float64(x)+0.5-size/2, float64(y)+0.5-size/2)

			// a white ring of radius 20 and a dot of radius 3, each with a black border
			ring := 1 - min(1, max(0, math.Abs(d-20)-1.5))
			ringBorder := 1 - min(1, max(0, math.Abs(d-20)-3.5))
			dot := 1 - min(1, max(0, d-3))
			dotBorder := 1 - min(1, max(0, d-5))

			white := max(ring, dot)
			alpha := max(ringBorder, dotBorder)

			gray := uint8(255 * white)
			img.SetNRGBA(x, y, color.NRGBA{R: gray, G: gray, B: gray, A: uint8(255 * alpha * 0.85)})
		}
	}

	return NewTextureFromImage(ctx, img, TextureFromImageOptions{Label: "Crosshair"})
}

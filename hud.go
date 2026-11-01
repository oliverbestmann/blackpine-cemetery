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

// DamageFlash is the red overlay when the player takes damage.
type DamageFlash struct {
	ImmutableComponent[DamageFlash]
}

func hudPlugin(app *App) {
	app.AddSystems(Update, hudPinSystem, System(hudTextSystem, damageFlashSystem).RunIf(NotInState(GameStateLoading)))
}

func spawnHudSystem(commands *Commands, assets *GameAssets) {
	commands.Spawn(
		Round{},
		Camera{Order: 1},
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

	// full screen red flash, invisible until the player is hit
	commands.Spawn(
		Round{},
		TransformFromXYZ(0, 0, 0.1),
		Sprite{Texture: assets.WhiteTexture, CustomSize: Some(glm.Vec2f{4000, 4000}), Color: ColorSRGBA(0.8, 0, 0, 0)},
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
			if player.Health < 30 {
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

	alpha := min(0.5, player.Damaged)
	if player.Health <= 0 {
		alpha = 0.5
	}

	for flash := range flashes.Items() {
		flash.Sprite.Color = ColorSRGBA(0.7, 0, 0, alpha)
	}
}

func whiteTexture(ctx *RenderContext) *Texture {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for idx := range img.Pix {
		img.Pix[idx] = 255
	}

	return NewTextureFromImage(ctx, img, TextureFromImageOptions{Label: "White"})
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

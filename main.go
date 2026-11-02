package main

import (
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/byke/byke2d/vyn"
	"github.com/oliverbestmann/zombies/physics"
)

type GameState int

const (
	GameStateLoading GameState = iota
	GameStatePlaying
	GameStateDead
)

// Round marks the top level entities of a round. A new round despawns them, children included.
type Round struct {
	ImmutableComponent[Round]
}

func init() {
	level := slog.LevelInfo
	if os.Getenv("ZOMBIES_DEBUG") != "" {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, Level: level})
	slog.SetDefault(slog.New(handler))
}

func main() {
	var app App

	app.AddPlugin(PluginRender)

	// after PluginRender, it inserts DefaultWindowConfig()
	windowConfig := DefaultWindowConfig()
	windowConfig.Title = "Blackpine Cemetery"
	app.InsertResource(windowConfig)
	app.InsertResource(ShadowMapConfig{Size: 2048})
	app.InsertResource(GlobalSpatialScale{Scale: glm.V3f(1, 1, 1).Scale(spatialScale)})
	app.InsertResource(GlobalAmbientLightNone)

	app.AddPlugin(physics.Plugin)
	if os.Getenv("ZOMBIES_DEBUG") != "" {
		app.AddPlugin(physics.PluginDebugColliders)
	}
	app.AddPlugin(gamePlugin)

	if path := os.Getenv("ZOMBIES_SCREENSHOT"); path != "" {
		frame, _ := strconv.Atoi(os.Getenv("ZOMBIES_SCREENSHOT_FRAME"))
		addScreenshot(&app, path, max(frame, 10))
	}

	app.MustRun()
}

// gamePlugin adds all game systems, without the renderer and physics.
func gamePlugin(app *App) {
	app.InitState(GameStateLoading)
	app.InsertResource(SoundBudget{})
	app.InsertResource(Stats{})
	app.InsertResource(newArsenal())
	app.InsertResource(TestInput{
		Fire:  os.Getenv("ZOMBIES_AUTOFIRE") != "",
		Drive: os.Getenv("ZOMBIES_DRIVE") != "",
	})

	app.AddSystems(Startup, startLoadingSystem, spawnLoadingScreenSystem)
	app.AddSystems(Update, System(finishLoadingSystem).RunIf(InState(GameStateLoading)))
	app.AddSystems(First, resetSoundBudgetSystem)
	app.AddSystems(OnEnter(GameStatePlaying), startRoundSystem)
	app.AddSystems(Update, ExitOnEscapeSystem)
	app.AddSystems(Update, System(deathSystem).RunIf(InState(GameStatePlaying)))
	app.AddSystems(Update, System(restartSystem).RunIf(InState(GameStateDead)))

	worldPlugin(app)
	playerPlugin(app)
	hudPlugin(app)
	zombiesPlugin(app)
	gibsPlugin(app)
	wavesPlugin(app)
	particlesPlugin(app)
	explosionsPlugin(app)
	weaponsPlugin(app)
	projectilesPlugin(app)
	pickupsPlugin(app)
	carPlugin(app)
	campfirePlugin(app)
	scorePlugin(app)
	soundsPlugin(app)
}

// startRoundSystem replaces the world of the last round with a fresh one.
func startRoundSystem(vt VirtualTime, commands *Commands, arsenal *Arsenal, waves *Waves, rounds Query[struct {
	_        With[Round]
	EntityId EntityId
}],
) {
	for item := range rounds.Items() {
		commands.Entity(item.EntityId).Despawn()
	}

	stats := Stats{Wave: 1}

	// for testing: start at a later wave
	if wave, err := strconv.Atoi(os.Getenv("ZOMBIES_WAVE")); err == nil && wave > 1 {
		stats.Wave = wave
	}

	stats.Show(vt, "THE DEAD ARE RISING\nWASD + MOUSE - 1-7 WEAPONS - F CAR", 5*time.Second)
	commands.InsertResource(stats)
	*arsenal = newArsenal()

	// for testing: all weapons, starting with the given one
	if weapon, err := strconv.Atoi(os.Getenv("ZOMBIES_WEAPON")); err == nil {
		for kind := range weaponCount {
			arsenal.Give(kind)
		}

		arsenal.Select(WeaponKind(weapon))
	}
	startWaves(waves, stats.Wave)
	commands.RunSystem(spawnWorldSystem)
	commands.RunSystem(spawnPlayerSystem)
	commands.RunSystem(spawnHudSystem)
	commands.RunSystem(spawnCarSystem)

	if os.Getenv("ZOMBIES_DEMO") != "" {
		commands.RunSystem(spawnDemoCrowdSystem)
	}
}

func spawnLoadingScreenSystem(commands *Commands) {
	// the game shows its own loading screen from here on
	hideLoadingIndicator()

	commands.Spawn(
		Camera{Order: 10},
		Camera2d,
		ClearColor{Color: ColorSRGBA(0.08, 0.02, 0.02, 1)},
		OrthographicProjection{
			ViewportOrigin: glm.V2f(0.5, 0.5),
			ScalingMode:    ScalingModeAutoMin{MinWidth: 1000, MinHeight: 1000},
		},
		DespawnOnExitState(GameStateLoading),
	)

	commands.Spawn(
		NewTransform(),
		AnchorCenter,
		Text{Text: "Digging up the dead...", Size: 48, Color: ColorSRGB(0.8, 0.1, 0.05)},
		hudFont(),
		LoadingText{},
		DespawnOnExitState(GameStateLoading),
	)
}

// addScreenshot renders offscreen and saves the given frame as PNG.
// Debug dumps only happen every 10th frame, so the frame is rounded down to a multiple of 10.
func addScreenshot(app *App, path string, frame int) {
	frame -= frame % 10

	// the dump fires once per camera, the HUD camera comes second
	var count int

	app.AddPlugin(PluginDebugDumpCamera(func(frameIndex int, im *image.NRGBA) error {
		if frameIndex != frame {
			return nil
		}

		count++
		path := path
		if count > 1 {
			path = fmt.Sprintf("%s.%d.png", strings.TrimSuffix(path, ".png"), count)
		}

		fp, err := os.Create(path)
		if err != nil {
			return err
		}

		defer fp.Close()

		slog.Info("Saving screenshot", slog.String("path", path), slog.Int("frame", frame))
		return png.Encode(fp, im)
	}))

	app.InsertResource(WindowConfig{
		Width:     960,
		Height:    540,
		Offscreen: &OffscreenWindowConfig{FrameCount: frame + 5},
	})

	app.InsertResource(GlobalVolume{Volume: 0})
}

// deathSystem ends the round when the player died.
func deathSystem(vt VirtualTime, stats *Stats, state *NextState[GameState], players Query[Player]) {
	player, ok := players.Single()
	if !ok || player.Health > 0 {
		return
	}

	stats.Show(vt, fmt.Sprintf("YOU GOT EATEN\n%d POINTS - %d KILLS - PRESS R TO RISE AGAIN", stats.Score, stats.Kills), time.Hour)
	state.Set(GameStateDead)
}

func restartSystem(keys Keys, state *NextState[GameState]) {
	if keys.IsJustPressed(vyn.KeyR) {
		state.Set(GameStatePlaying)
	}
}

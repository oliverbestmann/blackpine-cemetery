package main

import (
	"image"
	"image/png"
	"log/slog"
	"os"
	"strconv"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
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
	windowConfig.Title = "Zombies of Hollow Pines"
	app.InsertResource(windowConfig)
	app.InsertResource(ShadowMapConfig{Size: 2048})
	app.InsertResource(GlobalSpatialScale{Scale: glm.Vec3f{1, 1, 1}.Scale(spatialScale)})
	app.InsertResource(GlobalAmbientLightNone)

	app.AddPlugin(physics.Plugin)
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
	app.InsertResource(Arsenal{})

	app.AddSystems(Startup, startLoadingSystem, spawnLoadingScreenSystem)
	app.AddSystems(Update, System(finishLoadingSystem).RunIf(InState(GameStateLoading)))
	app.AddSystems(First, resetSoundBudgetSystem)
	app.AddSystems(OnEnter(GameStatePlaying), startRoundSystem)
	app.AddSystems(Update, ExitOnEscapeSystem)

	worldPlugin(app)
	playerPlugin(app)
	hudPlugin(app)
}

// startRoundSystem replaces the world of the last round with a fresh one.
func startRoundSystem(commands *Commands, rounds Query[struct {
	_        With[Round]
	EntityId EntityId
}],
) {
	for item := range rounds.Items() {
		commands.Entity(item.EntityId).Despawn()
	}

	commands.InsertResource(Stats{Wave: 1})
	commands.RunSystem(spawnWorldSystem)
	commands.RunSystem(spawnPlayerSystem)
	commands.RunSystem(spawnHudSystem)
}

func spawnLoadingScreenSystem(commands *Commands) {
	commands.Spawn(
		Camera{Order: 10},
		Camera2d,
		ClearColor{Color: ColorSRGBA(0.08, 0.02, 0.02, 1)},
		OrthographicProjection{
			ViewportOrigin: glm.Vec2f{0.5, 0.5},
			ScalingMode:    ScalingModeAutoMin{MinWidth: 1000, MinHeight: 1000},
		},
		DespawnOnExitState(GameStateLoading),
	)

	commands.Spawn(
		NewTransform(),
		AnchorCenter,
		Text{Text: "Digging up the dead...", Size: 40, Color: ColorSRGB(0.8, 0.1, 0.05)},
		LoadingText{},
		DespawnOnExitState(GameStateLoading),
	)
}

// addScreenshot renders offscreen and saves the given frame as PNG.
// Debug dumps only happen every 10th frame, so the frame is rounded down to a multiple of 10.
func addScreenshot(app *App, path string, frame int) {
	frame -= frame % 10

	app.AddPlugin(PluginDebugDumpCamera(func(frameIndex int, im *image.NRGBA) error {
		if frameIndex != frame {
			return nil
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

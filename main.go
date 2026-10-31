package main

import (
	"image"
	"image/png"
	"log/slog"
	"os"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

func init() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})

	slog.SetDefault(slog.New(handler))
}

func main() {
	var app App

	app.AddPlugin(PluginRender)

	app.AddSystems(Startup, setupSystem)
	app.AddSystems(Update, ExitOnEscapeSystem)

	// ZOMBIES_SCREENSHOT=out.png renders a few frames offscreen and saves frame 10
	if path := os.Getenv("ZOMBIES_SCREENSHOT"); path != "" {
		addScreenshot(&app, path)
	}

	app.MustRun()
}

func setupSystem(commands *Commands) {
	commands.Spawn(
		Camera{},
		Camera2d,
		ClearColor{Color: ColorSRGB(0.1, 0.12, 0.1)},
		OrthographicProjection{
			ViewportOrigin: glm.Vec2f{0.5, 0.5},
		},
	)

	commands.Spawn(
		TransformFromXYZ(0, 0, 0.1),
		AnchorCenter,
		Text{Text: "Zombies", Size: 64, Color: ColorSRGB(0.5, 0.9, 0.4)},
	)
}

func addScreenshot(app *App, path string) {
	app.AddPlugin(PluginDebugDumpCamera(func(frameIndex int, im *image.NRGBA) error {
		if frameIndex != 10 {
			return nil
		}

		fp, err := os.Create(path)
		if err != nil {
			return err
		}

		defer fp.Close()

		return png.Encode(fp, im)
	}))

	app.InsertResource(WindowConfig{
		Width:     640,
		Height:    480,
		Offscreen: &OffscreenWindowConfig{FrameCount: 15},
	})
}

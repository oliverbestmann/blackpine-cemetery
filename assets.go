package main

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/byke/byke2d/gltf"
	"github.com/oliverbestmann/webgpu/wgpu"
)

// modelFiles are the models the game uses, relative to assets/ and without the .glb suffix.
var modelFiles = []string{
	"graveyard/character-zombie",
	"graveyard/character-skeleton",
	"graveyard/character-vampire",
	"graveyard/character-keeper",
	"graveyard/character-ghost",
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
	"graveyard/grave",
	"graveyard/grave-border",
	"graveyard/candle",
	"graveyard/road",
	"graveyard/candle-multiple",
	"graveyard/urn-round",
	"graveyard/urn-square",
	"graveyard/crypt-a",
	"graveyard/crypt-b",
	"graveyard/crypt-large",
	"graveyard/crypt-large-roof",
	"graveyard/altar-stone",
	"graveyard/bench",
	"graveyard/fire-basket",
	"graveyard/lantern-candle",
	"graveyard/lightpost-double",
	"graveyard/coffin-old",
	"graveyard/pumpkin-tall-carved",
	"graveyard/trunk",
	"graveyard/rocks-tall",
	"survival/grass",
	"survival/grass-large",
	"survival/patch-grass",
	"survival/patch-grass-large",
	"survival/rock-flat-grass",
	"graveyard/crypt",
	"graveyard/crypt-small",
	"graveyard/pine",
	"graveyard/pine-crooked",
	"graveyard/pine-fall",
	"graveyard/pine-fall-crooked",
	"graveyard/iron-fence",
	"graveyard/iron-fence-damaged",
	"graveyard/pillar-square",
	"graveyard/brick-wall",
	"graveyard/hay-bale",
	"graveyard/hay-bale-bundled",
	"graveyard/pumpkin",
	"graveyard/pumpkin-carved",
	"graveyard/coffin",
	"graveyard/rocks",
	"graveyard/lightpost-single",
	"graveyard/debris",
	"graveyard/shovel",
	"blaster/blaster-a",
	"blaster/blaster-b",
	"blaster/blaster-c",
	"blaster/blaster-d",
	"blaster/blaster-e",
	"blaster/blaster-f",
	"blaster/blaster-g",
	"blaster/blaster-h",
	"blaster/blaster-i",
	"blaster/blaster-j",
	"blaster/blaster-k",
	"blaster/blaster-l",
	"blaster/blaster-m",
	"blaster/blaster-n",
	"blaster/blaster-o",
	"blaster/blaster-p",
	"blaster/blaster-q",
	"blaster/blaster-r",
	"blaster/grenade-a",
	"blaster/crate-medium",
	"blaster/crate-small",
	"blaster/clip-large",
	"car/sedan",
	"car/police",
	"car/taxi",
	"car/suv",
	"car/truck",
	"car/van",
	"car/debris-tire",
	"car/debris-door",
	"car/debris-bumper",
	"car/box",
	"car/cone",
	"car/cone-flat",
	"car/firetruck",
	"car/ambulance",
	"car/garbage-truck",
	"car/sedan-sports",
	"car/hatchback-sports",
	"car/debris-bolt",
	"car/debris-door-window",
	"car/debris-drivetrain",
	"car/debris-drivetrain-axle",
	"car/debris-nut",
	"car/debris-plate-a",
	"car/debris-plate-b",
	"car/debris-plate-small-a",
	"car/debris-plate-small-b",
	"car/debris-spoiler-a",
	"car/debris-spoiler-b",
	"survival/barrel",
	"survival/tent-canvas",
	"survival/tent",
	"survival/workbench",
	"survival/bedroll",
	"survival/bedroll-packed",
	"survival/chest",
	"survival/bucket",
	"survival/resource-wood",
	"survival/tree-log-small",
	"survival/fence-fortified",
	"survival/campfire-pit",
	"survival/campfire-stand",
	"survival/box",
	"survival/box-large",
	"survival/rock-a",
	"survival/rock-b",
	"survival/rock-c",
	"survival/tree-log",
	"survival/resource-planks",
}

// Models are all models of modelFiles, by name.
type Models struct {
	byName map[string]*Model
}

// Get returns the model with the given name, it panics for unknown names.
func (m *Models) Get(name string) *Model {
	model, ok := m.byName[name]
	if !ok {
		panic(fmt.Errorf("unknown model %q", name))
	}

	return model
}

// GameAssets are all assets besides the models.
type GameAssets struct {
	Ground           *Texture
	Skybox, Diffuse  *Texture
	Specular         *Texture
	Sounds           Sounds
	WhiteTexture     *Texture
	CrosshairTexture *Texture
	SkullTexture     *Texture
	ArrowTexture     *Texture

	// simple meshes for particles and effects
	Cube, Sphere *Mesh

	BloodMaterial, DustMaterial, TracerMaterial StandardMaterial
	FireMaterial, SmokeMaterial, RocketMaterial StandardMaterial
}

// glowing returns a material that glows with the given color, for bloom.
// byke adds the emissive color after exposure, so values are in output units.
func glowing(r, g, b float32) StandardMaterial {
	return StandardMaterial{BaseColor: ColorBlack, EmissiveScale: glm.Vec3f{r, g, b}, PerceptualRoughness: 1}
}

func plain(color Color, roughness float32) StandardMaterial {
	return StandardMaterial{BaseColor: color, PerceptualRoughness: roughness}
}

// loadingAssets are the assets while they load.
type loadingAssets struct {
	models   map[string]AsyncAsset[*gltf.Handle]
	textures map[string]AsyncAsset[*Texture]

	ground, diffuse, specular AsyncAsset[*Texture]
	skull                     AsyncAsset[*Texture]
	sounds                    loadingSounds
}

func startLoadingSystem(commands *Commands, assets *Assets) {
	cube := &LoadTextureSettings{LoadKTXTextureOptions: LoadKTXTextureOptions{}}
	cube.OverrideTextureViewDimension = wgpu.TextureViewDimensionCube

	repeat := &LoadTextureSettings{}
	repeat.AddressModeU = wgpu.AddressModeRepeat
	repeat.AddressModeV = wgpu.AddressModeRepeat
	repeat.FilterMode = wgpu.FilterModeLinear
	repeat.MipmapFilter = wgpu.MipmapFilterModeLinear

	loading := loadingAssets{
		models:   map[string]AsyncAsset[*gltf.Handle]{},
		textures: map[string]AsyncAsset[*Texture]{},
		ground:   assets.TextureWithSettings("textures/ground.jpg", repeat),
		diffuse:  assets.TextureWithSettings("sky/diffuse.ktx2", cube),
		specular: assets.TextureWithSettings("sky/specular.ktx2", cube),
		skull:    assets.Texture("textures/skull.png"),
		sounds:   loadSounds(assets),
	}

	for _, name := range modelFiles {
		loading.models[name] = assets.GLTF(name + ".glb")

		kit := kitOf(name)
		if _, ok := loading.textures[kit]; !ok {
			loading.textures[kit] = assets.Texture(path.Join(kit, "Textures/colormap.png"))
		}
	}

	commands.InsertResource(loading)
}

func kitOf(name string) string {
	kit, _, _ := strings.Cut(name, "/")
	return kit
}

// progress returns the share of loaded assets in [0, 1]. It panics if an asset failed.
func (l *loadingAssets) progress() float32 {
	var done, total int

	count := func(ok bool) {
		total++
		if ok {
			done++
		}
	}

	for _, asset := range l.models {
		count(isDone(asset))
	}

	for _, asset := range l.textures {
		count(isDone(asset))
	}

	count(isDone(l.ground))
	count(isDone(l.diffuse))
	count(isDone(l.specular))
	count(isDone(l.skull))

	for _, asset := range l.sounds.all() {
		count(isDone(asset))
	}

	return float32(done) / float32(total)
}

func isDone[T any](asset AsyncAsset[T]) bool {
	_, err, ok := asset.Poll()
	if err != nil {
		panic(fmt.Errorf("load assets: %w", err))
	}

	return ok
}

// finishLoadingSystem builds the model library once everything is loaded and starts the game.
func finishLoadingSystem(ctx *RenderContext, commands *Commands, loading *loadingAssets, state *NextState[GameState], text Query[struct {
	_    With[LoadingText]
	Text *Text
}],
) {
	progress := loading.progress()

	for item := range text.Items() {
		item.Text.Text = fmt.Sprintf("Digging up the dead... %d%%", int(progress*100))
	}

	if progress < 1 {
		return
	}

	materials := map[string]StandardMaterial{}
	for kit, texture := range loading.textures {
		materials[kit] = StandardMaterial{
			BaseColor:           ColorWhite,
			PerceptualRoughness: 0.75,
			StandardMaterialBindGroup: StandardMaterialBindGroup{
				Texture: texture.Await(),
			},
		}
	}

	models := Models{byName: map[string]*Model{}}
	for _, name := range slices.Sorted(maps.Keys(loading.models)) {
		models.byName[name] = loadModel(loading.models[name].Await(), materials[kitOf(name)])
	}

	commands.InsertResource(models)

	commands.InsertResource(GameAssets{
		Ground:           loading.ground.Await(),
		Diffuse:          loading.diffuse.Await(),
		Specular:         loading.specular.Await(),
		Skybox:           loading.specular.Await(),
		Sounds:           loading.sounds.await(),
		WhiteTexture:     whiteTexture(ctx),
		CrosshairTexture: crosshairTexture(ctx),
		SkullTexture:     loading.skull.Await(),
		ArrowTexture:     arrowTexture(ctx),
		Cube:             Cube(),
		Sphere:           Sphere(0.5, 8, 6),
		BloodMaterial:    plain(ColorSRGB(0.35, 0.01, 0.01), 0.3),
		DustMaterial:     plain(ColorSRGB(0.45, 0.38, 0.3), 1),
		SmokeMaterial:    plain(ColorSRGB(0.45, 0.42, 0.4), 1),
		RocketMaterial:   plain(ColorSRGB(0.3, 0.32, 0.3), 0.5),
		TracerMaterial:   glowing(15, 8.75, 2.5),
		FireMaterial:     glowing(10, 2.25, 0.2),
	})

	state.Set(GameStatePlaying)
}

// LoadingText shows the loading progress.
type LoadingText struct {
	ImmutableComponent[LoadingText]
}

package main

import (
	"fmt"
	"math/rand/v2"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

// SoundSet is a group of variations of one sound, a random one plays.
type SoundSet []*AudioSource

func (s SoundSet) pick() *AudioSource {
	return s[rand.IntN(len(s))]
}

// Sounds are all sound effects.
type Sounds struct {
	ShotSmall, ShotRetro, ShotLarge SoundSet
	Launch                          SoundSet
	Explosion, ExplosionLow         SoundSet
	Hit, Splat                      SoundSet
	Pickup                          SoundSet
	Engine                          SoundSet
	Metal, Wood, Plate              SoundSet
	Footstep                        SoundSet
}

type loadingSounds map[string][]AsyncAsset[*AudioSource]

// soundFiles maps each sound set to its file name prefix and number of variations.
var soundFiles = map[string]int{
	"laserSmall":             5,
	"laserRetro":             3,
	"laserLarge":             2,
	"thrusterFire":           2,
	"explosionCrunch":        5,
	"lowFrequency_explosion": 2,
	"impactPunch_heavy":      5,
	"slime":                  2,
	"forceField":             2,
	"engineCircular":         1,
	"impactMetal":            2,
	"impactWood_medium":      3,
	"impactPlate_medium":     2,
	"footstep_grass":         5,
}

func loadSounds(assets *Assets) loadingSounds {
	sounds := loadingSounds{}

	for prefix, count := range soundFiles {
		for idx := range count {
			path := fmt.Sprintf("sounds/%s_%03d.ogg", prefix, idx)
			sounds[prefix] = append(sounds[prefix], assets.AudioWithSettings(path, &LoadAudioSettings{AutoDecode: AutoDecodeModeAlways}))
		}
	}

	return sounds
}

func (l loadingSounds) all() []AsyncAsset[*AudioSource] {
	var all []AsyncAsset[*AudioSource]
	for _, assets := range l {
		all = append(all, assets...)
	}

	return all
}

func (l loadingSounds) await() Sounds {
	set := func(prefix string) SoundSet {
		var set SoundSet
		for _, asset := range l[prefix] {
			set = append(set, asset.Await())
		}

		return set
	}

	return Sounds{
		ShotSmall:    set("laserSmall"),
		ShotRetro:    set("laserRetro"),
		ShotLarge:    set("laserLarge"),
		Launch:       set("thrusterFire"),
		Explosion:    set("explosionCrunch"),
		ExplosionLow: set("lowFrequency_explosion"),
		Hit:          set("impactPunch_heavy"),
		Splat:        set("slime"),
		Pickup:       set("forceField"),
		Engine:       set("engineCircular"),
		Metal:        set("impactMetal"),
		Wood:         set("impactWood_medium"),
		Plate:        set("impactPlate_medium"),
		Footstep:     set("footstep_grass"),
	}
}

// spatial sounds play at full volume within 1/spatialScale meters
const spatialScale = 0.15

// microphone hears the spatial sounds, it sits on the camera.
var microphone = Microphone{LeftEarOffset: glm.Vec3f{-0.1, 0, 0}, RightEarOffset: glm.Vec3f{0.1, 0, 0}}

// SoundBudget limits the sounds started per frame, a crowd dying at once would be a wall of noise.
type SoundBudget struct {
	left int
}

const soundsPerFrame = 6

func resetSoundBudgetSystem(budget *SoundBudget) {
	budget.left = soundsPerFrame
}

// playAt plays a sound once at a position in the world.
func playAt(commands *Commands, budget *SoundBudget, set SoundSet, position glm.Vec3f, volume float32) {
	if budget.left <= 0 || len(set) == 0 {
		return
	}

	budget.left--

	commands.Spawn(
		Round{},
		TransformFromXYZ(position.XYZ()),
		AudioPlayerOf(set.pick()),
		PlaybackSettingsDespawn.WithVolume(float64(volume)).WithSpatial(),
	)
}

// playUI plays a sound without position, e.g. the player's own weapon.
func playUI(commands *Commands, budget *SoundBudget, set SoundSet, volume float32) {
	if budget.left <= 0 || len(set) == 0 {
		return
	}

	budget.left--

	commands.Spawn(
		Round{},
		AudioPlayerOf(set.pick()),
		PlaybackSettingsDespawn.WithVolume(float64(volume)),
	)
}

// impactSoundSystem plays a thud when things crash into each other.
func impactSoundSystem(commands *Commands, budget *SoundBudget, assets *GameAssets, hits *MessageReader[physics.Hit]) {
	played := 0
	for _, hit := range hits.Read() {
		// keep a few for the guns
		if hit.Speed < 4 || played >= 2 {
			continue
		}

		played++

		set := assets.Sounds.Wood
		switch {
		case hit.Speed > 12:
			set = assets.Sounds.Metal
		case hit.Speed > 7:
			set = assets.Sounds.Plate
		}

		playAt(commands, budget, set, hit.Point, min(1, hit.Speed/15))
	}
}

// Footsteps times the player's steps.
type Footsteps struct {
	distance float32
}

const stepLength = 2.2

func footstepSystem(
	vt VirtualTime,
	commands *Commands,
	budget *SoundBudget,
	assets *GameAssets,
	steps *Footsteps,
	players Query[struct {
		Player         Player
		LinearVelocity physics.LinearVelocity
	}],
) {
	player, ok := players.Single()
	if !ok || !player.Player.Grounded || player.Player.Driving != NoEntityId {
		return
	}

	steps.distance += flat(player.LinearVelocity.Vec3f).Length() * vt.DeltaSecs
	if steps.distance > stepLength {
		steps.distance = 0
		playUI(commands, budget, assets.Sounds.Footstep, 0.25)
	}
}

func soundsPlugin(app *App) {
	app.InsertResource(Footsteps{})
	app.AddSystems(Update, System(impactSoundSystem, footstepSystem).RunIf(InState(GameStatePlaying)))
}

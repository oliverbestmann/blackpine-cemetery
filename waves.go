package main

import (
	"fmt"
	"math"
	"time"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// Waves spawns the zombies of the current wave over time.
type Waves struct {
	// zombies of the current wave not spawned yet
	pending int

	// time until the next zombie spawns
	cooldown float32

	// pause between waves
	pause float32
}

// waveSize is the number of zombies in a wave.
func waveSize(wave int) int {
	return 25 + 20*(wave-1) + 3*(wave-1)*(wave-1)
}

// waveSpawnInterval is the time between two zombies of a wave, later waves come faster.
func waveSpawnInterval(wave int) float32 {
	return float32(max(0.04, 0.35*math.Pow(0.82, float64(wave-1))))
}

// waveKind picks the kind of the n-th zombie in a wave.
func waveKind(wave int, roll float32) ZombieKind {
	brute := min(0.12, 0.02*float32(wave-2))
	runner := min(0.35, 0.08*float32(wave-1))
	ghost := min(0.12, 0.03*float32(wave-1))

	switch {
	case roll < ghost:
		return ZombieGhost
	case roll < ghost+brute:
		return ZombieBrute
	case roll < ghost+brute+runner:
		return ZombieRunner
	default:
		return ZombieWalker
	}
}

// maxZombiesAlive caps the crowd, for performance.
const maxZombiesAlive = 160

func wavesPlugin(app *App) {
	app.InsertResource(Waves{})
	app.AddSystems(Update, System(waveSystem).RunIf(InState(GameStatePlaying)))
}

func startWaves(waves *Waves) {
	*waves = Waves{pending: waveSize(1), pause: 5}
}

func waveSystem(
	vt VirtualTime,
	commands *Commands,
	models *Models,
	rng *GameRng,
	stats *Stats,
	waves *Waves,
	players Query[struct {
		_         With[Player]
		Transform Transform
	}],
	zombies Query[struct{ _ With[Zombie] }],
) {
	dt := vt.DeltaSecs
	alive := zombies.Count()

	if waves.pause > 0 {
		waves.pause -= dt
		if waves.pause <= 0 {
			stats.Show(vt, fmt.Sprintf("WAVE %d", stats.Wave), 2*time.Second)
		}

		return
	}

	if waves.pending == 0 {
		if alive == 0 {
			// next wave
			stats.Wave++
			waves.pending = waveSize(stats.Wave)
			waves.pause = 3
		}

		return
	}

	waves.cooldown -= dt

	player, _ := players.Single()

	for waves.cooldown <= 0 && waves.pending > 0 && alive < maxZombiesAlive {
		waves.cooldown += waveSpawnInterval(stats.Wave)
		waves.pending--
		alive++

		kind := waveKind(stats.Wave, rng.Float32())
		position := spawnPosition(rng, player.Transform.Translation)
		spawnZombie(commands, models, kind, position, yawTowards(player.Transform.Translation.Sub(position)), zombieRiseSecs)
	}

	waves.cooldown = max(waves.cooldown, -1)
}

// spawnPosition picks where a zombie climbs out of the ground: mostly in the graveyard,
// sometimes anywhere in the arena, but never right next to the player.
func spawnPosition(rng *GameRng, player glm.Vec3f) glm.Vec3f {
	for {
		var pos glm.Vec3f

		if rng.Float32() < 0.6 {
			pos = graveyardCenter.Add(glm.Vec3f{
				(rng.Float32()*2 - 1) * (graveyardHalfWidth - 2),
				0,
				(rng.Float32()*2 - 1) * (graveyardHalfDepth - 2),
			})
		} else {
			pos = glm.Vec3f{
				(rng.Float32()*2 - 1) * (arenaHalfSize - 5),
				0,
				(rng.Float32()*2 - 1) * (arenaHalfSize - 5),
			}
		}

		if flat(pos.Sub(player)).Length() > 25 {
			return pos
		}
	}
}

// spawnDemoCrowdSystem spawns a crowd right in front of the player, for screenshots.
func spawnDemoCrowdSystem(commands *Commands, models *Models, rng *GameRng) {
	for idx := range 40 {
		pos := glm.Vec3f{(rng.Float32()*2 - 1) * 12, 0, -4 - rng.Float32()*20}
		spawnZombie(commands, models, waveKind(5, rng.Float32()), pos, 0, float32(idx%2)*zombieRiseSecs*rng.Float32())
	}
}

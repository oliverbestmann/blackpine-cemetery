package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

type PickupKind int

const (
	PickupWeapon PickupKind = iota
	PickupAmmo
	PickupHealth
)

const (
	pickupRange    = 2.0
	pickupLifetime = 30
	pickupHealth   = 25
)

// Pickup lies on the ground until the player walks over it.
type Pickup struct {
	Component[Pickup]
	Kind   PickupKind
	Weapon WeaponKind
	Age    float32
	Height float32
}

// Drop is what a dead zombie leaves behind.
type Drop struct {
	Kind   PickupKind
	Weapon WeaponKind
}

// rollDrop decides what a killed zombie drops, if anything.
func rollDrop(rng *rand.Rand, kind ZombieKind, wave int) (Drop, bool) {
	// a gun, not the machine gun or the shovel the player always has
	weapon := WeaponKind(1 + rng.IntN(int(Shovel)-1))

	// brutes and the keeper always drop a big gun
	if kind == ZombieBrute || kind == ZombieKeeper {
		return Drop{Kind: PickupWeapon, Weapon: weapon}, true
	}

	roll := rng.Float32()
	switch {
	case roll < 0.04+0.01*float32(min(wave, 5)):
		return Drop{Kind: PickupWeapon, Weapon: weapon}, true
	case roll < 0.30:
		return Drop{Kind: PickupAmmo}, true
	case roll < 0.36:
		return Drop{Kind: PickupHealth}, true
	default:
		return Drop{}, false
	}
}

// apply gives the pickup to the player and returns the message to show.
func (d Drop) apply(arsenal *Arsenal, player *Player) string {
	switch d.Kind {
	case PickupWeapon:
		def := weapons[d.Weapon]
		if arsenal.Give(d.Weapon) {
			arsenal.Select(d.Weapon)
			return def.Name + "!"
		}

		return fmt.Sprintf("+%d %s AMMO", def.PickupAmmo, def.Name)

	case PickupAmmo:
		// ammo for every weapon the player has
		for kind := range weaponCount {
			if arsenal.Owned[kind] {
				arsenal.AddAmmo(kind, weapons[kind].PickupAmmo)
			}
		}

		return "AMMO"

	default:
		player.Health = min(playerMaxHealth, player.Health+pickupHealth)
		return fmt.Sprintf("+%d HEALTH", pickupHealth)
	}
}

func (d Drop) model() (string, float32) {
	switch d.Kind {
	case PickupWeapon:
		return weapons[d.Weapon].Model, 2.2
	case PickupAmmo:
		return "blaster/clip-large", 3
	default:
		return "graveyard/pumpkin-carved", 2
	}
}

// glow is the color of the marker above the pickup.
func (d Drop) glow() StandardMaterial {
	switch d.Kind {
	case PickupWeapon:
		return glowing(6, 2.5, 0.05)
	case PickupAmmo:
		return glowing(0.3, 5, 0.1)
	default:
		return glowing(6, 0.15, 0.1)
	}
}

func pickupsPlugin(app *App) {
	app.AddSystems(Update, System(dropSystem, pickupSystem).RunIf(InState(GameStatePlaying)))
}

func dropSystem(commands *Commands, models *Models, assets *GameAssets, rng *GameRng, stats *Stats, killed *MessageReader[ZombieKilled]) {
	for _, kill := range killed.Read() {
		drop, ok := rollDrop(rng.Rand, kill.Kind, stats.Wave)
		if ok {
			spawnPickup(commands, models, assets, drop, flat(kill.Position))
		}
	}
}

func spawnPickup(commands *Commands, models *Models, assets *GameAssets, drop Drop, pos glm.Vec3f) {
	name, scale := drop.model()
	model := models.Get(name)

	// center the model on the pickup position, so it spins around its middle
	center := model.Min.Add(model.Max).Scale(-0.5 * scale)

	pickup := commands.Spawn(
		Round{},
		Pickup{Kind: drop.Kind, Weapon: drop.Weapon, Height: 0.8},
		TransformFromXYZ(pos.XYZ()),
		InheritVisibility,
	).Id()

	commands.Spawn(
		ChildOf{Parent: pickup},
		TransformFromXYZ(center.XYZ()).WithScaleXYZ(scale, scale, scale),
		MeshInstance{Mesh: model.Merged},
		model.Material,
	)

	// a glowing diamond above it, a cube standing on a corner, to spot the pickup from afar
	corner := glm.RotationXQuat(glm.Rad(math.Atan(1 / math.Sqrt2))).Mul(glm.RotationZQuat(glm.Rad(math.Pi / 4)))
	commands.Spawn(
		ChildOf{Parent: pickup},
		TransformFromXYZ(0, 1.5, 0).WithRotation(corner).WithScaleXYZ(0.4, 0.4, 0.4),
		MeshInstance{Mesh: assets.Cube},
		drop.glow(),
		NotShadowCaster{},
	)
}

func pickupSystem(
	vt VirtualTime,
	commands *Commands,
	arsenal *Arsenal,
	stats *Stats,
	budget *SoundBudget,
	assets *GameAssets,
	players Query[struct {
		Player    *Player
		Transform Transform
	}],
	pickups Query[struct {
		EntityId  EntityId
		Pickup    *Pickup
		Transform *Transform
	}],
) {
	player, ok := players.Single()
	if !ok {
		return
	}

	for item := range pickups.Items() {
		p := item.Pickup
		p.Age += vt.DeltaSecs

		if p.Age > pickupLifetime {
			commands.Entity(item.EntityId).Despawn()
			continue
		}

		// spin and bob
		item.Transform.Rotation = glm.RotationYQuat(glm.Rad(p.Age * 2))
		item.Transform.Translation[1] = p.Height + 0.15*sin32(p.Age*3)

		if p.Age < 0.3 || player.Player.Health <= 0 {
			continue
		}

		if flat(item.Transform.Translation.Sub(player.Transform.Translation)).Length() > pickupRange {
			continue
		}

		// no health while healthy, it stays for later
		if p.Kind == PickupHealth && player.Player.Health >= playerMaxHealth {
			continue
		}

		msg := Drop{Kind: p.Kind, Weapon: p.Weapon}.apply(arsenal, player.Player)
		stats.Show(vt, msg, 1200*time.Millisecond)
		playUI(commands, budget, assets.Sounds.Pickup, 0.6)
		commands.Entity(item.EntityId).Despawn()
	}
}

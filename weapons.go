package main

import (
	"fmt"
	"math"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/byke/byke2d/vyn"
	"github.com/oliverbestmann/zombies/physics"
)

type WeaponKind int

const (
	MachineGun WeaponKind = iota
	DualUzi
	Shotgun
	Gatling
	GrenadeLauncher
	RocketLauncher
	weaponCount
)

type ProjectileKind int

const (
	Hitscan ProjectileKind = iota
	GrenadeShot
	RocketShot
)

// WeaponDef describes a weapon.
type WeaponDef struct {
	Name  string
	Model string

	// Dual weapons fire alternately from two guns.
	Dual bool

	// Interval is the time between shots in seconds.
	Interval float32

	// SpinUp is the time until a gatling reaches full speed, it fires slower before.
	SpinUp float32

	// Damage per pellet.
	Damage  float32
	Pellets int

	// Spread is the random angle of each pellet in radians.
	Spread float32

	// Push is the velocity change of a hit zombie in m/s.
	Push float32

	Projectile ProjectileKind

	StartAmmo, MaxAmmo, PickupAmmo int

	Sound  func(s *Sounds) SoundSet
	Volume float32

	// Kick is the recoil in radians.
	Kick float32

	// ViewOffset places the model in the camera frame, ViewScale scales it.
	ViewOffset glm.Vec3f
	ViewScale  float32
}

var weapons = [weaponCount]WeaponDef{
	MachineGun: {
		Name: "MACHINE GUN", Model: "blaster/blaster-a",
		Interval: 0.09, Damage: 16, Pellets: 1, Spread: 0.015, Push: 3,
		StartAmmo: 400, MaxAmmo: 999, PickupAmmo: 150,
		Sound: func(s *Sounds) SoundSet { return s.ShotSmall }, Volume: 0.35, Kick: 0.012,
		ViewOffset: glm.Vec3f{0.28, -0.3, -0.55}, ViewScale: 0.6,
	},
	DualUzi: {
		Name: "DOUBLE UZI", Model: "blaster/blaster-c", Dual: true,
		Interval: 0.05, Damage: 11, Pellets: 1, Spread: 0.05, Push: 2,
		StartAmmo: 0, MaxAmmo: 999, PickupAmmo: 200,
		Sound: func(s *Sounds) SoundSet { return s.ShotRetro }, Volume: 0.25, Kick: 0.008,
		ViewOffset: glm.Vec3f{0.3, -0.28, -0.5}, ViewScale: 0.5,
	},
	Shotgun: {
		Name: "SHOTGUN", Model: "blaster/blaster-d",
		Interval: 0.7, Damage: 14, Pellets: 12, Spread: 0.09, Push: 8,
		StartAmmo: 0, MaxAmmo: 200, PickupAmmo: 30,
		Sound: func(s *Sounds) SoundSet { return s.ShotLarge }, Volume: 0.6, Kick: 0.08,
		ViewOffset: glm.Vec3f{0.28, -0.3, -0.6}, ViewScale: 0.65,
	},
	Gatling: {
		Name: "GATLING GUN", Model: "blaster/blaster-h",
		Interval: 0.022, SpinUp: 0.6, Damage: 13, Pellets: 1, Spread: 0.04, Push: 3,
		StartAmmo: 0, MaxAmmo: 1999, PickupAmmo: 500,
		Sound: func(s *Sounds) SoundSet { return s.ShotSmall }, Volume: 0.18, Kick: 0.006,
		ViewOffset: glm.Vec3f{0.3, -0.32, -0.6}, ViewScale: 0.75,
	},
	GrenadeLauncher: {
		Name: "GRENADE LAUNCHER", Model: "blaster/blaster-k",
		Interval: 0.45, Damage: 0, Pellets: 1, Spread: 0.01,
		Projectile: GrenadeShot,
		StartAmmo:  0, MaxAmmo: 150, PickupAmmo: 20,
		Sound: func(s *Sounds) SoundSet { return s.Launch }, Volume: 0.5, Kick: 0.05,
		ViewOffset: glm.Vec3f{0.3, -0.3, -0.6}, ViewScale: 0.65,
	},
	RocketLauncher: {
		Name: "ROCKET LAUNCHER", Model: "blaster/blaster-r",
		Interval: 0.55, Damage: 0, Pellets: 1, Spread: 0.005,
		Projectile: RocketShot,
		StartAmmo:  0, MaxAmmo: 100, PickupAmmo: 15,
		Sound: func(s *Sounds) SoundSet { return s.Launch }, Volume: 0.6, Kick: 0.06,
		ViewOffset: glm.Vec3f{0.3, -0.3, -0.55}, ViewScale: 0.7,
	},
}

// Arsenal holds the player's weapons and ammo.
type Arsenal struct {
	Owned   [weaponCount]bool
	Ammo    [weaponCount]int
	Current WeaponKind

	cooldown float32

	// spin of the gatling, 0 to 1
	spin float32

	// raise animation after switching, counts down from 1
	raise float32

	// recoil of the view model, decays
	recoil float32

	// for dual weapons, the gun that fires next
	side int
}

func newArsenal() Arsenal {
	var a Arsenal
	for kind, def := range weapons {
		a.Ammo[kind] = def.StartAmmo
	}

	a.Owned[MachineGun] = true
	a.Current = MachineGun
	a.raise = 1
	return a
}

func (a *Arsenal) hudText() string {
	def := weapons[a.Current]
	return fmt.Sprintf("%s  %d", def.Name, a.Ammo[a.Current])
}

// Give adds a weapon and its pickup ammo. It returns true if the weapon is new.
func (a *Arsenal) Give(kind WeaponKind) bool {
	isNew := !a.Owned[kind]
	a.Owned[kind] = true
	a.AddAmmo(kind, weapons[kind].PickupAmmo)
	return isNew
}

func (a *Arsenal) AddAmmo(kind WeaponKind, amount int) {
	a.Ammo[kind] = min(weapons[kind].MaxAmmo, a.Ammo[kind]+amount)
}

// Select switches to a weapon if it is owned.
func (a *Arsenal) Select(kind WeaponKind) {
	if kind < 0 || kind >= weaponCount || !a.Owned[kind] || kind == a.Current {
		return
	}

	a.Current = kind
	a.raise = 1
	a.spin = 0
	a.cooldown = max(a.cooldown, 0.15)
}

// Cycle selects the next owned weapon in the given direction.
func (a *Arsenal) Cycle(dir int) {
	for step := 1; step < int(weaponCount); step++ {
		kind := WeaponKind((int(a.Current) + dir*step + 2*int(weaponCount)) % int(weaponCount))
		if a.Owned[kind] && a.Ammo[kind] > 0 {
			a.Select(kind)
			return
		}
	}
}

// Trigger advances the weapon by dt while the trigger is held and returns how many shots fire.
func (a *Arsenal) Trigger(dt float32, held bool) int {
	def := weapons[a.Current]

	a.cooldown -= dt
	a.raise = max(0, a.raise-dt*4)

	if def.SpinUp > 0 {
		if held {
			a.spin = min(1, a.spin+dt/def.SpinUp)
		} else {
			a.spin = max(0, a.spin-dt/def.SpinUp)
		}
	}

	if !held || a.raise > 0.5 {
		a.cooldown = max(a.cooldown, 0)
		return 0
	}

	interval := def.Interval
	if def.SpinUp > 0 {
		// a gatling starts at a quarter of its full rate
		interval /= 0.25 + 0.75*a.spin
	}

	shots := 0
	for a.cooldown <= 0 && a.Ammo[a.Current] > 0 {
		a.cooldown += interval
		a.Ammo[a.Current]--
		shots++
	}

	a.cooldown = max(a.cooldown, -interval)
	return shots
}

// ViewModel is the gun in front of the camera.
type ViewModel struct {
	Component[ViewModel]
	Kind WeaponKind

	// Side is 0 for the right gun, 1 for the left gun of dual weapons.
	Side int
}

// MuzzleFlash is the light at the muzzle, it flashes when firing.
type MuzzleFlash struct {
	Component[MuzzleFlash]
	Time float32
}

func weaponsPlugin(app *App) {
	app.AddSystems(Update, System(weaponSelectSystem, fireSystem).Chain().RunIf(InState(GameStatePlaying)))
	app.AddSystems(PostUpdate, System(viewModelSystem, muzzleFlashSystem).InSet(beforeTransforms).After(cameraFollowSystem))
}

func weaponSelectSystem(keys Keys, arsenal *Arsenal) {
	digits := []vyn.Key{vyn.KeyDigit1, vyn.KeyDigit2, vyn.KeyDigit3, vyn.KeyDigit4, vyn.KeyDigit5, vyn.KeyDigit6}
	for idx, key := range digits {
		if keys.IsJustPressed(key) {
			arsenal.Select(WeaponKind(idx))
		}
	}

	if keys.IsJustPressed(vyn.KeyQ) {
		arsenal.Cycle(-1)
	}

	if keys.IsJustPressed(vyn.KeyE) || keys.IsJustPressed(vyn.KeyTab) {
		arsenal.Cycle(1)
	}

	// out of ammo, take the next weapon
	if arsenal.Ammo[arsenal.Current] == 0 {
		arsenal.Cycle(1)
	}
}

// spawnViewModels spawns the view models of all weapons below the camera, hidden until selected.
func spawnViewModels(commands *Commands, models *Models, camera EntityId) {
	for kind, def := range weapons {
		sides := 1
		if def.Dual {
			sides = 2
		}

		for side := range sides {
			model := models.Get(def.Model)

			gun := commands.Spawn(
				ChildOf{Parent: camera},
				NewTransform(),
				Visibility{},
				ViewModel{Kind: WeaponKind(kind), Side: side},
			).Id()

			commands.Spawn(
				ChildOf{Parent: gun},
				// the Kenney blasters point along +z, the camera looks along -z
				NewTransform().WithRotation(glm.RotationYQuat(math.Pi)),
				MeshInstance{Mesh: model.Merged},
				model.Material,
				NotShadowCaster{},
			)
		}

		// a light at the muzzle of each weapon
		commands.Spawn(
			ChildOf{Parent: camera},
			TransformFromXYZ(def.ViewOffset[0], def.ViewOffset[1]+0.1, def.ViewOffset[2]-0.5),
			MuzzleFlash{Time: 1},
			PointLight{Color: ColorSRGB(1, 0.7, 0.3), Intensity: 0, Range: 12},
		)
	}
}

// viewModelSystem shows the current weapon with bobbing, recoil and the raise animation.
func viewModelSystem(
	vt VirtualTime,
	arsenal *Arsenal,
	players Query[struct {
		Player         Player
		LinearVelocity physics.LinearVelocity
	}],
	guns Query[struct {
		ViewModel  ViewModel
		Transform  *Transform
		Visibility *Visibility
	}],
) {
	player, _ := players.Single()

	arsenal.recoil *= decay(15, vt.DeltaSecs)

	speed := flat(player.LinearVelocity.Vec3f).Length() / playerSpeed
	t := float32(vt.Elapsed.Seconds())
	bob := glm.Vec3f{sin32(t*9) * 0.015 * speed, float32(math.Abs(float64(sin32(t*9)))) * 0.02 * speed, 0}

	for gun := range guns.Items() {
		def := weapons[gun.ViewModel.Kind]

		if gun.ViewModel.Kind != arsenal.Current || player.Player.Driving != NoEntityId {
			gun.Visibility.SetInvisible()
			continue
		}

		gun.Visibility.SetInherit()

		offset := def.ViewOffset
		if gun.ViewModel.Side == 1 {
			offset[0] = -offset[0]
		}

		recoil := arsenal.recoil
		if def.Dual && gun.ViewModel.Side != arsenal.side {
			recoil *= 0.3
		}

		offset = offset.Add(bob).Add(glm.Vec3f{0, -arsenal.raise * 0.4, recoil * 0.15})

		rotation := glm.RotationXQuat(glm.Rad(recoil * 0.5))
		if def.SpinUp > 0 {
			// the whole gun rattles while spinning
			rotation = rotation.Mul(glm.RotationZQuat(glm.Rad(arsenal.spin * 0.03 * sin32(t*60))))
		}

		gun.Transform.Translation = offset
		gun.Transform.Rotation = rotation
		gun.Transform.Scale = glm.Vec3f{def.ViewScale, def.ViewScale, def.ViewScale}
	}
}

func muzzleFlashSystem(vt VirtualTime, flashes Query[struct {
	Flash *MuzzleFlash
	Light *PointLight
}],
) {
	for item := range flashes.Items() {
		item.Flash.Time += vt.DeltaSecs
		item.Light.Intensity = max(0, 1-item.Flash.Time/0.06) * 60000
	}
}

// fireSystem fires the current weapon while the left mouse button is held.
func fireSystem(
	vt VirtualTime,
	commands *Commands,
	buttons MouseButtons,
	cursor MouseCursor,
	keys Keys,
	world physics.PhysicsWorld,
	arsenal *Arsenal,
	rng *GameRng,
	budget *SoundBudget,
	assets *GameAssets,
	models *Models,
	testInput *TestInput,
	damages *MessageWriter[ZombieDamage],
	propHits *MessageWriter[PropHit],
	players Query[Player],
	cameras Query[struct {
		Camera    *PlayerCamera
		Transform Transform
	}],
	flashes Query[*MuzzleFlash],
) {
	player, ok := players.Single()
	if !ok || player.Health <= 0 || player.Driving != NoEntityId {
		return
	}

	camera, ok := cameras.Single()
	if !ok {
		return
	}

	held := (cursor.Grabbed && buttons.IsPressed(vyn.MouseButtonLeft)) || keys.IsPressed(vyn.KeyControlLeft) || testInput.Fire
	shots := arsenal.Trigger(vt.DeltaSecs, held)
	if shots == 0 {
		return
	}

	def := weapons[arsenal.Current]
	origin := camera.Transform.Translation
	rotation := camera.Transform.Rotation

	for range shots {
		arsenal.recoil = min(1, arsenal.recoil+0.5)
		camera.Camera.Kick += def.Kick
		arsenal.side = 1 - arsenal.side

		muzzleOffset := def.ViewOffset.Add(glm.Vec3f{0, 0.08, -0.45 * def.ViewScale})
		if def.Dual && arsenal.side == 1 {
			muzzleOffset[0] = -muzzleOffset[0]
		}

		muzzle := origin.Add(rotation.Transform(muzzleOffset))

		playUI(commands, budget, def.Sound(&assets.Sounds), def.Volume)

		for range def.Pellets {
			spread := glm.RotationYQuat(glm.Rad((rng.Float32()*2 - 1) * def.Spread)).
				Mul(glm.RotationXQuat(glm.Rad((rng.Float32()*2 - 1) * def.Spread)))
			dir := rotation.Mul(spread).Transform(glm.Vec3f{0, 0, -1})

			switch def.Projectile {
			case Hitscan:
				fireHitscan(commands, world, assets, rng, damages, propHits, def, origin, muzzle, dir)
			case GrenadeShot:
				spawnGrenade(commands, models, muzzle, dir)
			case RocketShot:
				spawnRocket(commands, assets, muzzle, dir)
			}
		}
	}

	for flash := range flashes.Items() {
		flash.Time = 0
	}
}

// hitscanMask are the things bullets hit.
const hitscanMask = layerWorld | layerProp | layerZombie | layerCar | layerFlyer

// PropHit tells that a bullet or explosion hit a prop, explosive barrels listen to it.
type PropHit struct {
	Entity EntityId
	Damage float32
}

func fireHitscan(
	commands *Commands,
	world physics.PhysicsWorld,
	assets *GameAssets,
	rng *GameRng,
	damages *MessageWriter[ZombieDamage],
	propHits *MessageWriter[PropHit],
	def WeaponDef,
	origin, muzzle, dir glm.Vec3f,
) {
	const maxRange = 250

	end := origin.Add(dir.Scale(maxRange))

	hit, ok := world.CastRayMask(origin, dir, maxRange, hitscanMask)
	if ok {
		end = hit.Point

		damages.Write(ZombieDamage{
			Zombie: hit.Entity,
			Amount: def.Damage,
			Push:   dir.Scale(def.Push),
			Point:  hit.Point,
		})

		propHits.Write(PropHit{Entity: hit.Entity, Damage: def.Damage})

		if hit.IsDynamic() {
			// bullets push loose things around, but never more than a few m/s
			impulse := min(hit.Mass()*def.Push*0.5, 40*def.Push)
			hit.ApplyImpulse(dir.Scale(impulse))
		}

		// a puff of dust
		for range 2 {
			spawnParticle(commands, assets.Cube, assets.DustMaterial, hit.Point, Particle{
				Velocity:   hit.Normal.Scale(2).Add(randomInSphere(rng.Rand).Scale(1.5)),
				Gravity:    4,
				Drag:       3,
				Life:       0.35,
				StartScale: glm.Vec3f{0.12, 0.12, 0.12},
				EndScale:   glm.Vec3f{0.02, 0.02, 0.02},
			})
		}
	}

	spawnTracer(commands, assets, muzzle, end)
}

// spawnTracer draws a short glowing line from the muzzle towards the hit point.
func spawnTracer(commands *Commands, assets *GameAssets, from, to glm.Vec3f) {
	dir := to.Sub(from)
	length := dir.Length()
	if length < 0.5 {
		return
	}

	dir = dir.Scale(1 / length)

	// a streak that flies along the bullet path
	streak := min(length, 6)
	rotation := glm.QuatFromRotationArc(glm.Vec3f{0, 0, 1}, dir)

	commands.Spawn(
		Round{},
		NewTransform().
			WithTranslationXYZ(from.Add(dir.Scale(streak/2)).XYZ()).
			WithRotation(rotation).
			WithScaleXYZ(0.025, 0.025, streak),
		MeshInstance{Mesh: assets.Cube},
		assets.TracerMaterial,
		NotShadowCaster{},
		Particle{
			Velocity:   dir.Scale(min(400, length*20)),
			Life:       min(0.06, length/400),
			StartScale: glm.Vec3f{0.025, 0.025, streak},
			EndScale:   glm.Vec3f{0.01, 0.01, streak},
		},
	)
}

// TestInput fakes player input, for screenshots.
type TestInput struct {
	// Fire holds the trigger.
	Fire bool

	// Drive gets into the car and drives in circles.
	Drive bool
}

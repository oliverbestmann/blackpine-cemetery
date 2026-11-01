package main

import (
	"math"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/byke/byke2d/vyn"
	"github.com/oliverbestmann/zombies/physics"
)

const (
	carModel = "car/police"

	carMaxReverse = 10
	carBraking    = 35

	// the player enters the car within this distance
	carEnterRange = 4.5

	// zombies in front of a car faster than this get run over
	carHitSpeed = 4

	// zombies claw at the car from its hull, a bit further away than at the player
	carZombieReach = 1.8

	carRespawnSecs = 15
)

var carStart = glm.Vec3f{7, 0, 14}

// CarKind holds the driving stats of a car model.
type CarKind struct {
	MaxHealth    float32
	Acceleration float32
	MaxSpeed     float32
	TurnRate     float32
}

var defaultCarKind = CarKind{MaxHealth: 300, Acceleration: 14, MaxSpeed: 30, TurnRate: 2.2}

// heavy vehicles take more damage but are slow
var carKinds = map[string]CarKind{
	"car/firetruck":     {MaxHealth: 700, Acceleration: 6, MaxSpeed: 22, TurnRate: 1.5},
	"car/garbage-truck": {MaxHealth: 600, Acceleration: 7, MaxSpeed: 20, TurnRate: 1.5},
	"car/ambulance":     {MaxHealth: 400, Acceleration: 11, MaxSpeed: 27, TurnRate: 1.9},
}

var carDebris = []string{
	"car/debris-bolt", "car/debris-bumper", "car/debris-door", "car/debris-door-window",
	"car/debris-drivetrain", "car/debris-drivetrain-axle", "car/debris-nut", "car/debris-plate-a",
	"car/debris-plate-b", "car/debris-plate-small-a", "car/debris-plate-small-b",
	"car/debris-spoiler-a", "car/debris-spoiler-b", "car/debris-tire",
}

// Car is the drivable car.
type Car struct {
	Component[Car]

	// Upside counts how long the car lies on its side or roof.
	Upside float32

	// Engine is the looping engine sound while someone drives.
	Engine EntityId

	Health float32

	// Wrecked counts down to the respawn after the car blew up.
	Wrecked float32

	// Smoke counts down to the next smoke puff of a damaged car.
	Smoke float32

	Kind CarKind

	// HitRange is the distance from the center to the front bumper.
	HitRange float32

	// Home is where the car respawns.
	Home Transform
}

// spawnCar spawns a drivable car of the given model.
func spawnCar(commands *Commands, models *Models, name string, transform Transform) {
	kind, ok := carKinds[name]
	if !ok {
		kind = defaultCarKind
	}

	model := models.Get(name)
	car := Car{
		Health:   kind.MaxHealth,
		Kind:     kind,
		HitRange: max(-model.Min[2], model.Max[2])*transform.Scale[2] + 0.4,
		Home:     transform,
	}

	spawnProp(commands, model, transform, PropSettings{Dynamic: true, Density: 150, Slices: 3}, car)
}

func carPlugin(app *App) {
	app.AddSystems(Update, System(enterCarSystem, driveCarSystem, runOverSystem, carDamageSystem).Chain().RunIf(InState(GameStatePlaying)))
	app.AddSystems(PostUpdate, System(carCameraSystem).InSet(beforeTransforms).RunIf(InState(GameStatePlaying)))
}

func spawnCarSystem(commands *Commands, models *Models) {
	s := float32(scaleCar)
	transform := TransformFromXYZ(carStart.XYZ()).
		WithRotation(glm.RotationYQuat(glm.DegToRad(-30))).
		WithScaleXYZ(s, s, s)

	spawnCar(commands, models, carModel, transform)

	// the firetruck waits on the other side of the start
	spawnCar(commands, models, "car/firetruck", TransformFromXYZ(-9, 0, 16).WithRotation(glm.RotationYQuat(glm.DegToRad(25))).WithScaleXYZ(s, s, s))
}

// carForward is the direction the car drives to, flat on the ground. The Kenney cars face +z.
func carForward(rotation glm.Quat) glm.Vec3f {
	f := flat(rotation.Transform(glm.Vec3f{0, 0, 1}))
	if f.Length() < 0.01 {
		return glm.Vec3f{0, 0, 1}
	}

	return f.Normalize()
}

func enterCarSystem(
	keys Keys,
	testInput *TestInput,
	commands *Commands,
	assets *GameAssets,
	players Query[struct {
		EntityId  EntityId
		Player    *Player
		Transform Transform
	}],
	cars Query[struct {
		EntityId  EntityId
		Car       *Car
		Transform Transform
	}],
) {
	if !keys.IsJustPressed(vyn.KeyF) && !testInput.Drive {
		return
	}

	player, ok := players.Single()
	if !ok || player.Player.Health <= 0 {
		return
	}

	if player.Player.Driving != NoEntityId {
		car, ok := cars.Get(player.Player.Driving)
		if !ok || testInput.Drive {
			return
		}

		exitCar(commands, player.EntityId, player.Player, car.EntityId, car.Car, car.Transform)
		return
	}

	// the closest car in reach
	target, bestDist := NoEntityId, float32(carEnterRange)
	for item := range cars.Items() {
		dist := flat(item.Transform.Translation.Sub(player.Transform.Translation)).Length()
		if testInput.Drive {
			// the test drive takes the police car at the start
			dist = item.Transform.Translation.Sub(carStart).Length()
		}

		if dist < bestDist && item.Car.Health > 0 {
			target, bestDist = item.EntityId, dist
		}
	}

	car, ok := cars.Get(target)
	if !ok {
		return
	}

	player.Player.Driving = car.EntityId
	commands.Entity(player.EntityId).Insert(physics.Disabled{})
	commands.Entity(car.EntityId).Insert(physics.ControlledVelocity{Angular: true})

	if len(assets.Sounds.Engine) > 0 {
		car.Car.Engine = commands.Spawn(
			ChildOf{Parent: car.EntityId},
			NewTransform(),
			AudioPlayerOf(assets.Sounds.Engine[0]),
			PlaybackSettingsLoop.WithVolume(0.5).WithSpatial(),
		).Id()
	}
}

// exitCar puts the player next to the car and stops the engine.
func exitCar(commands *Commands, playerId EntityId, player *Player, carId EntityId, car *Car, transform Transform) {
	// get out on the left side
	forward := carForward(transform.Rotation)
	side := glm.Vec3f{forward[2], 0, -forward[0]}
	exit := transform.Translation.Add(side.Scale(2.6))
	exit[1] = max(exit[1], playerHalfHeight+0.1)

	player.Driving = NoEntityId
	// the camera looks along -z
	player.Yaw = yawTowards(forward) + math.Pi
	player.previous = exit
	player.current = exit

	commands.Entity(playerId).Remove[physics.Disabled]().Insert(physics.Teleport{Position: exit})
	commands.Entity(carId).Remove[physics.ControlledVelocity]()

	if car.Engine != NoEntityId {
		commands.Entity(car.Engine).Despawn()
		car.Engine = NoEntityId
	}
}

// carDamageSystem blows up the car when its health runs out, lets a damaged car smoke
// and puts a wreck back to the start after a while.
func carDamageSystem(
	vt VirtualTime,
	commands *Commands,
	assets *GameAssets,
	models *Models,
	rng *GameRng,
	explosions *MessageWriter[Explosion],
	players Query[struct {
		EntityId EntityId
		Player   *Player
	}],
	cars Query[struct {
		EntityId  EntityId
		Car       *Car
		Transform Transform
	}],
) {
	for car := range cars.Items() {
		c := car.Car
		pos := car.Transform.Translation

		if c.Wrecked > 0 {
			c.Wrecked -= vt.DeltaSecs
			if c.Wrecked <= 0 {
				c.Health = c.Kind.MaxHealth
				commands.Entity(car.EntityId).Insert(physics.Teleport{
					Position: c.Home.Translation.Add(glm.Vec3f{0, 1, 0}),
					Rotation: c.Home.Rotation,
					Rotate:   true,
				})
			}
		} else if c.Health <= 0 {
			c.Wrecked = carRespawnSecs

			if player, ok := players.Single(); ok && player.Player.Driving == car.EntityId {
				// thrown out, the explosion hurts him as he is now outside
				exitCar(commands, player.EntityId, player.Player, car.EntityId, c, car.Transform)
			}

			// the blast pushes the wreck too
			explosions.Write(Explosion{Position: pos, Radius: 10, Damage: 150})

			// parts fly off
			for range 8 {
				at := car.Transform
				at.Translation = pos.Add(randomInSphere(rng.Rand).Scale(1.5)).Add(glm.Vec3f{0, 1, 0})
				at.Rotation = randomYaw(rng.Rand)
				spawnGibs(commands, rng, models.Get(carDebris[rng.IntN(len(carDebris))]), at, glm.Vec3f{0, 4, 0}, 9)
			}
		}

		if c.Health > c.Kind.MaxHealth*0.4 {
			continue
		}

		// smoke gets thicker with damage, the wreck smokes the most
		c.Smoke -= vt.DeltaSecs
		if c.Smoke > 0 {
			continue
		}

		c.Smoke = 0.05 + 0.25*max(0, c.Health)/(c.Kind.MaxHealth*0.4)
		s := 0.4 + 0.4*rng.Float32()
		spawnParticle(commands, assets.Sphere, assets.SmokeMaterial, pos.Add(glm.Vec3f{0, 1.2, 0}).Add(randomInSphere(rng.Rand).Scale(0.4)), Particle{
			Velocity:   randomInSphere(rng.Rand).Add(glm.Vec3f{0, 2.5, 0}),
			Gravity:    -1,
			Drag:       1,
			Life:       1.2 + rng.Float32()*0.6,
			StartScale: glm.Vec3f{s, s, s},
			EndScale:   glm.Vec3f{s, s, s}.Scale(2),
		})
	}
}

// carVelocity applies the driver's input to the car velocity, it returns the new velocity and yaw rate.
func carVelocity(kind CarKind, velocity glm.Vec3f, forward glm.Vec3f, throttle, steer, dt float32) (glm.Vec3f, float32) {
	speed := velocity.Dot(forward)

	switch {
	case throttle > 0 && speed < 0, throttle < 0 && speed > 0:
		// braking
		speed += throttle * carBraking * dt
	case throttle != 0:
		speed += throttle * kind.Acceleration * dt
	default:
		speed *= decay(0.6, dt)
	}

	speed = clamp(speed, -carMaxReverse, kind.MaxSpeed)

	// the tires grip, sideways sliding fades out quickly
	lateral := flat(velocity).Sub(forward.Scale(velocity.Dot(forward))).Scale(decay(6, dt))

	result := forward.Scale(speed).Add(lateral)
	result[1] = velocity[1]

	// turning needs speed, and reverses when driving backwards
	yawRate := steer * kind.TurnRate * clamp(speed/8, -1, 1)

	return result, yawRate
}

func driveCarSystem(
	vt VirtualTime,
	keys Keys,
	testInput *TestInput,
	commands *Commands,
	world physics.PhysicsWorld,
	players Query[struct {
		Player    *Player
		Transform *Transform
	}],
	cars Query[struct {
		EntityId        EntityId
		Car             *Car
		Transform       Transform
		LinearVelocity  *physics.LinearVelocity
		AngularVelocity *physics.AngularVelocity
	}],
) {
	player, ok := players.Single()
	if !ok || player.Player.Driving == NoEntityId {
		return
	}

	car, ok := cars.Get(player.Player.Driving)
	if !ok {
		return
	}

	dt := vt.DeltaSecs

	// the player rides along, zombies chase him
	player.Transform.Translation = car.Transform.Translation.Add(glm.Vec3f{0, 1, 0})
	player.Player.previous = player.Transform.Translation
	player.Player.current = player.Transform.Translation

	up := car.Transform.Rotation.Transform(glm.Vec3f{0, 1, 0})
	if up[1] < 0.4 {
		car.Car.Upside += dt
	} else {
		car.Car.Upside = 0
	}

	// put it back on its wheels after a while
	if car.Car.Upside > 1.5 {
		car.Car.Upside = 0
		commands.Entity(car.EntityId).Insert(physics.Teleport{
			Position: car.Transform.Translation.Add(glm.Vec3f{0, 1.5, 0}),
			Rotation: glm.RotationYQuat(yawTowards(carForward(car.Transform.Rotation))),
			Rotate:   true,
		})

		return
	}

	_, grounded := world.CastRayMask(car.Transform.Translation.Add(glm.Vec3f{0, 1, 0}), glm.Vec3f{0, -1, 0}, 2, layerWorld|layerProp)
	if !grounded || up[1] < 0.7 {
		return
	}

	var throttle, steer float32
	if keys.IsPressed(vyn.KeyW) || keys.IsPressed(vyn.KeyArrowUp) {
		throttle += 1
	}
	if keys.IsPressed(vyn.KeyS) || keys.IsPressed(vyn.KeyArrowDown) {
		throttle -= 1
	}
	if keys.IsPressed(vyn.KeyA) || keys.IsPressed(vyn.KeyArrowLeft) {
		steer += 1
	}
	if keys.IsPressed(vyn.KeyD) || keys.IsPressed(vyn.KeyArrowRight) {
		steer -= 1
	}

	if testInput.Drive {
		throttle, steer = 1, 0.15
	}

	velocity, yawRate := carVelocity(car.Car.Kind, car.LinearVelocity.Vec3f, carForward(car.Transform.Rotation), throttle, steer, dt)
	car.LinearVelocity.Vec3f = velocity
	// steer the yaw only, keep roll and pitch from collisions so the car can tip over
	car.AngularVelocity.Vec3f[1] = yawRate
}

// runOverSystem kills zombies in front of a fast car.
func runOverSystem(
	commands *Commands,
	budget *SoundBudget,
	assets *GameAssets,
	damages *MessageWriter[ZombieDamage],
	cars Query[struct {
		Car            Car
		Transform      Transform
		LinearVelocity physics.LinearVelocity
	}],
	zombies Query[struct {
		EntityId  EntityId
		Zombie    Zombie
		Transform Transform
	}],
	explosives Query[struct {
		Explosive *Explosive
		Transform Transform
	}],
	breakables Query[struct {
		EntityId  EntityId
		Breakable Breakable
		Transform Transform
	}],
) {
	for car := range cars.Items() {
		velocity := car.LinearVelocity.Vec3f
		speed := flat(velocity).Length()
		if speed < carHitSpeed {
			continue
		}

		pos := car.Transform.Translation

		// rammed barrels blow up, the explosion damages the car
		for e := range explosives.Items() {
			offset := e.Transform.Translation.Sub(pos)
			if flat(offset).Length() < car.Car.HitRange+0.6 && offset.Dot(velocity) > 0 && e.Explosive.Health < 1e8 {
				e.Explosive.Health = 0
			}
		}

		// heavy trucks plow through walls and trees. A bit of extra range, so they break
		// before the static body stops the truck.
		if car.Car.Kind.MaxHealth >= 600 && speed > 2*carHitSpeed {
			for b := range breakables.Items() {
				offset := b.Transform.Translation.Sub(pos)
				if flat(offset).Length() < car.Car.HitRange+1.5 && offset.Dot(velocity) > 0 {
					breakLoose(commands, b.EntityId, b.Breakable, b.Transform, velocity.Scale(0.6), glm.Vec3f{0, 1, 0}.Cross(velocity).Scale(0.1))
				}
			}
		}

		for z := range zombies.Items() {
			offset := z.Transform.Translation.Sub(pos)
			if z.Zombie.Dead || abs32(offset[1]) > 2.5 || flat(offset).Length() > car.Car.HitRange {
				continue
			}

			// only what is in front of the car, not what it drives away from
			if offset.Dot(velocity) < 0 {
				continue
			}

			damages.Write(ZombieDamage{
				Zombie:    z.EntityId,
				Amount:    speed * 12,
				Push:      velocity.Scale(0.9).Add(glm.Vec3f{0, 4 + speed*0.3, 0}),
				Point:     z.Transform.Translation,
				Explosive: speed > 15,
				Source:    SourceCar,
			})

			playAt(commands, budget, assets.Sounds.Metal, z.Transform.Translation, 0.6)
		}
	}
}

// carCameraSystem looks at the car from behind while driving.
func carCameraSystem(
	vt VirtualTime,
	players Query[Player],
	cars Query[struct {
		_         With[Car]
		Transform Transform
	}],
	cameras Query[struct {
		Camera    *PlayerCamera
		Transform *Transform
	}],
) {
	player, ok := players.Single()
	if !ok || player.Driving == NoEntityId {
		return
	}

	car, ok := cars.Get(player.Driving)
	if !ok {
		return
	}

	camera, ok := cameras.Single()
	if !ok {
		return
	}

	forward := carForward(car.Transform.Rotation)
	target := car.Transform.Translation.Add(glm.Vec3f{0, 1.5, 0})
	goal := target.Sub(forward.Scale(9)).Add(glm.Vec3f{0, 3, 0})

	// smooth, the car moves in fixed steps
	t := 1 - decay(8, vt.DeltaSecs)
	position := camera.Transform.Translation.Add(goal.Sub(camera.Transform.Translation).Scale(t))

	camera.Camera.Shake *= decay(6, vt.DeltaSecs)

	camera.Transform.Translation = position
	camera.Transform.Rotation = glm.RotationYQuat(yawTowards(target.Sub(position)) + math.Pi).
		Mul(glm.RotationXQuat(glm.Rad(-0.25)))
}

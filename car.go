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

	carMaxSpeed     = 30
	carMaxReverse   = 10
	carAcceleration = 14
	carBraking      = 35
	carTurnRate     = 2.2

	// the player enters the car within this distance
	carEnterRange = 4.5

	// zombies within this distance of the car center get run over
	carHitRange = 2.6
	carHitSpeed = 4
)

var carStart = glm.Vec3f{7, 0, 14}

// Car is the drivable car.
type Car struct {
	Component[Car]

	// Upside counts how long the car lies on its side or roof.
	Upside float32

	// Engine is the looping engine sound while someone drives.
	Engine EntityId
}

func carPlugin(app *App) {
	app.AddSystems(Update, System(enterCarSystem, driveCarSystem, runOverSystem).Chain().RunIf(InState(GameStatePlaying)))
	app.AddSystems(PostUpdate, System(carCameraSystem).InSet(beforeTransforms).RunIf(InState(GameStatePlaying)))
}

func spawnCarSystem(commands *Commands, models *Models) {
	s := float32(scaleCar)
	transform := TransformFromXYZ(carStart.XYZ()).
		WithRotation(glm.RotationYQuat(glm.DegToRad(-30))).
		WithScaleXYZ(s, s, s)

	spawnProp(commands, models.Get(carModel), transform, PropSettings{Dynamic: true, Density: 150, Slices: 3}, Car{})
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

		// get out on the left side
		forward := carForward(car.Transform.Rotation)
		side := glm.Vec3f{forward[2], 0, -forward[0]}
		exit := car.Transform.Translation.Add(side.Scale(2.6))
		exit[1] = max(exit[1], playerHalfHeight+0.1)

		player.Player.Driving = NoEntityId
		// the camera looks along -z
		player.Player.Yaw = yawTowards(forward) + math.Pi
		player.Player.previous = exit
		player.Player.current = exit

		commands.Entity(player.EntityId).Remove[physics.Disabled]().Insert(physics.Teleport{Position: exit})
		commands.Entity(car.EntityId).Remove[physics.ControlledVelocity]()

		if car.Car.Engine != NoEntityId {
			commands.Entity(car.Car.Engine).Despawn()
			car.Car.Engine = NoEntityId
		}

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

		if dist < bestDist {
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

// carVelocity applies the driver's input to the car velocity, it returns the new velocity and yaw rate.
func carVelocity(velocity glm.Vec3f, forward glm.Vec3f, throttle, steer, dt float32) (glm.Vec3f, float32) {
	speed := velocity.Dot(forward)

	switch {
	case throttle > 0 && speed < 0, throttle < 0 && speed > 0:
		// braking
		speed += throttle * carBraking * dt
	case throttle != 0:
		speed += throttle * carAcceleration * dt
	default:
		speed *= decay(0.6, dt)
	}

	speed = clamp(speed, -carMaxReverse, carMaxSpeed)

	// the tires grip, sideways sliding fades out quickly
	lateral := flat(velocity).Sub(forward.Scale(velocity.Dot(forward))).Scale(decay(6, dt))

	result := forward.Scale(speed).Add(lateral)
	result[1] = velocity[1]

	// turning needs speed, and reverses when driving backwards
	yawRate := steer * carTurnRate * clamp(speed/8, -1, 1)

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

	velocity, yawRate := carVelocity(car.LinearVelocity.Vec3f, carForward(car.Transform.Rotation), throttle, steer, dt)
	car.LinearVelocity.Vec3f = velocity
	car.AngularVelocity.Vec3f = glm.Vec3f{0, yawRate, 0}
}

// runOverSystem kills zombies in front of a fast car.
func runOverSystem(
	commands *Commands,
	budget *SoundBudget,
	assets *GameAssets,
	damages *MessageWriter[ZombieDamage],
	cars Query[struct {
		_              With[Car]
		Transform      Transform
		LinearVelocity physics.LinearVelocity
	}],
	zombies Query[struct {
		EntityId  EntityId
		Zombie    Zombie
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

		for z := range zombies.Items() {
			offset := z.Transform.Translation.Sub(pos)
			if z.Zombie.Dead || abs32(offset[1]) > 2.5 || flat(offset).Length() > carHitRange {
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

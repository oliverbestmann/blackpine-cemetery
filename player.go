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
	playerRadius = 0.35

	// distance between the two sphere centers of the capsule, the player is 1.8 m tall
	playerCapsuleLength = 1.1
	playerHalfHeight    = playerCapsuleLength/2 + playerRadius

	// eye height above the capsule center
	playerEyeOffset = 0.7

	playerSpeed     = 9
	playerJumpSpeed = 6
	playerMaxHealth = 100

	mouseSensitivity = 0.0025
)

// Player is the body of the player, a capsule moved by physics.
type Player struct {
	Component[Player]

	Yaw, Pitch glm.Rad
	Health     float32

	lastCursor *glm.Vec2f

	// physics positions of the last two fixed steps, the camera interpolates between them
	previous, current glm.Vec3f

	// Grounded is true if the player stands on something.
	Grounded bool

	// Damaged counts down after taking damage, for the red screen flash.
	Damaged float32

	// Driving is the car the player sits in, if any.
	Driving EntityId
}

// PlayerCamera is the camera, it follows the player.
type PlayerCamera struct {
	Component[PlayerCamera]

	// Shake is the current shake strength, it decays over time.
	Shake float32

	// Kick is the recoil pitch offset in radians, it decays over time.
	Kick float32
}

// afterWriteBack runs in FixedPostUpdate after physics wrote back the poses.
var afterWriteBack = (&SystemSet{Name: "AfterWriteBack"}).After(physics.SystemsWriteBack)

func playerPlugin(app *App) {
	app.AddSystems(Update, System(lookSystem, moveSystem).Chain().RunIf(InState(GameStatePlaying)))
	app.AddSystems(FixedPostUpdate, System(trackPlayerSystem).InSet(afterWriteBack))
	app.AddSystems(PostUpdate, System(cameraFollowSystem).InSet(beforeTransforms))
}

var playerStart = glm.Vec3f{0, playerHalfHeight + 0.05, 10}

func spawnPlayerSystem(commands *Commands, assets *GameAssets, models *Models) {
	commands.Spawn(
		Round{},
		Player{Health: playerMaxHealth, previous: playerStart, current: playerStart},
		TransformFromXYZ(playerStart.XYZ()),
		physics.RigidBody{Type: physics.Dynamic, LockRotation: true, Friction: 0.01, Density: 400},
		physics.ControlledVelocity{},
		physics.CollisionLayers{Category: layerPlayer, Mask: layerWorld | layerProp | layerZombie | layerCar},
		physics.CapsuleCollider(playerRadius, playerCapsuleLength),
	)

	camera := commands.Spawn(
		Round{},
		PlayerCamera{},
		// the view models are children, they need a visible root
		InheritVisibility,
		Camera{},
		Camera3d,
		HDR{},
		BloomNatural,
		Skybox{Texture: assets.Skybox, Intensity: ColorLinearRGB(skyBrightness, skyBrightness, skyBrightness)},
		EnvironmentMapLight{
			DiffuseMap:  assets.Diffuse,
			SpecularMap: assets.Specular,
			Intensity:   environmentIntensity,
		},
		microphone,
		PerspectiveProjection{Fov: glm.DegToRad(80), Near: 0.05, Far: 1000},
		TransformFromXYZ(playerStart.Add(glm.Vec3f{0, playerEyeOffset, 0}).XYZ()),
	).Id()

	spawnViewModels(commands, models, assets, camera)
}

const (
	skyBrightness        = 1
	environmentIntensity = 1000
)

func lookSystem(
	keys Keys,
	buttons MouseButtons,
	cursor MouseCursor,
	win *PrimaryWindow,
	vt VirtualTime,
	players Query[*Player],
) {
	if !cursor.Grabbed && buttons.IsJustPressed(vyn.MouseButtonLeft) {
		win.SetCursorGrab(true)
	}

	for player := range players.Items() {
		var delta glm.Vec2f
		if last := player.lastCursor; last != nil {
			delta = cursor.Vec2f.Sub(*last)
		}

		player.lastCursor = new(cursor.Vec2f)

		if cursor.Grabbed {
			player.Yaw -= glm.Rad(delta[0] * mouseSensitivity)
			player.Pitch -= glm.Rad(delta[1] * mouseSensitivity)
		}

		turn := glm.Rad(2 * vt.DeltaSecs)
		if keys.IsPressed(vyn.KeyArrowLeft) {
			player.Yaw += turn
		}
		if keys.IsPressed(vyn.KeyArrowRight) {
			player.Yaw -= turn
		}
		if keys.IsPressed(vyn.KeyArrowUp) {
			player.Pitch += turn
		}
		if keys.IsPressed(vyn.KeyArrowDown) {
			player.Pitch -= turn
		}

		player.Pitch = max(min(player.Pitch, glm.DegToRad(89)), glm.DegToRad(-89))
	}
}

// moveInput returns the walk direction in the player's local frame, x to the right and z backwards.
func moveInput(keys Keys) glm.Vec3f {
	var move glm.Vec3f

	if keys.IsPressed(vyn.KeyW) {
		move[2] -= 1
	}
	if keys.IsPressed(vyn.KeyS) {
		move[2] += 1
	}
	if keys.IsPressed(vyn.KeyA) {
		move[0] -= 1
	}
	if keys.IsPressed(vyn.KeyD) {
		move[0] += 1
	}

	if move.Length() > 0 {
		move = move.Normalize()
	}

	return move
}

// walkVelocity turns the local walk direction into a world velocity for the given yaw.
func walkVelocity(move glm.Vec3f, yaw glm.Rad) glm.Vec3f {
	return glm.RotationYQuat(yaw).Transform(move).Scale(playerSpeed)
}

func moveSystem(
	keys Keys,
	world physics.PhysicsWorld,
	players Query[struct {
		Player         *Player
		Transform      Transform
		LinearVelocity *physics.LinearVelocity
	}],
) {
	for item := range players.Items() {
		if item.Player.Driving != NoEntityId {
			continue
		}

		// stands on something if a ray from the capsule center reaches the ground just below the capsule
		origin := item.Transform.Translation
		_, grounded := world.CastRayMask(origin, glm.Vec3f{0, -1, 0}, playerHalfHeight+0.15, layerWorld|layerProp|layerCar|layerZombie)
		item.Player.Grounded = grounded

		velocity := walkVelocity(moveInput(keys), item.Player.Yaw)
		velocity[1] = item.LinearVelocity.Vec3f[1]

		if grounded && keys.IsJustPressed(vyn.KeySpace) {
			velocity[1] = playerJumpSpeed
		}

		item.LinearVelocity.Vec3f = velocity
	}
}

// trackPlayerSystem records the physics position after each step, for interpolation.
func trackPlayerSystem(players Query[struct {
	Player    *Player
	Transform Transform
}],
) {
	for item := range players.Items() {
		item.Player.previous = item.Player.current
		item.Player.current = item.Transform.Translation
	}
}

// cameraFollowSystem puts the camera at the player's eyes, between the last two physics steps.
func cameraFollowSystem(
	vt VirtualTime,
	fixed FixedTime,
	players Query[*Player],
	cameras Query[struct {
		Camera    *PlayerCamera
		Transform *Transform
	}],
) {
	player, ok := players.Single()
	if !ok || player.Driving != NoEntityId {
		return
	}

	camera, ok := cameras.Single()
	if !ok {
		return
	}

	// the time since the last fixed step, as share of a step
	alpha := float32((vt.Elapsed - fixed.Elapsed).Seconds() / fixed.StepInterval.Seconds())
	alpha = max(0, min(1, alpha))

	position := player.previous.Add(player.current.Sub(player.previous).Scale(alpha))
	position = position.Add(glm.Vec3f{0, playerEyeOffset, 0})

	// shake and recoil
	camera.Camera.Shake *= float32(math.Exp(float64(-6 * vt.DeltaSecs)))
	camera.Camera.Kick *= float32(math.Exp(float64(-12 * vt.DeltaSecs)))

	t := float64(vt.Elapsed.Seconds())
	shake := camera.Camera.Shake
	shakeYaw := glm.Rad(shake * 0.04 * float32(math.Sin(t*41)))
	shakePitch := glm.Rad(shake * 0.04 * float32(math.Sin(t*37+1)))

	rotation := glm.RotationYQuat(player.Yaw + shakeYaw).
		Mul(glm.RotationXQuat(player.Pitch + glm.Rad(camera.Camera.Kick) + shakePitch))

	// dead: lying on the ground, looking at the feet of the crowd
	if player.Health <= 0 {
		position[1] = 0.3
		rotation = rotation.Mul(glm.RotationZQuat(glm.Rad(1.2)))
	}

	camera.Transform.Translation = position
	camera.Transform.Rotation = rotation
}

// forward returns the direction the camera looks at.
func cameraForward(transform Transform) glm.Vec3f {
	return transform.Rotation.Transform(glm.Vec3f{0, 0, -1})
}

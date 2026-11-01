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

	// distance between the two sphere centers of the capsule. The capsule floats on the pogo
	// of the mover, its bottom is 0.3 m above the ground, so the player walks up steps.
	playerCapsuleLength = 0.5
	playerPogoLength    = 0.65

	// height of the capsule center above the ground
	playerHalfHeight = playerPogoLength + playerCapsuleLength/2

	// eye height above the capsule center
	playerEyeOffset = 0.7

	playerSpeed     = 9
	playerJumpSpeed = 7
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

	// DeadFor counts the seconds since the player died.
	DeadFor float32

	// deathEye is the eye position at the moment of death, the camera stays there.
	deathEye glm.Vec3f
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
	app.AddSystems(Update, System(lookSystem).RunIf(InState(GameStatePlaying)))

	// also runs after death, to stop the player
	app.AddSystems(Update, System(moveSystem).After(lookSystem))
	app.AddSystems(FixedPostUpdate, System(trackPlayerSystem).InSet(afterWriteBack))
	app.AddSystems(PostUpdate, System(cameraFollowSystem).InSet(beforeTransforms))
}

var playerStart = glm.Vec3f{0, playerHalfHeight + 0.05, 10}

func spawnPlayerSystem(commands *Commands, assets *GameAssets, models *Models) {
	commands.Spawn(
		Round{},
		Player{Health: playerMaxHealth, previous: playerStart, current: playerStart},
		TransformFromXYZ(playerStart.XYZ()),
		physics.RigidBody{Type: physics.Dynamic, LockRotation: true, Density: 200},
		physics.Mover{MoverSettings: playerMoverSettings()},
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
		gameBloom,
		gameColorGrading,
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

	// draws only the view models on top of the world, so they never clip into walls
	commands.Spawn(
		ChildOf{Parent: camera},
		NewTransform(),
		Camera{Order: 1},
		Camera3d,
		HDR{},
		gameColorGrading,
		ClearColor{Color: ColorSRGBA(0, 0, 0, 0)},
		RenderLayersOf(viewModelLayer),
		EnvironmentMapLight{
			DiffuseMap:  assets.Diffuse,
			SpecularMap: assets.Specular,
			Intensity:   environmentIntensity,
		},
		PerspectiveProjection{Fov: glm.DegToRad(80), Near: 0.01, Far: 10},
	)

	spawnViewModels(commands, models, assets, camera)
}

const viewModelLayer = 2

// gameBloom is stronger than BloomNatural, so explosions and muzzle flashes glow.
var gameBloom = func() Bloom {
	b := BloomNatural
	b.Intensity = 0.3
	return b
}()

// gameColorGrading is a slightly washed out dusk look. Both 3d cameras use it,
// so the weapon matches the world.
// Temperature and Tint stay zero: byke uploads the white balance matrix with the wrong
// padding (wgsl.StructWriter.appendValue), any other value turns the image red.
var gameColorGrading = func() ColorGrading {
	g := DefaultColorGrading
	g.Global.PostSaturation = 0.85
	g.Shadows.Saturation = 0.8
	g.Shadows.Lift = 0.01
	g.Highlights.Gain = 1.05
	return g
}()

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

// walkDirection turns the local walk direction into a world direction for the given yaw.
func walkDirection(move glm.Vec3f, yaw glm.Rad) glm.Vec3f {
	return glm.RotationYQuat(yaw).Transform(move)
}

func playerMoverSettings() physics.MoverSettings {
	settings := physics.DefaultMoverSettings()
	settings.MaxSpeed = playerSpeed
	settings.JumpSpeed = playerJumpSpeed
	settings.PogoRestLength = playerPogoLength
	return settings
}

func moveSystem(
	keys Keys,
	players Query[struct {
		Player *Player
		Mover  *physics.Mover
	}],
) {
	for item := range players.Items() {
		item.Player.Grounded = item.Mover.OnGround

		if item.Player.Driving != NoEntityId || item.Player.Health <= 0 {
			item.Mover.Direction = glm.Vec3f{}
			continue
		}

		item.Mover.Direction = walkDirection(moveInput(keys), item.Player.Yaw)

		if keys.IsJustPressed(vyn.KeySpace) {
			item.Mover.Jump = true
		}
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

	// dead: the camera drops to the ground where the player died and looks at the feet of the crowd
	if player.Health <= 0 {
		if player.DeadFor == 0 {
			player.deathEye = position
		}

		player.DeadFor += vt.DeltaSecs
		fall := min(1, player.DeadFor/0.5)
		fall *= fall

		position = player.deathEye
		position[1] += (0.3 - position[1]) * fall
		rotation = glm.RotationYQuat(player.Yaw).
			Mul(glm.RotationXQuat(player.Pitch * glm.Rad(1-fall))).
			Mul(glm.RotationZQuat(glm.Rad(1.2 * fall)))
	}

	camera.Transform.Translation = position
	camera.Transform.Rotation = rotation
}

// forward returns the direction the camera looks at.
func cameraForward(transform Transform) glm.Vec3f {
	return transform.Rotation.Transform(glm.Vec3f{0, 0, -1})
}

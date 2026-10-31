package main

import (
	"math"
	"math/rand/v2"
	"time"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

type ZombieKind int

const (
	// ZombieWalker is the common zombie.
	ZombieWalker ZombieKind = iota

	// ZombieRunner is a fast and weak skeleton.
	ZombieRunner

	// ZombieBrute is a big, slow and tough vampire.
	ZombieBrute
)

// zombieKinds are the stats of each kind.
var zombieKinds = map[ZombieKind]struct {
	model  string
	scale  float32
	health float32
	speed  float32
	damage float32
}{
	ZombieWalker: {"graveyard/character-zombie", 2.1, 40, 3.2, 8},
	ZombieRunner: {"graveyard/character-skeleton", 2.0, 20, 6.5, 5},
	ZombieBrute:  {"graveyard/character-vampire", 3.0, 250, 2.6, 20},
}

const (
	zombieRadius = 0.4

	// time to climb out of the ground
	zombieRiseSecs = 1.2

	zombieAttackRange = 1.5
	zombieAttackSecs  = 0.9
)

// Zombie is the body of a zombie, a capsule moved by physics.
type Zombie struct {
	Component[Zombie]

	Kind   ZombieKind
	Health float32
	Speed  float32
	Scale  float32

	// Yaw is the direction the zombie faces.
	Yaw glm.Rad

	// Phase of the walk cycle.
	Phase float32

	// Rising counts down while the zombie climbs out of the ground.
	Rising float32

	// Attack counts down to the next possible attack.
	Attack float32

	// Knockback is an extra velocity from hits, it decays quickly.
	Knockback glm.Vec3f

	// Dead is set once the zombie got killed, it despawns at the end of the frame.
	Dead bool

	// Visual is the entity that holds the model, it turns to Yaw.
	Visual EntityId

	// Parts of the model, in model part order.
	Parts []EntityId

	// halfHeight is the distance from the capsule center to the feet.
	halfHeight float32
}

// ZombieDamage hurts a zombie.
type ZombieDamage struct {
	Zombie EntityId
	Amount float32

	// Push is a velocity change in m/s, applied as knockback and to the gibs.
	Push glm.Vec3f

	// Point is where the zombie got hit, for blood.
	Point glm.Vec3f

	// Explosive damage tears the zombie apart more violently.
	Explosive bool
}

// GameRng is the random source of the game.
type GameRng struct {
	*rand.Rand
}

func zombiesPlugin(app *App) {
	app.AddMessage[ZombieDamage]()
	app.AddMessage[ZombieKilled]()
	app.InsertResource(GameRng{rand.New(rand.NewPCG(1, 2))})

	app.AddSystems(Update, System(zombieSteerSystem, zombieAnimateSystem).RunIf(NotInState(GameStateLoading)))
	app.AddSystems(Update, System(zombieDamageSystem).RunIf(NotInState(GameStateLoading)))
}

// ZombieKilled is sent when a zombie dies.
type ZombieKilled struct {
	Kind     ZombieKind
	Position glm.Vec3f
}

// spawnZombie spawns a zombie that climbs out of the ground at the given position.
func spawnZombie(commands *Commands, models *Models, kind ZombieKind, position glm.Vec3f, yaw glm.Rad, rising float32) {
	stats := zombieKinds[kind]
	model := models.Get(stats.model)

	height := model.Size()[1] * stats.scale
	radius := zombieRadius * stats.scale / 2.1
	length := max(0.1, height-2*radius)
	halfHeight := height / 2

	zombieId := commands.Spawn(
		Round{},
		TransformFromXYZ(position[0], halfHeight+0.02, position[2]),
		physics.RigidBody{Type: physics.Dynamic, LockRotation: true, Friction: 0.05, Density: 300},
		physics.ControlledVelocity{},
		physics.CollisionLayers{Category: layerZombie, Mask: layerWorld | layerProp | layerPlayer | layerZombie | layerCar},
		physics.CapsuleCollider(radius, length),
		InheritVisibility,
	).Id()

	visual := commands.Spawn(
		ChildOf{Parent: zombieId},
		// starts below the ground and climbs out
		TransformFromXYZ(0, -halfHeight-height, 0).
			WithRotation(glm.RotationYQuat(yaw)).
			WithScaleXYZ(stats.scale, stats.scale, stats.scale),
		InheritVisibility,
	).Id()

	parts := model.SpawnParts(commands, visual)

	commands.Entity(zombieId).Insert(Zombie{
		Kind:       kind,
		Health:     stats.health,
		Speed:      stats.speed,
		Scale:      stats.scale,
		Yaw:        yaw,
		Rising:     rising,
		Visual:     visual,
		Parts:      parts,
		halfHeight: halfHeight,
		Phase:      float32(position[0]),
	})
}

// zombieVelocity returns the walk velocity of a zombie at pos towards target.
func zombieVelocity(pos, target glm.Vec3f, speed float32) glm.Vec3f {
	dir := flat(target.Sub(pos))
	if dir.Length() < 0.01 {
		return glm.Vec3f{}
	}

	return dir.Normalize().Scale(speed)
}

func zombieSteerSystem(
	vt VirtualTime,
	players Query[struct {
		Player    *Player
		Transform Transform
	}],
	cameras Query[struct {
		Camera *PlayerCamera
	}],
	zombies Query[struct {
		Zombie         *Zombie
		Transform      Transform
		LinearVelocity *physics.LinearVelocity
	}],
) {
	player, ok := players.Single()
	if !ok {
		return
	}

	dt := vt.DeltaSecs
	target := player.Transform.Translation

	for item := range zombies.Items() {
		z := item.Zombie
		pos := item.Transform.Translation

		if z.Rising > 0 {
			z.Rising -= dt
			item.LinearVelocity.Vec3f = glm.Vec3f{0, item.LinearVelocity.Vec3f[1], 0}
			continue
		}

		velocity := zombieVelocity(pos, target, z.Speed)

		// turn towards the walk direction
		if velocity.Length() > 0.1 {
			z.Yaw = turnTowards(z.Yaw, yawTowards(velocity), glm.Rad(6*dt))
		}

		z.Knockback = z.Knockback.Scale(decay(4, dt))

		velocity = velocity.Add(z.Knockback)
		velocity[1] = item.LinearVelocity.Vec3f[1]
		item.LinearVelocity.Vec3f = velocity

		// attack the player when close
		z.Attack -= dt
		distance := flat(target.Sub(pos)).Length()
		if distance < zombieAttackRange*z.Scale/2.1 && z.Attack <= 0 && player.Player.Health > 0 && player.Player.Driving == NoEntityId {
			z.Attack = zombieAttackSecs
			player.Player.Health -= zombieKinds[z.Kind].damage
			player.Player.Damaged = 0.4

			for camera := range cameras.Items() {
				camera.Camera.Shake = max(camera.Camera.Shake, 0.6)
			}
		}
	}
}

// zombieAnimateSystem animates the walk procedurally: swinging legs, arms reaching forward.
func zombieAnimateSystem(
	vt VirtualTime,
	models *Models,
	zombies Query[struct {
		Zombie         *Zombie
		LinearVelocity physics.LinearVelocity
	}],
	transforms Query[*Transform],
) {
	dt := vt.DeltaSecs

	for item := range zombies.Items() {
		z := item.Zombie
		model := models.Get(zombieKinds[z.Kind].model)

		speed := flat(item.LinearVelocity.Vec3f).Length()
		z.Phase += dt * speed * 3.2 / z.Scale * 2.1

		visual, ok := transforms.Get(z.Visual)
		if !ok {
			continue
		}

		height := model.Size()[1] * z.Scale
		rise := clamp(z.Rising/zombieRiseSecs, 0, 1)
		bob := float32(math.Abs(float64(sin32(z.Phase)))) * 0.06 * z.Scale
		visual.Translation[1] = -z.halfHeight - rise*height + bob
		visual.Rotation = glm.RotationYQuat(z.Yaw).Mul(glm.RotationZQuat(glm.Rad(sin32(z.Phase) * 0.08)))

		swing := sin32(z.Phase) * 0.7
		attack := clamp(z.Attack/zombieAttackSecs, 0, 1)

		for idx, part := range model.Parts {
			if idx >= len(z.Parts) {
				break
			}

			transform, ok := transforms.Get(z.Parts[idx])
			if !ok {
				continue
			}

			rotation := glm.IdentityQuat()

			switch part.Name {
			case "leg-left":
				rotation = glm.RotationXQuat(glm.Rad(swing))
			case "leg-right":
				rotation = glm.RotationXQuat(glm.Rad(-swing))
			case "arm-left":
				// reach forward, swiping down when attacking
				rotation = glm.RotationYQuat(glm.Rad(-1.4)).Mul(glm.RotationZQuat(glm.Rad(0.2 + 0.15*swing - attack*1.2)))
			case "arm-right":
				rotation = glm.RotationYQuat(glm.Rad(1.4)).Mul(glm.RotationZQuat(glm.Rad(-0.2 + 0.15*swing + attack*1.2)))
			case "torso":
				rotation = glm.RotationXQuat(glm.Rad(0.25 + attack*0.3))
			case "head":
				rotation = glm.RotationZQuat(glm.Rad(0.3 * sin32(z.Phase*0.5)))
			}

			transform.Rotation = part.Local.Rotation.Mul(rotation)
		}
	}
}

func zombieDamageSystem(
	vt VirtualTime,
	commands *Commands,
	models *Models,
	rng *GameRng,
	stats *Stats,
	budget *SoundBudget,
	assets *GameAssets,
	damages *MessageReader[ZombieDamage],
	killed *MessageWriter[ZombieKilled],
	zombies Query[struct {
		EntityId       EntityId
		Zombie         *Zombie
		Transform      Transform
		LinearVelocity physics.LinearVelocity
	}],
) {
	for _, damage := range damages.Read() {
		item, ok := zombies.Get(damage.Zombie)
		if !ok || item.Zombie.Dead {
			continue
		}

		z := item.Zombie
		z.Health -= damage.Amount
		z.Knockback = z.Knockback.Add(flat(damage.Push).Scale(0.3))

		spawnBlood(commands, assets, rng, damage.Point, damage.Push.Scale(0.2), 3)

		if z.Health > 0 {
			playAt(commands, budget, assets.Sounds.Hit, damage.Point, 0.4)
			continue
		}

		z.Dead = true

		playAt(commands, budget, assets.Sounds.Splat, damage.Point, 1)

		model := models.Get(zombieKinds[z.Kind].model)
		visual := TransformFromXYZ(item.Transform.Translation.Add(glm.Vec3f{0, -z.halfHeight, 0}).XYZ()).
			WithRotation(glm.RotationYQuat(z.Yaw)).
			WithScaleXYZ(z.Scale, z.Scale, z.Scale)

		push := damage.Push
		spread := float32(2)
		if damage.Explosive {
			spread = 6
		}

		spawnGibs(commands, rng, model, visual, item.LinearVelocity.Vec3f.Add(push), spread)
		spawnBlood(commands, assets, rng, item.Transform.Translation, push.Scale(0.3), 14)

		commands.Entity(item.EntityId).Despawn()

		if msg := stats.AddKill(vt.Elapsed); msg != "" {
			stats.Show(vt, msg, 1500*time.Millisecond)
		}

		killed.Write(ZombieKilled{Kind: z.Kind, Position: item.Transform.Translation})
	}
}

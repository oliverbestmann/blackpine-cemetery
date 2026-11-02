package main

import (
	"math"
	"math/rand/v2"

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

	// ZombieGhost flies around and dives at the player.
	ZombieGhost

	// ZombieKeeper is the graveyard keeper, a boss every few waves. He raises the dead around him.
	ZombieKeeper
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
	ZombieGhost:  {"graveyard/character-ghost", 2.4, 30, 8, 10},
	ZombieKeeper: {"graveyard/character-keeper", 5.5, 3000, 2.8, 35},
}

const (
	zombieRadius = 0.4

	// time to climb out of the ground
	zombieRiseSecs = 1.2

	zombieAttackRange = 1.5
	zombieAttackSecs  = 0.9

	// ghosts cruise at this height and dive at the player when close
	ghostCruiseHeight = 7
	ghostDiveRange    = 14
	ghostAttackRange  = 2
	ghostClimbSecs    = 2.5

	// the keeper raises this many walkers around him every keeperRaiseSecs
	keeperRaiseSecs  = 7
	keeperRaiseCount = 4
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

	// Raise counts down to the keeper raising the next dead.
	Raise float32

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

	Source DamageSource
}

// DamageSource is what hurt a zombie, for the score.
type DamageSource int

const (
	SourceGun DamageSource = iota
	SourceMelee
	SourceCar
	SourceExplosion
)

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
	Source   DamageSource
}

// spawnZombie spawns a zombie that climbs out of the ground at the given position.
func spawnZombie(commands *Commands, models *Models, kind ZombieKind, position glm.Vec3f, yaw glm.Rad, rising float32) {
	stats := zombieKinds[kind]
	model := models.Get(stats.model)

	height := model.Size().Y * stats.scale
	radius := zombieRadius * stats.scale / 2.1
	length := max(0.1, height-2*radius)
	halfHeight := height / 2

	y := halfHeight + 0.02
	layers := physics.CollisionLayers{Category: layerZombie, Mask: layerWorld | layerProp | layerPlayer | layerZombie | layerCar}
	if kind == ZombieGhost {
		// ghosts fly through everything but each other
		y = ghostCruiseHeight + position.Y
		layers = physics.CollisionLayers{Category: layerFlyer, Mask: layerFlyer}
		rising = 0
	}

	zombieId := commands.Spawn(
		Round{},
		TransformFromXYZ(position.X, y, position.Z),
		physics.RigidBody{Type: physics.Dynamic, LockRotation: true, Friction: 0.05, Density: 300},
		physics.ControlledVelocity{},
		layers,
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
		Phase:      float32(position.X),
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
	commands *Commands,
	models *Models,
	rng *GameRng,
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
	cars Query[*Car],
) {
	player, ok := players.Single()
	if !ok {
		return
	}

	dt := vt.DeltaSecs
	target := player.Transform.Translation

	// a driving player is out of reach, zombies attack the car instead
	car, driving := cars.Get(player.Player.Driving)
	reach := float32(0)
	if driving {
		reach = carZombieReach
	}

	hurt := func(amount float32) {
		if driving {
			car.Health -= amount
		} else {
			player.Player.Health -= amount
		}

		player.Player.Damaged = 0.4
	}

	for item := range zombies.Items() {
		z := item.Zombie
		pos := item.Transform.Translation

		if z.Rising > 0 {
			z.Rising -= dt
			item.LinearVelocity.Vec3f = glm.V3f(0, item.LinearVelocity.Vec3f.Y, 0)
			continue
		}

		if z.Kind == ZombieGhost {
			if steerGhost(z, item.Transform.Translation, item.LinearVelocity, player.Player, target, reach, dt) {
				hurt(zombieKinds[z.Kind].damage)
			}

			continue
		}

		if z.Kind == ZombieKeeper {
			z.Raise -= dt
			if z.Raise <= 0 && zombies.Count() < maxZombiesAlive {
				z.Raise = keeperRaiseSecs

				for i := range keeperRaiseCount {
					angle := glm.Rad(float32(i)*2*math.Pi/keeperRaiseCount + rng.Float32())
					at := pos.Add(glm.V3f(cos32(float32(angle)), 0, sin32(float32(angle))).Scale(4 + 2*rng.Float32()))
					at.Y = 0
					spawnZombie(commands, models, ZombieWalker, at, yawTowards(target.Sub(at)), zombieRiseSecs)
				}
			}
		}

		velocity := zombieVelocity(pos, target, z.Speed)

		// turn towards the walk direction
		if velocity.Length() > 0.1 {
			z.Yaw = turnTowards(z.Yaw, yawTowards(velocity), glm.Rad(6*dt))
		}

		z.Knockback = z.Knockback.Scale(decay(4, dt))

		velocity = velocity.Add(z.Knockback)
		velocity.Y = item.LinearVelocity.Vec3f.Y
		item.LinearVelocity.Vec3f = velocity

		// attack the player when close
		z.Attack -= dt
		distance := flat(target.Sub(pos)).Length()
		if distance < zombieAttackRange*z.Scale/2.1+reach && z.Attack <= 0 && player.Player.Health > 0 {
			z.Attack = zombieAttackSecs
			hurt(zombieKinds[z.Kind].damage)

			for camera := range cameras.Items() {
				camera.Camera.Shake = max(camera.Camera.Shake, 0.6)
			}
		}
	}
}

// steerGhost flies a ghost: cruise towards the player up high, dive at him when close,
// climb away after an attack. It returns true when the ghost hits.
func steerGhost(z *Zombie, pos glm.Vec3f, velocity *physics.LinearVelocity, player *Player, target glm.Vec3f, reach, dt float32) bool {
	z.Attack -= dt
	z.Phase += dt * 4

	toPlayer := flat(target.Sub(pos))
	distance := toPlayer.Length()

	var goal glm.Vec3f
	switch {
	case z.Attack > zombieAttackSecs:
		// climbing away after a hit
		goal = pos.Sub(toPlayer.Normalize().Scale(5)).Add(glm.V3f(0, 6, 0))
	case distance < ghostDiveRange && z.Attack <= 0:
		goal = target.Add(glm.V3f(0, 0.3, 0))
	default:
		// circle a bit while closing in
		side := glm.V3f(-toPlayer.Z, 0, toPlayer.X).Normalize().Scale(6 * sin32(z.Phase*0.3))
		goal = target.Add(side).Add(glm.V3f(0, ghostCruiseHeight, 0))
	}

	dir := goal.Sub(pos)
	if dir.Length() > 0.1 {
		dir = dir.Normalize()
	}

	speed := z.Speed
	if z.Attack <= 0 && distance < ghostDiveRange {
		speed *= 1.4
	}

	if distance > 0.1 {
		z.Yaw = turnTowards(z.Yaw, yawTowards(toPlayer), glm.Rad(4*dt))
	}

	z.Knockback = z.Knockback.Scale(decay(4, dt))
	v := dir.Scale(speed).Add(z.Knockback)

	// never below the ground
	if pos.Y < 1.5 {
		v.Y = max(v.Y, 2)
	}

	velocity.Vec3f = v

	if z.Attack <= 0 && target.Sub(pos).Length() < ghostAttackRange+reach && player.Health > 0 {
		z.Attack = zombieAttackSecs + ghostClimbSecs
		return true
	}

	return false
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

		height := model.Size().Y * z.Scale
		rise := clamp(z.Rising/zombieRiseSecs, 0, 1)
		bob := float32(math.Abs(float64(sin32(z.Phase)))) * 0.06 * z.Scale
		visual.Translation.Y = -z.halfHeight - rise*height + bob
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
		knockback := float32(0.3)
		if z.Kind == ZombieKeeper {
			// the boss barely moves
			knockback = 0.03
		}

		z.Knockback = z.Knockback.Add(flat(damage.Push).Scale(knockback))

		spawnBlood(commands, assets, rng, damage.Point, damage.Push.Scale(0.2), 3)

		if z.Health > 0 {
			playAt(commands, budget, assets.Sounds.Hit, damage.Point, 0.4)
			continue
		}

		z.Dead = true

		playAt(commands, budget, assets.Sounds.Splat, damage.Point, 1)

		model := models.Get(zombieKinds[z.Kind].model)
		visual := TransformFromXYZ(item.Transform.Translation.Add(glm.V3f(0, -z.halfHeight, 0)).XYZ()).
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

		killed.Write(ZombieKilled{Kind: z.Kind, Position: item.Transform.Translation, Source: damage.Source})
	}
}

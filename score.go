package main

import (
	"fmt"
	"time"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// zombiePoints are the points for a kill, before the multi kill multiplier.
var zombiePoints = map[ZombieKind]int{
	ZombieWalker: 100,
	ZombieRunner: 75,
	ZombieBrute:  500,
	ZombieGhost:  150,
}

// maxMultiplier caps the multi kill multiplier: the n-th kill of a multi kill counts n times.
const maxMultiplier = 10

type achievement int

const (
	achFirstBlood achievement = iota
	achCentury
	achGravedigger
	achRoadkill
	achFireworks
	achBruteForce
	achUntouchable
	achSurvivor
	achLastStand
)

// achievements give bonus points, each once per round.
var achievements = [...]struct {
	name  string
	bonus int
}{
	achFirstBlood:  {"FIRST BLOOD", 100},
	achCentury:     {"CENTURY", 2500},
	achGravedigger: {"GRAVEDIGGER", 1000},
	achRoadkill:    {"ROADKILL", 250},
	achFireworks:   {"FIREWORKS", 1000},
	achBruteForce:  {"BRUTE FORCE", 500},
	achUntouchable: {"UNTOUCHABLE", 1500},
	achSurvivor:    {"SURVIVOR", 2000},
	achLastStand:   {"LAST STAND", 750},
}

const (
	centuryKills     = 100
	gravediggerKills = 10
	fireworksKills   = 5
	survivorWave     = 5
	lastStandHealth  = 15
)

// unlock gives the bonus of an achievement, if the player does not have it yet.
func (s *Stats) unlock(vt VirtualTime, a achievement) {
	if s.achieved[a] {
		return
	}

	s.achieved[a] = true
	s.Score += achievements[a].bonus
	s.Show(vt, fmt.Sprintf("%s\n+%d", achievements[a].name, achievements[a].bonus), 2*time.Second)
}

// scoreKill counts a kill, gives its points and checks the kill achievements.
// It returns the points and the multiplier.
func (s *Stats) scoreKill(vt VirtualTime, kill ZombieKilled, health float32) (int, int) {
	multi, msg := s.AddKill(vt.Elapsed)
	if msg != "" {
		s.Show(vt, msg, 1500*time.Millisecond)
	}

	multi = min(multi, maxMultiplier)
	points := zombiePoints[kill.Kind] * multi
	s.Score += points

	if s.Kills == 1 {
		s.unlock(vt, achFirstBlood)
	}

	if s.Kills == centuryKills {
		s.unlock(vt, achCentury)
	}

	if kill.Kind == ZombieBrute {
		s.unlock(vt, achBruteForce)
	}

	if health > 0 && health < lastStandHealth {
		s.unlock(vt, achLastStand)
	}

	switch kill.Source {
	case SourceMelee:
		s.meleeKills++
		if s.meleeKills == gravediggerKills {
			s.unlock(vt, achGravedigger)
		}

	case SourceCar:
		s.unlock(vt, achRoadkill)

	case SourceExplosion:
		// shortcut: kills of the same frame count as one explosion, chained barrels do not add up
		if s.blastAt != vt.Elapsed {
			s.blastAt = vt.Elapsed
			s.blastKills = 0
		}

		s.blastKills++
		if s.blastKills == fireworksKills {
			s.unlock(vt, achFireworks)
		}
	}

	return points, multi
}

// trackWave checks the wave achievements. A new wave number means the last wave got cleared.
func (s *Stats) trackWave(vt VirtualTime, health float32) {
	if health < s.lastHealth {
		s.waveHurt = true
	}

	s.lastHealth = health

	if s.Wave != s.seenWave {
		if s.seenWave > 0 && !s.waveHurt {
			s.unlock(vt, achUntouchable)
		}

		s.seenWave = s.Wave
		s.waveHurt = false
	}

	if s.Wave >= survivorWave {
		s.unlock(vt, achSurvivor)
	}
}

func scorePlugin(app *App) {
	app.AddSystems(Update, System(scoreSystem).Before(deathSystem).RunIf(InState(GameStatePlaying)))
	app.AddSystems(Update, System(scorePopupSystem, leaderboardSystem).RunIf(NotInState(GameStateLoading)))
	app.AddSystems(OnEnter(GameStateDead), submitScoreSystem)
}

func scoreSystem(vt VirtualTime, commands *Commands, stats *Stats, killed *MessageReader[ZombieKilled], players Query[Player]) {
	player, _ := players.Single()

	stats.trackWave(vt, player.Health)

	for _, kill := range killed.Read() {
		points, multi := stats.scoreKill(vt, kill, player.Health)

		text := fmt.Sprintf("+%d", points)
		if multi > 1 {
			text = fmt.Sprintf("+%d x%d", points, multi)
		}

		commands.Spawn(
			Round{},
			NewTransform(),
			AnchorCenter,
			// invisible until scorePopupSystem placed it
			Text{Text: text, Size: float32(32 + 4*min(multi, 5)), Color: ColorSRGBA(1, 0.8, 0.2, 0)},
			hudFont(),
			ScorePopup{World: kill.Position.Add(glm.Vec3f{0, 1.5, 0})},
			RenderLayersOf(hudLayer),
		)
	}
}

// ScorePopup is a hud text with the points of a kill, over the killed zombie.
// It rises and fades away.
type ScorePopup struct {
	Component[ScorePopup]
	World glm.Vec3f
	Age   float32
}

const scorePopupSecs = 1.2

func scorePopupSystem(
	vt VirtualTime,
	commands *Commands,
	screen ScreenSize,
	cameras Query[struct {
		_          With[PlayerCamera]
		Transform  GlobalTransform
		Projection PerspectiveProjection
	}],
	popups Query[struct {
		EntityId  EntityId
		Popup     *ScorePopup
		Transform *Transform
		Text      *Text
	}],
) {
	camera, ok := cameras.Single()
	if !ok {
		return
	}

	half := hudSize(screen).Scale(0.5)
	worldToCamera, _ := camera.Transform.Affine.TryInverse()
	worldToClip := camera.Projection.ToMat4f(half).Mul(worldToCamera)

	for popup := range popups.Items() {
		popup.Popup.Age += vt.DeltaSecs

		t := popup.Popup.Age / scorePopupSecs
		if t >= 1 {
			commands.Entity(popup.EntityId).Despawn()
			continue
		}

		// rises two meters
		p := popup.Popup.World
		clip := worldToClip.Transform(glm.Vec4f{p[0], p[1] + 2*t, p[2], 1})

		alpha := 1 - t*t
		if clip[3] <= 0 {
			// behind the camera
			alpha = 0
		}

		pos := glm.Vec2f{clip[0] / clip[3], clip[1] / clip[3]}.Mul(half)
		popup.Transform.Translation = glm.Vec3f{pos[0], pos[1], 0.3}
		popup.Text.Color = ColorSRGBA(1, 0.8, 0.2, alpha)
	}
}

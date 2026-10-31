# Zombies - plan

The original request is in [PROMPT.md](PROMPT.md).

A small over-the-top first person shooter in the spirit of Serious Sam: huge crowds of
zombies run at you, you mostly run backwards and shoot, and everything flies around
when something explodes.

## Setting and style

**"Hollow Pines"**: a cemetery at the edge of a small town at a blood red dusk. The
dead climb out of the graveyard in waves. The area is a big open field, with
graves, crypts, fences, trees, hay bales, crates, barrels and abandoned cars as
cover and as physics props.

Style is low poly toy/comic, using the CC0 Kenney kits (all consistent in style):

- graveyard-kit: zombie, skeleton, ghost (flying enemy), vampire, graves, crypts, trees, fences
- blaster-kit: guns (view models and pickups), grenades, crates
- car-kit: drivable car, wrecks, debris (car parts flying around)
- survival-kit: barrels, boxes, rocks
- sci-fi-sounds + impact-sounds: weapon, explosion and impact sounds

Sky: Poly Haven `kloppenheim_06_puresky` (CC0), tinted red/orange, converted to a cube
map plus diffuse/specular environment maps with glTF-IBL-Sampler (see `tools/`).

## Architecture

Go module `github.com/oliverbestmann/zombies`, byke (ECS + renderer) and box3d-go (physics).

| File | Content |
|------|---------|
| `main.go` | app setup, game states, screenshot hook |
| `physics/` | box3d integration, copied from load20sgj and extended (motion locks, velocity control, collision filters) |
| `assets.go` | loading, prefab extraction from glTF (shared meshes and materials, so instances batch) |
| `world.go` | arena, sky, sun, props |
| `player.go` | first person controller (dynamic capsule), health |
| `weapons.go` | weapon table, firing, ammo, view model |
| `projectiles.go` | rockets and grenades |
| `explosions.go` | damage, physics impulse, point light flash, fireball, camera shake |
| `zombies.go` | spawning in waves, steering, procedural walk animation, attacks |
| `flyers.go` | flying ghosts that swoop at the player |
| `gibs.go` | body parts and blood chunks as physics debris, despawn after a while |
| `pickups.go` | weapon, ammo and health drops |
| `car.go` | drivable car that runs zombies over |
| `hud.go` | crosshair, health, ammo, weapon, kills, wave |
| `sounds.go` | sound effects |

Prefabs: glTF scenes are spawned once, their mesh and material are harvested by node
name, then cloned for every instance. byke caches meshes per spawned scene, so spawning
a glTF scene per zombie would upload the meshes again and break batching.

Zombies are built from the six parts of the Kenney character (legs, torso, arms, head)
and animated procedurally. Each zombie is a dynamic capsule with locked rotation that
steers towards the player, so crowds push each other and the props. When a zombie dies
its parts become dynamic bodies and fly away (gibs).

## Weapons

| Key | Weapon | Type |
|-----|--------|------|
| 1 | Machine gun | hitscan, fast |
| 2 | Double uzi | hitscan, very fast, spread, two guns |
| 3 | Gatling gun | hitscan, spins up, insane rate |
| 4 | Shotgun | hitscan, many pellets |
| 5 | Grenade launcher | bouncing grenades with fuse (physics bodies) |
| 6 | Rocket launcher | rockets, explode on contact |

Mouse wheel / Q cycles weapons. Zombies drop ammo, health and new weapons.

## Milestones and tasks

- [x] M0 scaffold app, screenshot hook
- [x] M1 world: assets imported, ground, sky + IBL, sun with shadows following the player, props
- [x] M2 player: mouse look, WASD, jump, physics capsule, collision with props
- [x] M3 weapons: machine gun hitscan, view model, muzzle flash, HUD crosshair + ammo
- [x] M4 zombies: prefab, spawning waves, steering, walk animation, damage player
- [x] M5 gore: gibs with physics, blood chunks, cleanup limits
- [x] M6 explosives: rockets, grenades, explosive barrels, point lights, impulses, camera shake
- [x] M7 all weapons + pickups/drops + weapon switching
- [x] M8 flying enemies (ghosts)
- [x] M9 car: enter/exit (F), drive, run over zombies
- [ ] M10 sound
- [ ] M11 game flow: start, waves, death, restart, score
- [ ] M12 polish: performance with many zombies, balance, README with controls

## Verification

- Unit tests for pure logic (weapon timing, damage falloff, spawning, steering, waves).
- Physics tests with a headless app (no renderer), stepping the fixed schedules.
- Offscreen render: `ZOMBIES_SCREENSHOT=out.png` renders frame 10 (see `screenshot.sh`),
  checked visually after each milestone.
- `go vet ./...` and `gofumpt`.

## Notes / decisions

- Own glTF loader (`model.go`) instead of SceneRoot, so meshes are shared and batched. Zombies
  are built from the 6 parts of the Kenney character models and animated procedurally.
- Zombies and the player are dynamic capsules with locked rotation, their velocity is set each
  frame (`physics.ControlledVelocity`). Physics resolves crowds and props.
- Every entity with a mesh below another entity needs a `Visibility` on all its ancestors. byke
  stops propagating visibility at entities without one, the mesh stays invisible.
- byke adds the emissive color after exposure, so emissive values are in output units (~1-10).
- Testing env vars: `ZOMBIES_DEMO=1` spawns a crowd in front of the player, `ZOMBIES_AUTOFIRE=1`
  holds the trigger, `ZOMBIES_WEAPON=n` gives all weapons and selects weapon n.
- Lavapipe renders about 3 frames per second, use frames 100-300 for screenshots.

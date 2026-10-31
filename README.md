# Zombies of Hollow Pines

A small over-the-top zombie shooter in the style of Serious Sam, built with
[byke](../byke) and box3d physics. The dead climb out of the graveyard of Hollow Pines
in ever bigger waves. Run backwards, keep shooting, and grab the guns they drop.

## Run

```sh
nix develop ../nix -c sh -c 'GOEXPERIMENT=simd go run .'
```

## Controls

| Key                  | Action                              |
|----------------------|-------------------------------------|
| Mouse                | look (click to grab the cursor)     |
| Left mouse / Ctrl    | fire                                |
| W A S D              | move                                |
| Space                | jump                                |
| 1 - 6                | select weapon                       |
| Q / E, Tab           | previous / next weapon              |
| F                    | enter or leave the police car       |
| W A S D (in the car) | drive, run the dead over            |
| R                    | rise again after you got eaten      |
| Esc                  | quit                                |

## Weapons

1. Machine gun - the starting gun
2. Double uzi - two guns, lots of bullets
3. Shotgun - 12 pellets, pushes the dead back
4. Gatling gun - spins up, then shreds everything
5. Grenade launcher - bouncing grenades, explode on contact with the dead
6. Rocket launcher - big explosions

Killed zombies sometimes drop weapons, ammo clips or carved pumpkins (health).
Brutes (the big vampires) always drop a weapon. Shoot the barrels, they explode
and set off other barrels nearby.

## Enemies

- Zombies: slow, many
- Skeletons: fast and weak, from wave 2
- Vampires: big and tough, from wave 3
- Ghosts: fly and dive at you, from wave 2

## Development

```sh
nix develop ../nix -c sh -c 'GOEXPERIMENT=simd go vet ./... && GOEXPERIMENT=simd go test ./...'

# offscreen screenshot of frame 200 with a crowd and the rocket launcher firing
ZOMBIES_DEMO=1 ZOMBIES_AUTOFIRE=1 ZOMBIES_WEAPON=5 nix develop ../nix -c ./screenshot.sh /tmp/z.png 200
```

The screenshot writes one image per camera: `/tmp/z.png` is the HUD, `/tmp/z.2.png` the 3D view.
Test env vars: `ZOMBIES_DEMO` (crowd in front of the player), `ZOMBIES_AUTOFIRE` (hold the trigger),
`ZOMBIES_WEAPON=n` (all weapons, select n), `ZOMBIES_DRIVE` (drive the car in circles).

Asset credits are in [CREDITS.md](CREDITS.md).

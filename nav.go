package main

import (
	"math"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/zombies/physics"
)

const (
	navCellSize    = 1.5
	navCells       = int(2 * arenaHalfSize / navCellSize)
	navRebuildSecs = 1

	// step costs for Dial's algorithm, 7/5 is close to sqrt(2)
	navCostStraight = 5
	navCostDiagonal = 7
)

// ZombieNav is a coarse grid over the arena with the walking distance from every cell
// to the player. All zombies share it: each one walks to the neighbor cell closest
// to the player, or straight at the player when nothing is in the way.
type ZombieNav struct {
	blocked []bool
	dist    []int32

	// cursor is the next cell to probe
	cursor int

	// buckets of cells by distance modulo the bucket count
	buckets [navCostDiagonal + 1][]int32
}

func navPlugin(app *App) {
	app.InsertResource(ZombieNav{})
	app.AddSystems(Update, System(zombieNavSystem).Before(zombieSteerSystem).RunIf(NotInState(GameStateLoading)))
}

// zombieNavSystem probes the grid against the physics world, spread over a second,
// and updates the distances after each full pass.
func zombieNavSystem(
	vt VirtualTime,
	world physics.PhysicsWorld,
	nav *ZombieNav,
	players Query[struct {
		_         With[Player]
		Transform Transform
	}],
) {
	player, ok := players.Single()
	if !ok {
		return
	}

	count := int(math.Ceil(float64(float32(navCells*navCells) * vt.DeltaSecs / navRebuildSecs)))

	// probe everything at once the first time
	if nav.blocked == nil {
		nav.blocked = make([]bool, navCells*navCells)
		nav.dist = make([]int32, navCells*navCells)
		count = len(nav.blocked)
	}

	const radius = navCellSize / 2
	end := min(nav.cursor+count, len(nav.blocked))
	for idx := nav.cursor; idx < end; idx++ {
		c := navCenter(idx)
		nav.blocked[idx] = world.OverlapsCapsule(c.Add(glm.V3f(0, 0.1+radius, 0)), c.Add(glm.V3f(0, 2.5, 0)), radius, layerWorld|layerProp|layerCar)
	}

	nav.cursor = end
	if nav.cursor == len(nav.blocked) {
		nav.cursor = 0
		nav.solve(player.Transform.Translation)
	}
}

// solve fills dist with the walking distance to target, using Dial's algorithm.
func (nav *ZombieNav) solve(target glm.Vec3f) {
	for idx := range nav.dist {
		nav.dist[idx] = math.MaxInt32
	}

	tx, tz := navCell(target)

	// the player may stand next to a tree or sit in a car, keep his surroundings open
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			if x, z := tx+dx, tz+dz; navInside(x, z) {
				nav.blocked[z*navCells+x] = false
			}
		}
	}

	if !navInside(tx, tz) {
		return
	}

	start := tz*navCells + tx
	nav.dist[start] = 0

	const bucketCount = int32(len(nav.buckets))
	for i := range nav.buckets {
		nav.buckets[i] = nav.buckets[i][:0]
	}

	nav.buckets[0] = append(nav.buckets[0], int32(start))
	pending := 1

	for d := int32(0); pending > 0; d++ {
		bucket := &nav.buckets[d%bucketCount]

		for len(*bucket) > 0 {
			idx := (*bucket)[len(*bucket)-1]
			*bucket = (*bucket)[:len(*bucket)-1]
			pending--

			// outdated entry, the cell was reached on a shorter path
			if nav.dist[idx] != d {
				continue
			}

			x, z := int(idx)%navCells, int(idx)/navCells
			for dz := -1; dz <= 1; dz++ {
				for dx := -1; dx <= 1; dx++ {
					n, ok := nav.step(x, z, dx, dz)
					if !ok || nav.blocked[n] {
						continue
					}

					nd := d + navCostStraight
					if dx != 0 && dz != 0 {
						nd = d + navCostDiagonal
					}

					if nd < nav.dist[n] {
						nav.dist[n] = nd
						nav.buckets[nd%bucketCount] = append(nav.buckets[nd%bucketCount], int32(n))
						pending++
					}
				}
			}
		}
	}
}

// step returns the neighbor of cell x,z in direction dx,dz. Diagonal steps must not cut
// the corner of a blocked cell.
func (nav *ZombieNav) step(x, z, dx, dz int) (int, bool) {
	nx, nz := x+dx, z+dz
	if (dx == 0 && dz == 0) || !navInside(nx, nz) {
		return 0, false
	}

	if dx != 0 && dz != 0 && (nav.blocked[z*navCells+nx] || nav.blocked[nz*navCells+x]) {
		return 0, false
	}

	return nz*navCells + nx, true
}

// direction returns where a zombie at pos should walk to reach the player at target.
// It is not normalized and has no y component.
func (nav *ZombieNav) direction(pos, target glm.Vec3f) glm.Vec3f {
	straight := flat(target.Sub(pos))

	x, z := navCell(pos)
	if nav.dist == nil || !navInside(x, z) || nav.clearLine(pos, target) {
		return straight
	}

	best, bestDist := -1, int32(math.MaxInt32)
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			if n, ok := nav.step(x, z, dx, dz); ok && nav.dist[n] < bestDist {
				best, bestDist = n, nav.dist[n]
			}
		}
	}

	// unreachable, maybe the zombie got pushed somewhere odd
	if best < 0 {
		return straight
	}

	return flat(navCenter(best).Sub(pos))
}

// clearLine reports if no blocked cell lies between a and b.
func (nav *ZombieNav) clearLine(a, b glm.Vec3f) bool {
	delta := flat(b.Sub(a))
	steps := int(delta.Length()/(navCellSize/2)) + 1

	for i := range steps + 1 {
		p := a.Add(delta.Scale(float32(i) / float32(steps)))
		if x, z := navCell(p); navInside(x, z) && nav.blocked[z*navCells+x] {
			return false
		}
	}

	return true
}

func navCell(p glm.Vec3f) (x, z int) {
	return int(math.Floor(float64((p.X + arenaHalfSize) / navCellSize))),
		int(math.Floor(float64((p.Z + arenaHalfSize) / navCellSize)))
}

func navInside(x, z int) bool {
	return x >= 0 && z >= 0 && x < navCells && z < navCells
}

func navCenter(idx int) glm.Vec3f {
	x, z := idx%navCells, idx/navCells
	return glm.V3f(
		(float32(x)+0.5)*navCellSize-arenaHalfSize,
		0,
		(float32(z)+0.5)*navCellSize-arenaHalfSize,
	)
}

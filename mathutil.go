package main

import (
	"math"
	"math/rand/v2"

	"github.com/oliverbestmann/byke/byke2d/glm"
)

// flat removes the vertical component.
func flat(v glm.Vec3f) glm.Vec3f {
	return glm.V3f(v.X, 0, v.Z)
}

// yawTowards returns the yaw that turns +z towards dir.
func yawTowards(dir glm.Vec3f) glm.Rad {
	return glm.Rad(math.Atan2(float64(dir.X), float64(dir.Z)))
}

// turnTowards turns angle towards target by at most step, the short way round.
func turnTowards(angle, target, step glm.Rad) glm.Rad {
	diff := math.Remainder(float64(target-angle), 2*math.Pi)
	if math.Abs(diff) <= float64(step) {
		return target
	}

	if diff > 0 {
		return angle + step
	}

	return angle - step
}

// decay returns the factor that reduces a value at the given rate per second over dt seconds.
func decay(rate, dt float32) float32 {
	return float32(math.Exp(float64(-rate * dt)))
}

// randomInSphere returns a random point in the unit sphere.
func randomInSphere(rng *rand.Rand) glm.Vec3f {
	for {
		v := glm.V3f(rng.Float32()*2-1, rng.Float32()*2-1, rng.Float32()*2-1)
		if v.Length() <= 1 {
			return v
		}
	}
}

func sin32(v float32) float32 {
	return float32(math.Sin(float64(v)))
}

func cos32(v float32) float32 {
	return float32(math.Cos(float64(v)))
}

func lerp(a, b, t float32) float32 {
	return a + (b-a)*t
}

func clamp(v, lo, hi float32) float32 {
	return max(lo, min(hi, v))
}

func abs32(v float32) float32 {
	return max(v, -v)
}

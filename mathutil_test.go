package main

import (
	"math"
	"testing"

	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestYawTowards(t *testing.T) {
	for _, dir := range []glm.Vec3f{glm.V3f(1, 0, 0), glm.V3f(0, 0, -1), glm.V3f(-1, 0, 1)} {
		got := glm.RotationYQuat(yawTowards(dir)).Transform(glm.V3f(0, 0, 1))
		if got.Sub(dir.Normalize()).Length() > 1e-5 {
			t.Errorf("yaw towards %v turns +z to %v", dir, got)
		}
	}
}

func TestTurnTowards(t *testing.T) {
	// the short way from 170° to -170° is over 180°
	got := turnTowards(glm.DegToRad(170), glm.DegToRad(-170), glm.DegToRad(5))
	if math.Abs(float64(got-glm.DegToRad(175))) > 1e-5 {
		t.Fatalf("expected 175°, got %v", float64(got)*180/math.Pi)
	}

	if got := turnTowards(0, 0.1, 1); got != 0.1 {
		t.Fatalf("should reach a close target, got %v", got)
	}
}

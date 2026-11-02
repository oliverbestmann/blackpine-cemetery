package main

import (
	"math"
	"testing"

	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestOverlayColor(t *testing.T) {
	cases := []struct {
		damaged, health float32
		want            glm.Vec4f
	}{
		{0, playerMaxHealth, glm.V4f(0, 0, 0, 1)},
		{0.4, 80, glm.V4f(1, 0, 0, 1)},   // just hit
		{0.2, 80, glm.V4f(0.5, 0, 0, 1)}, // flash fades
		{0, 15, glm.V4f(0, 0.5, 0, 1)},   // low health
		{0, 0, glm.V4f(1, 1, 0, 1)},      // dead
	}

	for _, c := range cases {
		if got := overlayColor(c.damaged, c.health).ToVec(); got.Sub(c.want).Length() > 1e-5 {
			t.Errorf("overlayColor(%v, %v) = %v, want %v", c.damaged, c.health, got, c.want)
		}
	}
}

func TestMarkerPlacement(t *testing.T) {
	half := glm.V2f(889, 500)
	projection := PerspectiveProjection{Fov: glm.DegToRad(80), Near: 0.05, Far: 1000}.ToMat4f(half)

	place := func(pitch float32, target glm.Vec3f) (glm.Vec2f, glm.Rad) {
		transform := TransformFromXYZ(0, 1.7, 0).WithRotation(glm.RotationXQuat(glm.DegToRad(pitch)))
		camera := GlobalTransform{Affine: glm.IdentityMat4f()}.Mul(transform).Affine
		worldToCamera, _ := camera.TryInverse()
		return markerPlacement(camera, projection.Mul(worldToCamera), half, target)
	}

	// ahead: on the floor below the crosshair, pointing up
	pos, angle := place(0, glm.V3f(0, 0, -10))
	if math.Abs(float64(pos.X)) > 1 || pos.Y > -100 || pos.Y < -half.Y+60 || math.Abs(float64(angle)) > 1e-3 {
		t.Errorf("ahead: %v %v", pos, angle)
	}

	// ahead and to the right: on the floor right of the center. A floor arrow pointing away from
	// the player lies in a vertical plane through the eye, so it still points up on screen.
	pos, angle = place(0, glm.V3f(5, 0, -10))
	if pos.X < 10 || pos.Y > -100 || math.Abs(float64(angle)) > 1e-3 {
		t.Errorf("right: %v %v", pos, angle)
	}

	// behind: at the bottom edge, pointing down
	pos, angle = place(0, glm.V3f(-1, 0, 10))
	if pos.Y != -half.Y+60 || pos.X > -half.X/2 || math.Abs(float64(angle)) < 3 {
		t.Errorf("behind: %v %v", pos, angle)
	}

	// looking up, the floor is out of view: at the bottom edge, pointing up
	pos, angle = place(70, glm.V3f(0, 0, -10))
	if pos != (glm.V2f(0, -half.Y+60)) || angle != 0 {
		t.Errorf("looking up: %v %v", pos, angle)
	}
}

func TestMarkerAngle(t *testing.T) {
	// camera at (0, 2, 0), turned 90 degrees left, so it looks along -x, and tilted down
	rotation := glm.RotationYQuat(glm.DegToRad(90)).Mul(glm.RotationXQuat(glm.DegToRad(-30)))
	camera := GlobalTransform{Affine: glm.IdentityMat4f()}.Mul(TransformFromXYZ(0, 2, 0).WithRotation(rotation)).Affine

	cases := []struct {
		target glm.Vec3f
		angle  float64
	}{
		{glm.V3f(-10, 0, 0), 0},           // ahead
		{glm.V3f(0, 5, -10), math.Pi / 2}, // right
		{glm.V3f(0, 0, 10), -math.Pi / 2}, // left
		{glm.V3f(10, 0, 0), math.Pi},      // behind
	}

	for _, c := range cases {
		got := math.Abs(float64(markerAngle(camera, c.target)))
		if math.Abs(got-math.Abs(c.angle)) > 1e-4 {
			t.Errorf("target %v: angle %v, want %v", c.target, got, c.angle)
		}

		if c.angle != 0 && c.angle != math.Pi && math.Signbit(float64(markerAngle(camera, c.target))) != math.Signbit(c.angle) {
			t.Errorf("target %v: wrong side", c.target)
		}
	}
}

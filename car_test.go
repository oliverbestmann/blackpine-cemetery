package main

import (
	"testing"

	"github.com/oliverbestmann/byke/byke2d/glm"
)

func TestCarAcceleratesToMaxSpeed(t *testing.T) {
	forward := glm.Vec3f{0, 0, 1}

	var velocity glm.Vec3f
	for range 600 {
		velocity, _ = carVelocity(defaultCarKind, velocity, forward, 1, 0, 1.0/60)
	}

	if speed := velocity.Dot(forward); speed < defaultCarKind.MaxSpeed-0.01 || speed > defaultCarKind.MaxSpeed+0.01 {
		t.Fatalf("expected max speed, got %v", speed)
	}
}

func TestCarBrakesAndReverses(t *testing.T) {
	forward := glm.Vec3f{0, 0, 1}
	velocity := glm.Vec3f{0, 0, 10}

	for range 600 {
		velocity, _ = carVelocity(defaultCarKind, velocity, forward, -1, 0, 1.0/60)
	}

	if speed := velocity.Dot(forward); speed > -carMaxReverse+0.01 {
		t.Fatalf("expected full reverse, got %v", speed)
	}
}

func TestCarGripKillsSliding(t *testing.T) {
	forward := glm.Vec3f{0, 0, 1}
	velocity := glm.Vec3f{10, -2, 0}

	for range 60 {
		velocity, _ = carVelocity(defaultCarKind, velocity, forward, 0, 0, 1.0/60)
	}

	if abs32(velocity[0]) > 0.1 {
		t.Fatalf("sideways sliding should stop, got %v", velocity)
	}

	if velocity[1] != -2 {
		t.Fatalf("vertical velocity is left to physics, got %v", velocity)
	}
}

func TestCarSteering(t *testing.T) {
	forward := glm.Vec3f{0, 0, 1}

	if _, yaw := carVelocity(defaultCarKind, glm.Vec3f{}, forward, 0, 1, 1.0/60); yaw != 0 {
		t.Fatalf("a standing car does not turn, got %v", yaw)
	}

	_, ahead := carVelocity(defaultCarKind, glm.Vec3f{0, 0, 10}, forward, 0, 1, 1.0/60)
	_, back := carVelocity(defaultCarKind, glm.Vec3f{0, 0, -10}, forward, 0, 1, 1.0/60)
	if ahead <= 0 || back >= 0 {
		t.Fatalf("steering reverses when backing up, got %v and %v", ahead, back)
	}
}

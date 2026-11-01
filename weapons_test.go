package main

import "testing"

func TestTriggerFiresAtWeaponRate(t *testing.T) {
	arsenal := newArsenal()

	// the raise animation blocks the first shots
	shots := 0
	for range 60 {
		shots += arsenal.Trigger(1.0/60, true)
	}

	// one second minus the raise time at one shot per interval
	expected := int((1 - 0.125) / weapons[MachineGun].Interval)
	if shots < expected-1 || shots > expected+1 {
		t.Fatalf("expected about %d shots, got %d", expected, shots)
	}

	if arsenal.Ammo[MachineGun] != weapons[MachineGun].StartAmmo-shots {
		t.Fatalf("each shot uses one bullet")
	}
}

func TestTriggerStopsWithoutAmmo(t *testing.T) {
	arsenal := newArsenal()
	arsenal.Ammo[MachineGun] = 3

	shots := 0
	for range 120 {
		shots += arsenal.Trigger(1.0/60, true)
	}

	if shots != 3 || arsenal.Ammo[MachineGun] != 0 {
		t.Fatalf("expected 3 shots, got %d", shots)
	}
}

func TestGatlingSpinsUp(t *testing.T) {
	arsenal := newArsenal()
	arsenal.Give(Gatling)
	arsenal.Select(Gatling)

	count := func(frames int) int {
		shots := 0
		for range frames {
			shots += arsenal.Trigger(1.0/60, true)
		}
		return shots
	}

	// skip the raise animation
	arsenal.raise = 0

	first := count(15)
	count(60)
	spun := count(15)

	if float32(spun) < 1.8*float32(first) {
		t.Fatalf("a spun up gatling should fire much faster, got %d then %d", first, spun)
	}
}

func TestCycleSkipsEmptyAndMissingWeapons(t *testing.T) {
	arsenal := newArsenal()
	arsenal.Give(Shotgun)
	arsenal.Give(RocketLauncher)
	arsenal.Ammo[Shotgun] = 0

	arsenal.Cycle(1)
	if arsenal.Current != RocketLauncher {
		t.Fatalf("expected the rocket launcher, got %v", arsenal.Current)
	}

	arsenal.Cycle(1)
	if arsenal.Current != Shovel {
		t.Fatalf("expected the shovel, got %v", arsenal.Current)
	}

	arsenal.Cycle(1)
	if arsenal.Current != MachineGun {
		t.Fatalf("expected the machine gun, got %v", arsenal.Current)
	}
}

func TestExplosiveDamageFalloff(t *testing.T) {
	if d := explosiveDamage(100, 10, 0); d != 100 {
		t.Fatalf("full damage in the center, got %v", d)
	}

	if d := explosiveDamage(100, 10, 5); d != 50 {
		t.Fatalf("half damage at half radius, got %v", d)
	}

	if d := explosiveDamage(100, 10, 12); d != 0 {
		t.Fatalf("no damage outside, got %v", d)
	}
}

func TestShovelNeedsNoAmmo(t *testing.T) {
	arsenal := newArsenal()
	arsenal.Ammo[MachineGun] = 0

	// the empty machine gun switches to the shovel
	arsenal.Cycle(1)
	if arsenal.Current != Shovel {
		t.Fatalf("expected the shovel, got %v", arsenal.Current)
	}

	swings := 0
	for range 120 {
		swings += arsenal.Trigger(1.0/60, true)
	}

	expected := int((2 - 0.125) / weapons[Shovel].Interval)
	if swings < expected-1 || swings > expected+1 {
		t.Fatalf("expected about %d swings, got %d", expected, swings)
	}
}

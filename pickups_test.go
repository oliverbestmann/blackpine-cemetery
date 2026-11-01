package main

import (
	"math/rand/v2"
	"testing"
)

func TestRollDropBruteAlwaysDropsWeapon(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 100 {
		drop, ok := rollDrop(rng, ZombieBrute, 1)
		if !ok || drop.Kind != PickupWeapon || drop.Weapon == MachineGun || drop.Weapon >= Shovel {
			t.Fatalf("unexpected drop %+v %v", drop, ok)
		}
	}
}

func TestRollDropRate(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))

	drops := 0
	for range 10000 {
		if _, ok := rollDrop(rng, ZombieWalker, 1); ok {
			drops++
		}
	}

	if drops < 1500 || drops > 2500 {
		t.Fatalf("expected about 20%% drops, got %d of 10000", drops)
	}
}

func TestDropApply(t *testing.T) {
	arsenal := newArsenal()
	player := Player{Health: 50}

	Drop{Kind: PickupWeapon, Weapon: Shotgun}.apply(&arsenal, &player)
	if !arsenal.Owned[Shotgun] || arsenal.Current != Shotgun || arsenal.Ammo[Shotgun] != weapons[Shotgun].PickupAmmo {
		t.Fatalf("shotgun not given: %+v", arsenal)
	}

	before := arsenal.Ammo[Shotgun]
	Drop{Kind: PickupAmmo}.apply(&arsenal, &player)
	if arsenal.Ammo[Shotgun] <= before || arsenal.Ammo[RocketLauncher] != 0 {
		t.Fatalf("ammo only for owned weapons: %+v", arsenal.Ammo)
	}

	Drop{Kind: PickupHealth}.apply(&arsenal, &player)
	Drop{Kind: PickupHealth}.apply(&arsenal, &player)
	Drop{Kind: PickupHealth}.apply(&arsenal, &player)
	if player.Health != playerMaxHealth {
		t.Fatalf("health capped at max, got %v", player.Health)
	}
}

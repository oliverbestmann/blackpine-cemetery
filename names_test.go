package main

import (
	"slices"
	"testing"
)

func TestPlayerNames(t *testing.T) {
	for _, names := range [][]string{firstNames, lastNames} {
		unique := slices.Compact(slices.Sorted(slices.Values(names)))
		if len(names) != 100 || len(unique) != 100 {
			t.Fatalf("want 100 unique names, got %d of %d", len(unique), len(names))
		}
	}
}

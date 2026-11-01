//go:build !js

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// playerName is a random name, made once and kept in the user's config dir.
// Screenshot and pgo runs do not submit highscores.
var playerName = sync.OnceValue(func() string {
	if os.Getenv("ZOMBIES_SCREENSHOT") != "" || os.Getenv("BYKE_OFFSCREEN_DURATION") != "" {
		return ""
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return randomPlayerName()
	}

	path := filepath.Join(dir, highscoreGame, "player-name")
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return strings.TrimSpace(string(data))
	}

	name := randomPlayerName()

	err = os.MkdirAll(filepath.Dir(path), 0o755)
	if err == nil {
		err = os.WriteFile(path, []byte(name+"\n"), 0o644)
	}

	if err != nil {
		slog.Warn("Failed to save the player name", slog.Any("err", err))
	}

	return name
})

func highscoreGameId() string { return highscoreGame }

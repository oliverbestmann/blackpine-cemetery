//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

func postScore(game, player string, score int) ([]highscoreEntry, error) {
	client := http.Client{Timeout: 10 * time.Second}

	resp, err := client.Post(highscoreEndpoint(game, player, score), "", nil)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var entries []highscoreEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return entries, nil
}

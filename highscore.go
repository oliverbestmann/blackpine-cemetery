package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

const (
	highscoreURL     = "https://highscore.narf.zone/games/%s/highscore"
	highscoreGame    = "zombies-of-hollow-pines"
	highscoreEntries = 8
)

type highscoreEntry struct {
	Player string `json:"player"`
	Score  int    `json:"score"`
}

// Leaderboard is the hud text showing the highscores after the player died.
// It receives the server's answer from a background request.
type Leaderboard struct {
	Component[Leaderboard]
	result chan []highscoreEntry
}

// submitScoreSystem posts the score once the player died. It only runs with a
// player name, which only the web page asks for.
func submitScoreSystem(commands *Commands, stats *Stats) {
	player := playerName()
	if player == "" {
		return
	}

	score := stats.Score

	// buffered, the request finishes even if the round is gone already
	result := make(chan []highscoreEntry, 1)
	go func() {
		entries, err := postScore(highscoreGameId(), player, score)
		if err != nil {
			slog.Warn("Failed to submit highscore", slog.Any("err", err))
			return
		}

		result <- entries
	}()

	commands.Spawn(
		Round{},
		NewTransform(),
		HudPin{Offset: glm.Vec2f{0, 60}},
		AnchorTopCenter,
		Text{Size: 36, Color: ColorSRGB(1, 0.95, 0.85)},
		hudFont(),
		Leaderboard{result: result},
		RenderLayersOf(hudLayer),
	)
}

func postScore(game, player string, score int) ([]highscoreEntry, error) {
	query := url.Values{"player": {player}, "score": {fmt.Sprint(score)}}
	endpoint := fmt.Sprintf(highscoreURL, url.PathEscape(game)) + "?" + query.Encode()

	client := http.Client{Timeout: 10 * time.Second}

	resp, err := client.Post(endpoint, "", nil)
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

// leaderboardSystem shows the highscores once the server answered.
func leaderboardSystem(boards Query[struct {
	Board *Leaderboard
	Text  *Text
}],
) {
	for item := range boards.Items() {
		select {
		case entries := <-item.Board.result:
			item.Text.Text = formatLeaderboard(entries, playerName())
		default:
		}
	}
}

// formatLeaderboard lists the best entries. The player's entry is marked, and appended
// if it is not among them. The server lists each player once.
func formatLeaderboard(entries []highscoreEntry, player string) string {
	var lines []string
	lines = append(lines, "HIGHSCORE")

	for idx, entry := range entries {
		own := entry.Player == player
		if idx >= highscoreEntries {
			if !own {
				continue
			}

			lines = append(lines, "...")
		}

		line := fmt.Sprintf("%d. %s  %d pts", idx+1, entry.Player, entry.Score)
		if own {
			line = "> " + line + " <"
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

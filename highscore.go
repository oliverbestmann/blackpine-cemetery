package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

const (
	highscoreURL     = "https://highscore.narf.zone/games/%s/highscore"
	highscoreGame    = "blackpine-cemetery"
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
// player name, see playerName.
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
		HudPin{Offset: glm.V2f(0, 60)},
		AnchorTopCenter,
		Text{Size: 36, Color: ColorSRGB(1, 0.95, 0.85)},
		hudFont(),
		Leaderboard{result: result},
		RenderLayersOf(hudLayer),
	)
}

// highscoreEndpoint is the url to post a score to.
func highscoreEndpoint(game, player string, score int) string {
	query := url.Values{"player": {player}, "score": {fmt.Sprint(score)}}
	return fmt.Sprintf(highscoreURL, url.PathEscape(game)) + "?" + query.Encode()
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

package main

import (
	"time"

	. "github.com/oliverbestmann/byke"
)

// Stats are the numbers of the current round.
type Stats struct {
	Kills int
	Wave  int
	Score int

	achieved [len(achievements)]bool

	// counters for achievements
	meleeKills int
	blastKills int
	blastAt    time.Duration
	seenWave   int
	waveHurt   bool
	lastHealth float32

	// recent kills for multi kill messages
	recentKills []time.Duration

	messageText  string
	messageUntil time.Duration
}

// Show shows a big message in the screen center for a while.
func (s *Stats) Show(vt VirtualTime, text string, duration time.Duration) {
	s.messageText = text
	s.messageUntil = vt.Elapsed + duration
}

func (s *Stats) message(vt VirtualTime) string {
	if vt.Elapsed > s.messageUntil {
		return ""
	}

	return s.messageText
}

// multiKillWindow is the time in which kills count together.
const multiKillWindow = 1500 * time.Millisecond

// multiKillMessages are shown for this many kills within multiKillWindow.
var multiKillMessages = []struct {
	kills int
	text  string
}{
	{50, "APOCALYPTIC!!!"},
	{30, "CARNAGE!!"},
	{20, "MASSACRE!"},
	{12, "RAMPAGE!"},
	{7, "MEGA KILL"},
	{4, "MULTI KILL"},
}

// AddKill counts a kill. It returns the number of kills in the current multi kill,
// and the message to show, if any.
func (s *Stats) AddKill(now time.Duration) (int, string) {
	s.Kills++

	// forget kills outside the window
	keep := s.recentKills[:0]
	for _, at := range s.recentKills {
		if now-at < multiKillWindow {
			keep = append(keep, at)
		}
	}

	s.recentKills = append(keep, now)

	count := len(s.recentKills)
	for _, m := range multiKillMessages {
		if count == m.kills {
			return count, m.text
		}
	}

	return count, ""
}

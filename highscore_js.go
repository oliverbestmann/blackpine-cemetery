package main

import "syscall/js"

// playerName returns the name entered on the web page before the game started.
func playerName() string {
	if name := js.Global().Get("playerName"); name.Type() == js.TypeString {
		return name.String()
	}

	return ""
}

// highscoreGameId keeps scores from local test runs out of the real highscore.
func highscoreGameId() string {
	switch js.Global().Get("location").Get("hostname").String() {
	case "localhost", "127.0.0.1":
		return highscoreGame + "-dev"
	}

	return highscoreGame
}

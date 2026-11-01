package main

import (
	"strings"
	"sync"
	"syscall/js"
)

// playerName returns the name entered on the web page before the game started.
// Without one, the player gets a random name, which the page offers again next time.
var playerName = sync.OnceValue(func() string {
	if name := js.Global().Get("playerName"); name.Type() == js.TypeString && strings.TrimSpace(name.String()) != "" {
		return strings.TrimSpace(name.String())
	}

	name := randomPlayerName()
	js.Global().Get("localStorage").Call("setItem", "playerName", name)
	return name
})

// highscoreGameId keeps scores from local test runs out of the real highscore.
func highscoreGameId() string {
	switch js.Global().Get("location").Get("hostname").String() {
	case "localhost", "127.0.0.1":
		return highscoreGame + "-dev"
	}

	return highscoreGame
}

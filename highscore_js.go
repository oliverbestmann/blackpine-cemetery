package main

import (
	"encoding/json"
	"fmt"
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

// postScore uses the browser's fetch, TinyGo's net/http does not work in wasm.
func postScore(game, player string, score int) ([]highscoreEntry, error) {
	type result struct {
		body string
		err  error
	}

	done := make(chan result, 1)

	var onBody, onResponse, onError js.Func
	onBody = js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- result{body: args[0].String()}
		return nil
	})

	onResponse = js.FuncOf(func(this js.Value, args []js.Value) any {
		resp := args[0]
		if !resp.Get("ok").Bool() {
			done <- result{err: fmt.Errorf("unexpected status %d", resp.Get("status").Int())}
			return nil
		}

		resp.Call("text").Call("then", onBody, onError)
		return nil
	})

	onError = js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- result{err: fmt.Errorf("fetch: %s", args[0].Call("toString").String())}
		return nil
	})

	defer onBody.Release()
	defer onResponse.Release()
	defer onError.Release()

	options := js.Global().Get("Object").New()
	options.Set("method", "POST")

	js.Global().Call("fetch", highscoreEndpoint(game, player, score), options).Call("then", onResponse, onError)

	res := <-done
	if res.err != nil {
		return nil, res.err
	}

	var entries []highscoreEntry
	if err := json.Unmarshal([]byte(res.body), &entries); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return entries, nil
}

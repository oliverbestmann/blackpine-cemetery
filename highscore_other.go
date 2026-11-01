//go:build !js

package main

// playerName is empty, only the web page asks for a name and submits highscores.
func playerName() string { return "" }

func highscoreGameId() string { return highscoreGame + "-dev" }

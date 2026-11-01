package main

import "syscall/js"

// hideLoadingIndicator removes the loading overlay of index.html.
func hideLoadingIndicator() {
	if el := js.Global().Get("document").Call("getElementById", "loading"); el.Truthy() {
		el.Call("remove")
	}
}

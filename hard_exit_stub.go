//go:build !cgo_whisper

package main

// hardExit is a no-op when the Metal/whisper backend is not linked.
func hardExit() {}

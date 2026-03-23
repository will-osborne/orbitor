//go:build cgo_whisper

package main

/*
#include <unistd.h>
*/
import "C"

// hardExit calls _exit(0) to terminate the process immediately, bypassing
// C++ static destructors registered via __cxa_atexit. This prevents the
// ggml-metal device teardown assertion from firing during process exit.
func hardExit() {
	C._exit(0)
}

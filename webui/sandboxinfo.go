package main

import (
	"os/exec"
	"runtime"
)

// sandboxBackend reports the OS sandbox backend `ar` would use on this host
// and whether its binary is present — the same zero-dep mirroring pattern as
// schedule.go (webui must not import internal packages). Presence-detection
// only: a found binary can still fail ar's runtime probe, in which case
// execute-class work fails closed; the wording therefore stays "detected",
// never "working".
func sandboxBackend() (name string, detected bool) {
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("sandbox-exec")
		return "seatbelt", err == nil
	case "linux":
		_, err := exec.LookPath("bwrap")
		return "bubblewrap", err == nil
	default:
		return "", false
	}
}

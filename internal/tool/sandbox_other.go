//go:build !darwin && !linux

package tool

import (
	"fmt"
	"os/exec"
)

func platformSandboxProbe(bool) (string, error) {
	return "unsupported", fmt.Errorf("no filesystem sandbox backend for this platform")
}

func platformSandboxCommand(string, string, []string, []sandboxDeny, bool) (*exec.Cmd, error) {
	return nil, fmt.Errorf("no filesystem sandbox backend for this platform")
}

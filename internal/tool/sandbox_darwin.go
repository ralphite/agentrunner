//go:build darwin

package tool

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func platformSandboxProbe(networkNone bool) (string, error) {
	bin, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return "sandbox-exec", fmt.Errorf("sandbox-exec unavailable: %w", err)
	}
	profile := "(version 1)\n(allow default)\n"
	if networkNone {
		profile += "(deny network*)\n"
	}
	if out, err := exec.Command(bin, "-p", profile, "/usr/bin/true").CombinedOutput(); err != nil {
		return "sandbox-exec", fmt.Errorf("sandbox-exec probe: %v: %s", err, bytes.TrimSpace(out))
	}
	return "sandbox-exec", nil
}

func platformSandboxCommand(root, command string, writable []string, denied []sandboxDeny, networkNone bool) (*exec.Cmd, error) {
	bin, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return nil, fmt.Errorf("sandbox-exec unavailable: %w", err)
	}
	var p strings.Builder
	p.WriteString("(version 1)\n(deny default)\n(import \"system.sb\")\n")
	p.WriteString("(allow process*)\n")
	p.WriteString("(allow file-read* (subpath \"/System\") (subpath \"/usr\") (subpath \"/bin\") (subpath \"/sbin\") (subpath \"/Library\") (subpath \"/opt\") (subpath \"/usr/local\") (subpath \"/nix/store\"))\n")
	if networkNone {
		p.WriteString("(deny network*)\n")
	} else {
		p.WriteString("(allow network*)\n")
	}
	seenAncestor := map[string]bool{}
	for _, path := range writable {
		path = filepath.Clean(path)
		p.WriteString("(allow file-read* file-write* (literal ")
		p.WriteString(strconv.Quote(path))
		p.WriteString(") (subpath ")
		p.WriteString(strconv.Quote(path))
		p.WriteString("))\n")
		for cur := filepath.Dir(path); cur != "/" && cur != "."; cur = filepath.Dir(cur) {
			seenAncestor[cur] = true
		}
	}
	ancestors := make([]string, 0, len(seenAncestor))
	for path := range seenAncestor {
		ancestors = append(ancestors, path)
	}
	sort.Strings(ancestors)
	for _, path := range ancestors {
		p.WriteString("(allow file-read-metadata (literal ")
		p.WriteString(strconv.Quote(path))
		p.WriteString("))\n")
	}
	for _, d := range denied {
		filter := "literal"
		if d.Subpath {
			filter = "subpath"
		}
		p.WriteString("(deny file-read* (")
		p.WriteString(filter)
		p.WriteByte(' ')
		p.WriteString(strconv.Quote(filepath.Clean(d.Path)))
		p.WriteString("))\n")
	}
	cmd := exec.Command(bin, "-p", p.String(), "bash", "-c", command)
	cmd.Dir = root
	return cmd, nil
}

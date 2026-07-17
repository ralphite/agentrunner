package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSandboxBackendDetection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("PATH-injection detection test is linux-shaped")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "bwrap"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	name, detected := sandboxBackend()
	if name != "bubblewrap" || !detected {
		t.Fatalf("with bwrap on PATH: got (%q,%v), want (bubblewrap,true)", name, detected)
	}
	t.Setenv("PATH", t.TempDir())
	if name, detected = sandboxBackend(); name != "bubblewrap" || detected {
		t.Fatalf("with empty PATH: got (%q,%v), want (bubblewrap,false)", name, detected)
	}
}

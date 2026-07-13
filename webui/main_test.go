package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTitleFromIDStripsLegacyAndCurrentEntropySuffix(t *testing.T) {
	for _, id := range []string{
		"20260712-120000-fix-upload-abcd",
		"20260712-120000-fix-upload-0123456789abcdef",
	} {
		if got := titleFromID(id); got != "fix upload" {
			t.Errorf("titleFromID(%q) = %q", id, got)
		}
	}
}

func TestResolveARPathExplicitWins(t *testing.T) {
	// An explicit -ar is always returned verbatim, sibling or not.
	if got := resolveARPath("/custom/ar", true); got != "/custom/ar" {
		t.Fatalf("explicit -ar not respected: %q", got)
	}
}

func TestARSiblingPreferredOverPATH(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "arwebui")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(dir, "ar")
	if err := os.WriteFile(sibling, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// An executable `ar` next to arwebui shadows the PATH fallback ("ar"), which
	// on Linux would otherwise resolve to GNU binutils' archiver.
	if got := arSiblingOr(exe, "ar"); got != sibling {
		t.Fatalf("sibling ar not preferred: got %q, want %q", got, sibling)
	}
}

func TestARSiblingFallsBackWhenMissing(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "arwebui")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No sibling ar at all → keep the PATH fallback unchanged.
	if got := arSiblingOr(exe, "ar"); got != "ar" {
		t.Fatalf("missing sibling should fall back to %q, got %q", "ar", got)
	}
}

func TestARSiblingIgnoresNonExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "arwebui")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-executable file named ar (e.g. a stray data file) must not be picked.
	if err := os.WriteFile(filepath.Join(dir, "ar"), []byte("not a binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := arSiblingOr(exe, "ar"); got != "ar" {
		t.Fatalf("non-executable ar should be ignored, got %q", got)
	}
}

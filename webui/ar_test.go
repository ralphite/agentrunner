package main

import (
	"path/filepath"
	"testing"
)

func TestParseSessionID(t *testing.T) {
	cases := []struct {
		name string
		res  arResult
		want string
	}{
		{
			// `ar new` announces the id on stderr; the reply is on stdout.
			name: "new: id on stderr",
			res: arResult{
				Stdout: "\n[gen-step 1]\n收到，请指示。\n",
				Stderr: "session 20260708-230920-task-5913\n(session 20260708-230920-task-5913 is waiting — continue: ...)\n",
			},
			want: "20260708-230920-task-5913",
		},
		{
			name: "fork: id on stdout",
			res:  arResult{Stdout: "session 20260708-231108-fork-ab12\n"},
			want: "20260708-231108-fork-ab12",
		},
		{
			// fork prints `forked <PARENT> @ <bar>` on stderr and
			// `session <NEW>` on stdout — the new id must win, not the parent.
			name: "fork: parent on stderr, new on stdout",
			res: arResult{
				Stdout: "session 20260709-024710-fork-bar-t1-df98\n",
				Stderr: "forked 20260708-224108-gin-gonic-gin-08e2 @ bar-t1\n",
			},
			want: "20260709-024710-fork-bar-t1-df98",
		},
		{
			name: "bare id anywhere",
			res:  arResult{Stderr: "created 20260708-010203-task-0001 ok"},
			want: "20260708-010203-task-0001",
		},
		{
			name: "none",
			res:  arResult{Stdout: "no session here"},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseSessionID(c.res); got != c.want {
				t.Fatalf("parseSessionID = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDaemonUnreachable(t *testing.T) {
	unreachable := []string{
		"agentrunner: daemon dial: dial unix /x/daemon.sock: connect: no such file or directory",
		"error (no daemon running? start one with: agentrunner daemon --detach)",
		"failed: is the daemon running?",
	}
	for _, s := range unreachable {
		if !daemonUnreachable(s) {
			t.Errorf("expected unreachable for %q", s)
		}
	}
	reachable := []string{
		"no session matches \"__arwebui_probe__\"",
		"unknown session",
		"",
	}
	for _, s := range reachable {
		if daemonUnreachable(s) {
			t.Errorf("expected reachable for %q", s)
		}
	}
}

func TestValidID(t *testing.T) {
	ok := []string{"20260708-230920-task-5913", "call_1_0", "bar-final", "a.b_c-1"}
	for _, s := range ok {
		if !validID(s) {
			t.Errorf("expected valid: %q", s)
		}
	}
	bad := []string{"", "a b", "x;y", "$(rm)", "a/b"}
	for _, s := range bad {
		if validID(s) {
			t.Errorf("expected invalid: %q", s)
		}
	}
}

func TestParseBarrierID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"barrier bar-m37\nsnapshot 1a2b3c4\n", "bar-m37"},
		{"snapshot 1a2b3c4\n", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := parseBarrierID(c.in); got != c.want {
			t.Errorf("parseBarrierID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMetaStoreMergeHydratesJournalMetadataWithoutReplacingTitle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	store := newMetaStore(path)
	store.set("s1", "", "My renamed task")
	store.merge(map[string]sessionMeta{
		"s1": {Workspace: "/tmp/project", Title: "Journal opening task"},
		"s2": {Workspace: "/tmp/other", Title: "External task"},
	})

	if got := store.get("s1"); got.Workspace != "/tmp/project" || got.Title != "My renamed task" {
		t.Fatalf("s1 metadata = %+v", got)
	}
	if got := store.get("s2"); got.Workspace != "/tmp/other" || got.Title != "External task" {
		t.Fatalf("s2 metadata = %+v", got)
	}

	reloaded := newMetaStore(path)
	if got := reloaded.get("s2"); got.Workspace != "/tmp/other" || got.Title != "External task" {
		t.Fatalf("reloaded metadata = %+v", got)
	}
}

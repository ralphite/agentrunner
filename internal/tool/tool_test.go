package tool

import (
	"reflect"
	"strings"
	"testing"
)

func TestRegistryLoads(t *testing.T) {
	want := []string{"artifacts_list", "artifacts_read", "ask_user", "bash", "edit_file", "exit_plan_mode", "finish_series", "glob", "goal_complete", "goal_status", "grep", "handoff_agent", "keyword_search", "kill", "output", "progress_update", "publish_artifact", "publish_note", "read_file", "read_notes", "schedule_next", "send_message", "skill", "spawn_agent", "web_fetch", "write_file"}
	if got := Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}

	def, ok := Get("bash")
	if !ok || def.Class != ClassExecute {
		t.Errorf("bash def = %+v, ok=%v", def, ok)
	}
	if def, _ := Get("read_file"); def.Class != ClassRead {
		t.Errorf("read_file class = %q", def.Class)
	}
	if def, _ := Get("edit_file"); def.Class != ClassEdit {
		t.Errorf("edit_file class = %q", def.Class)
	}
	if def, _ := Get("exit_plan_mode"); def.Class != ClassWait {
		t.Errorf("exit_plan_mode class = %q", def.Class)
	}
}

// TestSpawnAndOutputCarryFireAndYieldContract guards INC-85: the sub-agent
// orchestration contract (spawn is fire-and-yield; a finished result wakes you
// as a message; no polling or sleeping to wait) must stay surfaced in the
// model-facing tool descriptions, or weak models regress to output+sleep
// busy-waiting (session 20260721-070616, gemini-flash-latest).
func TestSpawnAndOutputCarryFireAndYieldContract(t *testing.T) {
	for _, name := range []string{"spawn_agent", "output"} {
		def, ok := Get(name)
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		d := strings.ToLower(def.Description)
		if !strings.Contains(d, "sleep") {
			t.Errorf("%s description dropped the anti-sleep guidance: %q", name, def.Description)
		}
		if !strings.Contains(d, "wake") && !strings.Contains(d, "end your turn") {
			t.Errorf("%s description dropped the auto-wake / end-your-turn contract: %q", name, def.Description)
		}
	}
}

func TestProviderDefs(t *testing.T) {
	defs, err := ProviderDefs([]string{"read_file", "bash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 2 || defs[0].Name != "read_file" {
		t.Fatalf("defs = %+v", defs)
	}
	if !strings.Contains(string(defs[0].InputSchema), `"path"`) {
		t.Errorf("schema not carried: %s", defs[0].InputSchema)
	}

	if _, err := ProviderDefs([]string{"nope"}); err == nil {
		t.Fatal("expected unknown tool error")
	}
}

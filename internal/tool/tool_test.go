package tool

import (
	"reflect"
	"strings"
	"testing"
)

func TestRegistryLoads(t *testing.T) {
	want := []string{"bash", "edit_file", "exit_plan_mode", "handoff_agent", "publish_artifact", "publish_note", "read_file", "read_notes", "spawn_agent", "task_kill", "task_output"}
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

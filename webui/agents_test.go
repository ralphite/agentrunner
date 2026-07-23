package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleAgentsProjectsSharedRuntimeCatalog(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	reply := `[{"name":"dev","description":"Dev","source":"shipped","yaml":"name: dev\nsystem_prompt: test\ntools: []\n"},{"name":"plan","source":"shipped","yaml":"name: plan\nsystem_prompt: plan\ntools: []\n"}]`
	s := &server{arPath: writeFakeAR(t, argsFile, reply)}
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rr := httptest.NewRecorder()
	s.handleAgents(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var entries []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		YAML   string `json:"yaml"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "dev" || entries[1].Name != "plan" {
		t.Fatalf("catalog order = %+v", entries)
	}
	for _, entry := range entries {
		if entry.Source != "shipped" || strings.Contains(entry.YAML, "\nmodel:") {
			t.Fatalf("entry %q = %+v", entry.Name, entry)
		}
	}
	rawArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(rawArgs) != "agents\n--json\n" {
		t.Fatalf("args=%q", rawArgs)
	}
}

func TestWebModelInputIsExplicitAndValidated(t *testing.T) {
	args, err := (modelInput{Provider: "anthropic", Model: "claude-sonnet-5", Effort: "high"}).args()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(args, " "); got != "--model anthropic/claude-sonnet-5 --effort high" {
		t.Fatalf("args=%q", got)
	}
	for _, input := range []modelInput{
		{},
		{Provider: "gemini", Model: "gemini-flash-latest"},
		{Provider: "gemini", Model: "gemini-flash-latest", Effort: "maximum"},
		{Provider: "bad/provider", Model: "x", Effort: "medium"},
	} {
		if _, err := input.args(); err == nil {
			t.Fatalf("accepted invalid input %+v", input)
		}
	}
}

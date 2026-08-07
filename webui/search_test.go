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

func searchServer(t *testing.T, reply string) (*server, string) {
	t.Helper()
	argsFile := filepath.Join(t.TempDir(), "args")
	return &server{arPath: writeFakeAR(t, argsFile, reply)}, argsFile
}

func TestHandleSearchForwardsQueryAndKeepsBounds(t *testing.T) {
	reply := `{"query":"静止模型","matches":[{"session":"sess-a","title":"设计","role":"assistant","snippet":"…静止模型…","kind":"message"}],"sessions_scanned":12,"sessions_skipped":3,"truncated":true,"limit":30}`
	s, argsFile := searchServer(t, reply)

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=%E9%9D%99%E6%AD%A2%E6%A8%A1%E5%9E%8B&limit=5", nil)
	rr := httptest.NewRecorder()
	s.handleSearch(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	// The bounds are part of the answer — a palette that drops them turns
	// "we stopped early" into "there is nothing there".
	if out["truncated"] != true {
		t.Errorf("truncated dropped: %v", out["truncated"])
	}
	if out["sessionsScanned"] != float64(12) || out["sessions_scanned"] != float64(12) {
		t.Errorf("scanned bounds not exposed in both cases: %v / %v",
			out["sessionsScanned"], out["sessions_scanned"])
	}
	if out["sessionsSkipped"] != float64(3) {
		t.Errorf("sessionsSkipped = %v, want 3", out["sessionsSkipped"])
	}
	matches, _ := out["matches"].([]any)
	if len(matches) != 1 {
		t.Fatalf("matches = %v", out["matches"])
	}

	rawArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(rawArgs)
	if !strings.Contains(args, "search") || !strings.Contains(args, "--json") {
		t.Errorf("did not shell out to `ar search --json`: %q", args)
	}
	if !strings.Contains(args, "静止模型") {
		t.Errorf("query not forwarded verbatim: %q", args)
	}
	if !strings.Contains(args, "--limit") || !strings.Contains(args, "5") {
		t.Errorf("limit not forwarded: %q", args)
	}
	// The disclosure boundary must not be reachable from a URL.
	if strings.Contains(args, "--include-tools") {
		t.Errorf("web surface passed --include-tools: %q", args)
	}
}

func TestHandleSearchRejectsBadInput(t *testing.T) {
	s, _ := searchServer(t, `{"query":"x","matches":[]}`)

	for _, target := range []string{"/api/search", "/api/search?q=", "/api/search?q=x&limit=0", "/api/search?q=x&limit=999", "/api/search?q=x&limit=abc"} {
		rr := httptest.NewRecorder()
		s.handleSearch(rr, httptest.NewRequest(http.MethodGet, target, nil))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d, want 400", target, rr.Code)
		}
	}
}

// An empty result must serialize as [] rather than null: a palette iterating
// the field should not have to special-case a missing array.
func TestHandleSearchEmptyMatchesIsArray(t *testing.T) {
	s, _ := searchServer(t, `{"query":"nothing","sessions_scanned":4}`)
	rr := httptest.NewRecorder()
	s.handleSearch(rr, httptest.NewRequest(http.MethodGet, "/api/search?q=nothing", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"matches":[]`) {
		t.Errorf("empty matches not an array: %s", rr.Body.String())
	}
}

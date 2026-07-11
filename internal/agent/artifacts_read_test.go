package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/store"
)

// artifactsFixture publishes two text versions and one binary stream into a
// real CAS store and returns the loop plus the fold-truth published map.
func artifactsFixture(t *testing.T) (*Loop, map[string]int) {
	t.Helper()
	as, err := store.OpenArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := as.Publish("report", []byte("v1 报告 first")); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Publish("report", []byte("v2 报告 second — 长一点的正文")); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Publish("blob", []byte{0xff, 0xfe, 0x00, 0x01}); err != nil {
		t.Fatal(err)
	}
	return &Loop{Artifacts: as}, map[string]int{"report": 2, "blob": 1}
}

func TestArtifactsListShowsFoldTruth(t *testing.T) {
	l, published := artifactsFixture(t)
	res := l.runArtifactsList(published)
	if res.IsError {
		t.Fatalf("list: %s", res.Payload)
	}
	var out struct {
		Count     int `json:"count"`
		Artifacts []struct {
			Stream  string `json:"stream"`
			Version int    `json:"version"`
			Bytes   int    `json:"bytes"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(res.Payload, &out); err != nil || out.Count != 2 {
		t.Fatalf("want 2 streams: %s", res.Payload)
	}
	// Sorted by stream: blob then report; report shows the LATEST version.
	if out.Artifacts[1].Stream != "report" || out.Artifacts[1].Version != 2 || out.Artifacts[1].Bytes == 0 {
		t.Fatalf("report row wrong: %+v", out.Artifacts[1])
	}
	// A store stream missing from the fold map (orphan) must not appear.
	res = l.runArtifactsList(map[string]int{"report": 2})
	if strings.Contains(string(res.Payload), "blob") {
		t.Fatalf("orphan stream leaked into list: %s", res.Payload)
	}
}

func TestArtifactsReadFullVersionAndPaging(t *testing.T) {
	l, published := artifactsFixture(t)
	// Latest by default.
	res := l.runArtifactsRead(published, json.RawMessage(`{"stream":"report"}`))
	if res.IsError || !strings.Contains(string(res.Payload), "v2 报告") {
		t.Fatalf("latest read: %s", res.Payload)
	}
	// Historic version.
	res = l.runArtifactsRead(published, json.RawMessage(`{"stream":"report","version":1}`))
	if res.IsError || !strings.Contains(string(res.Payload), "v1 报告 first") {
		t.Fatalf("v1 read: %s", res.Payload)
	}
	// Paging: tiny window walks the content without splitting runes.
	var got strings.Builder
	off := 0
	for i := 0; i < 100; i++ {
		res = l.runArtifactsRead(published, json.RawMessage(
			`{"stream":"report","offset":`+itoaJSON(off)+`,"max_bytes":7}`))
		if res.IsError {
			t.Fatalf("page at %d: %s", off, res.Payload)
		}
		var page struct {
			Content    string `json:"content"`
			NextOffset *int   `json:"next_offset"`
			TotalBytes int    `json:"total_bytes"`
		}
		if err := json.Unmarshal(res.Payload, &page); err != nil {
			t.Fatal(err)
		}
		got.WriteString(page.Content)
		if page.NextOffset == nil {
			break
		}
		off = *page.NextOffset
	}
	if got.String() != "v2 报告 second — 长一点的正文" {
		t.Fatalf("paged reassembly mismatch: %q", got.String())
	}
}

func TestArtifactsReadEdges(t *testing.T) {
	l, published := artifactsFixture(t)
	for name, args := range map[string]string{
		"unknown stream": `{"stream":"nope"}`,
		"bad version":    `{"stream":"report","version":9}`,
		"bad offset":     `{"stream":"report","offset":100000}`,
		"missing stream": `{}`,
	} {
		if res := l.runArtifactsRead(published, json.RawMessage(args)); !res.IsError {
			t.Errorf("%s: want error, got %s", name, res.Payload)
		}
	}
	// Binary content answers metadata, not bytes.
	res := l.runArtifactsRead(published, json.RawMessage(`{"stream":"blob"}`))
	if res.IsError || !strings.Contains(string(res.Payload), `"binary":true`) {
		t.Fatalf("binary read: %s", res.Payload)
	}
	// No store wired at all.
	bare := &Loop{}
	if res := bare.runArtifactsList(published); !res.IsError {
		t.Fatal("no-store list must error")
	}
}

func itoaJSON(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

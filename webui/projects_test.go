package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestProjectStoreRoundTrip pins the on-disk contract: entries created through
// one store instance are visible to a fresh instance reading the same file
// (the 8809/QA-port sharing story), and update/remove persist.
func TestProjectStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fA := filepath.Join(dir, "a")
	fB := filepath.Join(dir, "b")
	for _, d := range []string{fA, fB} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "projects.json")

	s1 := newProjectStore(path)
	def, err := s1.create("Orca", []string{fA})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if def.ID == "" || def.CreatedAt == 0 {
		t.Fatalf("create returned incomplete def: %+v", def)
	}

	// A second process (fresh store, no shared memory) sees the entry.
	s2 := newProjectStore(path)
	got := s2.list()
	if len(got) != 1 || got[0].Name != "Orca" || got[0].Folders[0] != fA {
		t.Fatalf("fresh store list = %+v", got)
	}

	name := "Orca dev"
	if _, err := s2.update(def.ID, &name, []string{fA, fB}, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	got = s1.list()
	if len(got) != 1 || got[0].Name != "Orca dev" || len(got[0].Folders) != 2 {
		t.Fatalf("after update, other store sees %+v", got)
	}

	if err := s1.remove(def.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := s2.list(); len(got) != 0 {
		t.Fatalf("after remove, list = %+v", got)
	}
	if err := s1.remove(def.ID); err == nil {
		t.Fatalf("second remove should fail")
	}
}

// TestProjectStoreConcurrentWritersKeepEveryEntry is the flock twin: two store
// instances that share NOTHING but the file path (simulating two arwebui
// processes) hammer create concurrently; every entry must survive. Without the
// separate-lock-file discipline the rename dance loses writes silently.
func TestProjectStoreConcurrentWritersKeepEveryEntry(t *testing.T) {
	dir := t.TempDir()
	const n = 8
	folders := make([]string, 2*n)
	for i := range folders {
		folders[i] = filepath.Join(dir, fmt.Sprintf("f%02d", i))
		if err := os.Mkdir(folders[i], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "projects.json")
	sA := newProjectStore(path)
	sB := newProjectStore(path)

	var wg sync.WaitGroup
	errs := make(chan error, 2*n)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_, err := sA.create(fmt.Sprintf("A%d", i), []string{folders[i]})
			errs <- err
		}(i)
		go func(i int) {
			defer wg.Done()
			_, err := sB.create(fmt.Sprintf("B%d", i), []string{folders[n+i]})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}
	got := newProjectStore(path).list()
	if len(got) != 2*n {
		t.Fatalf("lost writes: have %d entries, want %d", len(got), 2*n)
	}
}

// TestProjectStoreRejectsDuplicateFolder pins folder exclusivity, including
// the canonical comparison that makes a symlinked spelling of an existing
// folder collide (the /tmp vs /private/tmp class on macOS).
func TestProjectStoreRejectsDuplicateFolder(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	s := newProjectStore(filepath.Join(dir, "projects.json"))
	if _, err := s.create("one", []string{real}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.create("two", []string{link}); err == nil {
		t.Fatalf("symlinked duplicate folder should be rejected")
	} else if !strings.Contains(err.Error(), `"one"`) {
		t.Fatalf("error should name the owning project: %v", err)
	}
	if _, err := s.create("three", []string{filepath.Join(dir, "real"), filepath.Join(dir, "real") + "/"}); err == nil {
		t.Fatalf("within-submit duplicate should be rejected")
	}
}

// TestProjectStoreKeepsMissingFolderEditable is the 洞 2 twin: a folder that
// vanished from disk must not brick the project — the user can still rename it
// and remove the dead folder. Only genuinely NEW folders get the exists check.
func TestProjectStoreKeepsMissingFolderEditable(t *testing.T) {
	dir := t.TempDir()
	alive := filepath.Join(dir, "alive")
	doomed := filepath.Join(dir, "doomed")
	for _, d := range []string{alive, doomed} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := newProjectStore(filepath.Join(dir, "projects.json"))
	def, err := s.create("p", []string{alive, doomed})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.RemoveAll(doomed); err != nil {
		t.Fatal(err)
	}

	// Rename with the dead folder still listed: must succeed.
	name := "renamed"
	if _, err := s.update(def.ID, &name, []string{alive, doomed}, nil); err != nil {
		t.Fatalf("rename with dead folder: %v", err)
	}
	// Removing the dead folder: must succeed.
	if _, err := s.update(def.ID, nil, []string{alive}, nil); err != nil {
		t.Fatalf("drop dead folder: %v", err)
	}
	// Adding a nonexistent folder is still refused.
	if _, err := s.update(def.ID, nil, []string{alive, filepath.Join(dir, "never")}, nil); err == nil {
		t.Fatalf("adding a nonexistent folder should fail")
	}
}

func projectTestServer(t *testing.T) *server {
	t.Helper()
	return &server{
		meta:     newMetaStore(""),
		projects: newProjectStore(filepath.Join(t.TempDir(), "projects.json")),
		spellings: func(context.Context) map[string]string {
			return nil // no journal in these tests; folders keep their spelling
		},
	}
}

func postProjectJSON(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// TestProjectsListPayloadCarriesOverlaysAndProjects pins the GET shape: the
// combined payload holds the overlay map AND the registry, with Missing
// computed for folders gone from disk.
func TestProjectsListPayloadCarriesOverlaysAndProjects(t *testing.T) {
	s := projectTestServer(t)
	dir := t.TempDir()
	fA := filepath.Join(dir, "a")
	fGone := filepath.Join(dir, "gone")
	for _, d := range []string{fA, fGone} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	rr := postProjectJSON(t, s.handleProjectCreate,
		fmt.Sprintf(`{"name":"P","folders":[%q,%q]}`, fA, fGone))
	if rr.Code != 200 {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	if err := os.RemoveAll(fGone); err != nil {
		t.Fatal(err)
	}
	name := "Custom"
	s.meta.setProject("/some/derived", &name, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	lr := httptest.NewRecorder()
	s.handleProjectsList(lr, req)
	var payload projectsPayload
	if err := json.Unmarshal(lr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.Overlays["/some/derived"].DisplayName != "Custom" {
		t.Fatalf("overlay missing: %+v", payload.Overlays)
	}
	if len(payload.Projects) != 1 || payload.Projects[0].Name != "P" {
		t.Fatalf("projects: %+v", payload.Projects)
	}
	if got := payload.Projects[0].Missing; len(got) != 1 || got[0] != fGone {
		t.Fatalf("missing = %v, want [%s]", got, fGone)
	}
}

// TestProjectValidationRejectsBadInput sweeps the 400 surface.
func TestProjectValidationRejectsBadInput(t *testing.T) {
	s := projectTestServer(t)
	ok := t.TempDir()
	many := make([]string, maxFoldersPerProj+1)
	for i := range many {
		many[i] = fmt.Sprintf("%q", filepath.Join(ok, fmt.Sprintf("x%d", i)))
	}
	cases := []struct{ name, body string }{
		{"empty name", fmt.Sprintf(`{"name":"  ","folders":[%q]}`, ok)},
		{"long name", fmt.Sprintf(`{"name":%q,"folders":[%q]}`, strings.Repeat("x", 201), ok)},
		{"newline name", fmt.Sprintf(`{"name":"a\nb","folders":[%q]}`, ok)},
		{"no folders", `{"name":"p","folders":[]}`},
		{"relative folder", `{"name":"p","folders":["not/abs"]}`},
		{"nonexistent folder", fmt.Sprintf(`{"name":"p","folders":[%q]}`, filepath.Join(ok, "nope"))},
		{"too many folders", fmt.Sprintf(`{"name":"p","folders":[%s]}`, strings.Join(many, ","))},
	}
	for _, c := range cases {
		if rr := postProjectJSON(t, s.handleProjectCreate, c.body); rr.Code != 400 {
			t.Fatalf("%s: code %d, body %s", c.name, rr.Code, rr.Body.String())
		}
	}
	if rr := postProjectJSON(t, s.handleProjectSave, `{"id":"","name":"x"}`); rr.Code != 400 {
		t.Fatalf("save without id: %d", rr.Code)
	}
	if rr := postProjectJSON(t, s.handleProjectSave, `{"id":"p-1-ffff","name":"x"}`); rr.Code != 400 {
		t.Fatalf("save unknown id: %d", rr.Code)
	}
	if rr := postProjectJSON(t, s.handleProjectDelete, `{"id":"p-1-ffff"}`); rr.Code != 400 {
		t.Fatalf("delete unknown id: %d", rr.Code)
	}
}

// TestProjectCreateUpgradeMigratesOverlay is the 洞 5 twin: creating a project
// from folders that carried derived-group overlay state migrates pin/fold onto
// the project key and CLEARS the per-folder entries — most importantly
// removed:true, which would otherwise resurrect the derived group hidden after
// the project is deleted.
func TestProjectCreateUpgradeMigratesOverlay(t *testing.T) {
	s := projectTestServer(t)
	dir := t.TempDir()
	fA := filepath.Join(dir, "a")
	if err := os.Mkdir(fA, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "Old name"
	yes := true
	s.meta.setProject(fA, &name, nil, &yes, &yes) // pinned + removed on the derived group

	rr := postProjectJSON(t, s.handleProjectCreate, fmt.Sprintf(`{"name":"New","folders":[%q]}`, fA))
	if rr.Code != 200 {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		projectsPayload
		Created projectDef `json:"created"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if _, still := resp.Overlays[fA]; still {
		t.Fatalf("derived overlay for %s should be cleared, got %+v", fA, resp.Overlays[fA])
	}
	if !resp.Overlays[projectOverlayKey(resp.Created.ID)].Pinned {
		t.Fatalf("pin should migrate onto the project key: %+v", resp.Overlays)
	}

	// Deleting the project must NOT leave the derived group hidden.
	dr := postProjectJSON(t, s.handleProjectDelete, fmt.Sprintf(`{"id":%q}`, resp.Created.ID))
	if dr.Code != 200 {
		t.Fatalf("delete: %d %s", dr.Code, dr.Body.String())
	}
	var after projectsPayload
	if err := json.Unmarshal(dr.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Overlays[fA].Removed {
		t.Fatalf("derived group came back hidden: %+v", after.Overlays[fA])
	}
	if len(after.Projects) != 0 {
		t.Fatalf("registry should be empty: %+v", after.Projects)
	}
}

// TestProjectCreateNormalizesToJournalSpelling is the 洞 4 twin: a folder that
// names the same directory as a journal workspace is stored in the JOURNAL's
// spelling, so the frontend's exact-match grouping actually claims the
// sessions.
func TestProjectCreateNormalizesToJournalSpelling(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	s := projectTestServer(t)
	s.spellings = func(context.Context) map[string]string {
		return map[string]string{canonPath(real): real} // journal says "real"
	}
	rr := postProjectJSON(t, s.handleProjectCreate, fmt.Sprintf(`{"name":"P","folders":[%q]}`, link))
	if rr.Code != 200 {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Created projectDef `json:"created"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Created.Folders[0] != real {
		t.Fatalf("folder stored as %q, want journal spelling %q", resp.Created.Folders[0], real)
	}
}

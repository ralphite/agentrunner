package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// projectDef is one entry in the explicit project registry (INC-104): a
// user-declared identity — a name plus one or more source folders — that the
// sidebar merges into a single group. Membership stays DERIVED: a session
// belongs to a project iff its journal workspace exactly matches one of the
// folders. The registry never rewrites the journal, never assigns a project id
// to a session, and deleting an entry only deletes the declaration — sessions,
// journals, and workspaces stay untouched and their per-workspace derived
// groups come back as-is (DESIGN §12, revised invariant).
type projectDef struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Folders   []string `json:"folders"`             // abs+Clean; [0] is the primary folder
	Order     int      `json:"order,omitempty"`     // manual sort rank; 0 = unranked
	CreatedAt int64    `json:"createdAt,omitempty"` // unix millis
	Missing   []string `json:"missing,omitempty"`   // computed at read time: folders gone from disk
}

// projectFile is the on-disk shape of <DataDir>/projects.json.
type projectFile struct {
	Version  int          `json:"version"`
	Projects []projectDef `json:"projects"`
}

// projectsPayload is what GET/POST /api/projects* return: the cosmetic
// workspace/project-keyed overlay plus the explicit registry, in one response
// so a single poll keeps both fresh.
type projectsPayload struct {
	Overlays map[string]projectMeta `json:"overlays"`
	Projects []projectDef           `json:"projects"`
}

// userError marks a validation failure whose message is safe and useful to
// show verbatim in the dialog (HTTP 400), as opposed to an I/O failure (500).
type userError string

func (e userError) Error() string { return string(e) }

const (
	maxProjects       = 200
	maxFoldersPerProj = 20
	maxProjectNameLen = 200
)

// projectStore owns <DataDir>/projects.json. The file is SHARED between every
// arwebui process on the machine (the user's daily port and any QA port), so
// the store holds no in-process cache: every read and every mutation re-reads
// the file under an exclusive flock, and mutations write back atomically
// before releasing it. The flock lives on a SEPARATE lock file — the data file
// is replaced by rename on every write, and a lock taken on a replaced inode
// excludes nobody (INC-104 锁纪律).
type projectStore struct {
	path     string // "" = in-memory only (tests)
	lockPath string
	mu       sync.Mutex   // serializes goroutines within this process
	mem      []projectDef // backing slice when path == ""
}

func newProjectStore(path string) *projectStore {
	s := &projectStore{path: path}
	if path != "" {
		s.lockPath = path + ".lock"
	}
	return s
}

// withLock is the single read-modify-write channel. fn receives the current
// entries and returns the replacement slice plus an explicit write intent —
// explicit because "nil means read-only" would silently skip persisting the
// deletion of the last entry (an empty result IS a nil slice in Go). The
// cross-process exclusion is the flock on lockPath; mu only keeps goroutines
// of this process from fighting over the lock file handle.
func (s *projectStore) withLock(fn func(cur []projectDef) (next []projectDef, write bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		next, write, err := fn(s.mem)
		if err != nil {
			return err
		}
		if write {
			s.mem = next
		}
		return nil
	}
	lf, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open project lock: %w", err)
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock projects: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) }()
	next, write, err := fn(s.readLocked())
	if err != nil {
		return err
	}
	if write {
		return s.writeLocked(next)
	}
	return nil
}

// readLocked parses the data file. A missing or corrupt file yields an empty
// registry — the store is user-declared data, but refusing to start over a
// parse error would brick every project feature at once; the atomic write
// path makes torn files effectively impossible.
func (s *projectStore) readLocked() []projectDef {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var pf projectFile
	if json.Unmarshal(b, &pf) != nil {
		return nil
	}
	return pf.Projects
}

func (s *projectStore) writeLocked(defs []projectDef) error {
	b, err := json.MarshalIndent(projectFile{Version: 1, Projects: defs}, "", " ")
	if err != nil {
		return fmt.Errorf("marshal projects: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write projects: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename projects: %w", err)
	}
	return nil
}

func (s *projectStore) list() []projectDef {
	var out []projectDef
	_ = s.withLock(func(cur []projectDef) ([]projectDef, bool, error) {
		out = append([]projectDef(nil), cur...)
		return nil, false, nil
	})
	return out
}

// folderSet returns every registered source folder, canonPath-normalized for
// launcher membership checks (INC-104: a registered folder is as "known" as a
// journal workspace — the user declared it in this UI).
func (s *projectStore) folderSet() map[string]bool {
	out := map[string]bool{}
	for _, def := range s.list() {
		for _, f := range def.Folders {
			out[canonPath(f)] = true
		}
	}
	return out
}

func newProjectID(now time.Time) string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("p-%d-%x", now.UnixMilli(), b)
}

// checkNewFolder validates a folder being ADDED to a project: it must exist as
// a directory and must not already belong to another project (compared
// canonically so /tmp and /private/tmp collide). Folders a project ALREADY
// holds are exempt from the exists check — otherwise a folder deleted on disk
// would lock the user out of ever editing (or fixing) the project again
// (INC-104 洞 2). Within-submit duplicates are the caller's check.
func checkNewFolder(f string, others []projectDef) error {
	c := canonPath(f)
	for _, o := range others {
		for _, of := range o.Folders {
			if canonPath(of) == c {
				return userError(fmt.Sprintf("That folder is already in %q: %s", o.Name, f))
			}
		}
	}
	if st, err := os.Stat(f); err != nil || !st.IsDir() {
		return userError("That folder isn't there — check the path: " + f)
	}
	return nil
}

func (s *projectStore) create(name string, folders []string) (projectDef, error) {
	var created projectDef
	err := s.withLock(func(cur []projectDef) ([]projectDef, bool, error) {
		if len(cur) >= maxProjects {
			return nil, false, userError("Too many projects — remove one first.")
		}
		seen := map[string]bool{}
		for _, f := range folders {
			c := canonPath(f)
			if seen[c] {
				return nil, false, userError("That folder is listed twice: " + f)
			}
			seen[c] = true
			if err := checkNewFolder(f, cur); err != nil {
				return nil, false, err
			}
		}
		created = projectDef{
			ID:        newProjectID(time.Now()),
			Name:      name,
			Folders:   folders,
			CreatedAt: time.Now().UnixMilli(),
		}
		return append(cur, created), true, nil
	})
	return created, err
}

// update applies a partial change: nil name / nil folders / nil order = leave
// unchanged. folders, when given, is the full replacement list from the dialog;
// only folders NOT already on the project get the exists check (洞 2).
func (s *projectStore) update(id string, name *string, folders []string, order *int) (projectDef, error) {
	var updated projectDef
	err := s.withLock(func(cur []projectDef) ([]projectDef, bool, error) {
		idx := -1
		for i := range cur {
			if cur[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, false, userError("That project doesn't exist anymore.")
		}
		next := append([]projectDef(nil), cur...)
		def := next[idx]
		if name != nil {
			def.Name = *name
		}
		if folders != nil {
			had := map[string]bool{}
			for _, f := range def.Folders {
				had[canonPath(f)] = true
			}
			others := append(append([]projectDef(nil), next[:idx]...), next[idx+1:]...)
			seen := map[string]bool{}
			for _, f := range folders {
				c := canonPath(f)
				if seen[c] {
					return nil, false, userError("That folder is listed twice: " + f)
				}
				seen[c] = true
				if had[c] {
					continue // already ours; stays editable even if gone from disk
				}
				if err := checkNewFolder(f, others); err != nil {
					return nil, false, err
				}
			}
			def.Folders = folders
		}
		if order != nil {
			def.Order = *order
		}
		next[idx] = def
		updated = def
		return next, true, nil
	})
	return updated, err
}

func (s *projectStore) remove(id string) error {
	return s.withLock(func(cur []projectDef) ([]projectDef, bool, error) {
		for i := range cur {
			if cur[i].ID == id {
				return append(append([]projectDef(nil), cur[:i]...), cur[i+1:]...), true, nil
			}
		}
		return nil, false, userError("That project doesn't exist anymore.")
	})
}

// ---- validation ----

func validateProjectName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", userError("Give the project a name.")
	}
	if len(n) > maxProjectNameLen {
		return "", userError("That name is too long — keep it under 200 characters.")
	}
	if strings.ContainsAny(n, "\n\r\x00") {
		return "", userError("A project name can't contain line breaks.")
	}
	return n, nil
}

func normalizeProjectFolder(p string) (string, error) {
	f := strings.TrimSpace(p)
	if f == "" {
		return "", userError("A source folder path can't be empty.")
	}
	if !filepath.IsAbs(f) {
		return "", userError("A source folder must be a full path (starting with /).")
	}
	return filepath.Clean(f), nil
}

func normalizeProjectFolders(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, userError("Add at least one source folder.")
	}
	if len(raw) > maxFoldersPerProj {
		return nil, userError(fmt.Sprintf("A project can hold at most %d folders.", maxFoldersPerProj))
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		f, err := normalizeProjectFolder(r)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// ---- journal-spelling normalization (洞 4) ----

// journalSpellings maps canonPath → the exact workspace spelling the journal
// uses, from the live session list. Best-effort: if `ar` is unreachable the map
// is empty and folders keep the user's spelling (fail-soft — grouping then only
// misses when the two spellings truly diverge, e.g. /tmp vs /private/tmp).
func (s *server) journalSpellings(ctx context.Context) map[string]string {
	if s.spellings != nil {
		return s.spellings(ctx)
	}
	out := map[string]string{}
	res := s.runAR(ctx, 15*time.Second, "sessions", "list", "--json")
	if res.Err != nil {
		return out
	}
	var rows []struct {
		Workspace string `json:"workspace"`
	}
	if json.Unmarshal([]byte(res.Stdout), &rows) != nil {
		return out
	}
	for _, row := range rows {
		if w := strings.TrimSpace(row.Workspace); w != "" {
			c := canonPath(w)
			if _, ok := out[c]; !ok {
				out[c] = w
			}
		}
	}
	return out
}

// toJournalSpelling rewrites each folder to the journal's spelling when both
// name the same directory, so the frontend's exact-match grouping claims the
// sessions the user expects. Idempotent: a folder already in journal form maps
// to itself.
func (s *server) toJournalSpelling(ctx context.Context, folders []string) []string {
	sp := s.journalSpellings(ctx)
	if len(sp) == 0 {
		return folders
	}
	out := make([]string, len(folders))
	for i, f := range folders {
		if j, ok := sp[canonPath(f)]; ok {
			out[i] = filepath.Clean(j)
		} else {
			out[i] = f
		}
	}
	return out
}

// ---- payload + handlers ----

// buildProjectsPayload assembles the combined overlay+registry response.
// Missing is computed here, at read time, so a folder deleted (or restored) on
// disk is reflected on the next poll without any write. A nil registry (bare
// test servers) is just an empty one.
func (s *server) buildProjectsPayload() projectsPayload {
	var defs []projectDef
	if s.projects != nil {
		defs = s.projects.list()
	}
	for i := range defs {
		var missing []string
		for _, f := range defs[i].Folders {
			if st, err := os.Stat(f); err != nil || !st.IsDir() {
				missing = append(missing, f)
			}
		}
		defs[i].Missing = missing
	}
	return projectsPayload{Overlays: s.meta.allProjects(), Projects: defs}
}

// projectFail maps a store/validation error onto the wire: userError → 400
// with the friendly sentence, anything else (I/O) → 500.
func projectFail(w http.ResponseWriter, err error) {
	var ue userError
	if errors.As(err, &ue) {
		badRequest(w, ue.Error())
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

// handleProjectCreate registers a new explicit project (INC-104). Creating one
// from an existing derived group ("upgrade") migrates that group's cosmetic
// pin/fold onto the new project key and clears the per-workspace overlay —
// otherwise a leftover removed:true would resurrect the derived group hidden
// if the project is later deleted (洞 5).
func (s *server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		Folders []string `json:"folders"`
	}
	if !readBody(w, r, &req) {
		return
	}
	name, err := validateProjectName(req.Name)
	if err != nil {
		projectFail(w, err)
		return
	}
	folders, err := normalizeProjectFolders(req.Folders)
	if err != nil {
		projectFail(w, err)
		return
	}
	folders = s.toJournalSpelling(r.Context(), folders)
	def, err := s.projects.create(name, folders)
	if err != nil {
		projectFail(w, err)
		return
	}
	s.absorbFolderOverlays(def, true)
	writeJSON(w, http.StatusOK, struct {
		projectsPayload
		Created projectDef `json:"created"`
	}{s.buildProjectsPayload(), def})
}

// absorbFolderOverlays clears the derived-group overlay of every folder now
// claimed by def (keeping lastOpened — setProject's all-default entries are
// dropped automatically). When migrate is set (create/upgrade), a pin or fold
// carried by any claimed folder moves onto the project's own overlay key.
func (s *server) absorbFolderOverlays(def projectDef, migrate bool) {
	overlays := s.meta.allProjects()
	pinned, folded := false, false
	cleared := ""
	off := false
	for _, f := range def.Folders {
		cur, ok := overlays[f]
		if !ok {
			continue
		}
		pinned = pinned || cur.Pinned
		folded = folded || cur.Folded
		s.meta.setProject(f, &cleared, &off, &off, &off)
	}
	if migrate && (pinned || folded) {
		s.meta.setProject(projectOverlayKey(def.ID), nil, &folded, &pinned, nil)
	}
}

// projectOverlayKey is the overlay map key for an explicit project's cosmetic
// state (pinned/folded). Workspace paths always start with "/", so the prefix
// can never collide with a derived group's key.
func projectOverlayKey(id string) string { return "project:" + id }

// handleProjectSave applies a partial update to one registry entry (INC-104).
func (s *server) handleProjectSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string   `json:"id"`
		Name    *string  `json:"name"`
		Folders []string `json:"folders"`
		Order   *int     `json:"order"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		badRequest(w, "id is required")
		return
	}
	if req.Name != nil {
		n, err := validateProjectName(*req.Name)
		if err != nil {
			projectFail(w, err)
			return
		}
		req.Name = &n
	}
	if req.Folders != nil {
		folders, err := normalizeProjectFolders(req.Folders)
		if err != nil {
			projectFail(w, err)
			return
		}
		req.Folders = s.toJournalSpelling(r.Context(), folders)
	}
	def, err := s.projects.update(req.ID, req.Name, req.Folders, req.Order)
	if err != nil {
		projectFail(w, err)
		return
	}
	if req.Folders != nil {
		s.absorbFolderOverlays(def, false)
	}
	writeJSON(w, http.StatusOK, s.buildProjectsPayload())
}

// handleProjectDelete deletes one registry entry. Sessions, journals, and
// workspaces are untouched; the folders' derived groups reappear as-is. The
// project's own overlay entry is cleared so a stale pin doesn't linger in the
// file forever.
func (s *server) handleProjectDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		badRequest(w, "id is required")
		return
	}
	if err := s.projects.remove(req.ID); err != nil {
		projectFail(w, err)
		return
	}
	cleared := ""
	off := false
	s.meta.setProject(projectOverlayKey(req.ID), &cleared, &off, &off, &off)
	writeJSON(w, http.StatusOK, s.buildProjectsPayload())
}

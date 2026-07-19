// arwebui is AgentRunner's self-contained local product surface. It talks to
// the system exclusively through the public `ar` CLI contract (os/exec) and
// serves a Vite-built React frontend embedded into the binary.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// version is the build stamp, set via -ldflags "-X main.version=<commit>" by
// scripts/deploy.sh. Left as "dev" for plain `go build`. It exists so a stale
// deploy is self-diagnosing: webui compares this against the `ar` binary it
// drives and shouts when they disagree (the stale-`ar` class — INC-43 `--steer`
// shipped in a new dist while the shared `ar` was pre-INC-43; see docs/LOG.md
// 2026-07-10 复盘).
var version = "dev"

type server struct {
	arPath      string
	runtimeDir  string
	worktreeDir string // stable shared root for `New worktree` checkouts (DataDir()/worktrees)
	version     string // this webui binary's build stamp (main.version)

	mu             sync.Mutex
	daemonCmd      *exec.Cmd // the daemon we spawned; nil when unmanaged
	daemonAlive    bool      // our child is still running
	daemonManage   bool      // we are supposed to manage one
	daemonExternal bool      // our spawn exited fast: an external daemon owns the socket
	stopping       bool      // shutdown in progress: do not respawn
	respawns       []time.Time

	runs *runRegistry // background submit/drive runs
	meta *metaStore   // sid → workspace/title we know from creating it

	// launch runs the OS "open" argv for the INC-53 launcher; nil = real exec
	// (runLaunch). workspaces resolves the launcher's allowed-workspace set;
	// nil = derive from the live `ar sessions list`. Both are fields so tests
	// can capture the argv and inject a known set without launching real apps.
	launch     func(ctx context.Context, argv []string) error
	workspaces func(ctx context.Context) map[string]bool
}

// resolveARPath picks the agentrunner binary. Unless the user passed -ar
// explicitly, prefer an `ar` sitting next to this arwebui executable: the
// one-line installer (INC-63) drops both binaries side by side, and on Linux a
// bare `ar` on PATH usually resolves to GNU binutils' archiver of the same
// name when ~/.local/bin is not ahead of /usr/bin. os.Executable resolves
// /proc/self/exe past the ~/.local/bin symlink to releases/<ver>/arwebui, whose
// sibling is the real ar.
func resolveARPath(flagVal string, explicit bool) string {
	if explicit {
		return flagVal
	}
	exe, err := os.Executable()
	if err != nil {
		return flagVal
	}
	return arSiblingOr(exe, flagVal)
}

// arSiblingOr returns the `ar` next to exe if it exists and is executable,
// else fallback. Split out from resolveARPath so the sibling-preference logic
// is testable without touching os.Executable().
func arSiblingOr(exe, fallback string) string {
	sibling := filepath.Join(filepath.Dir(exe), "ar")
	if fi, err := os.Stat(sibling); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
		return sibling
	}
	return fallback
}

func main() {
	arPath := flag.String("ar", "ar", "path to the agentrunner binary (default: the `ar` next to arwebui, else PATH)")
	addr := flag.String("addr", "127.0.0.1:8788", "listen address (keep it loopback)")
	envFile := flag.String("env-file", "", "KEY=VALUE file loaded into the environment before spawning anything")
	noDaemon := flag.Bool("no-daemon", false, "do not spawn `ar daemon`; use an external one")
	runtimeDir := flag.String("runtime", "runtime", "scratch dir for specs/uploads/workspaces/daemon.log")
	showVersion := flag.Bool("version", false, "print the build stamp and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("arwebui %s\n", version)
		return
	}

	arExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "ar" {
			arExplicit = true
		}
	})
	*arPath = resolveARPath(*arPath, arExplicit)

	if *envFile != "" {
		if err := loadEnvFile(*envFile); err != nil {
			log.Fatalf("arwebui: --env-file: %v", err)
		}
	}
	rt, err := filepath.Abs(*runtimeDir)
	if err != nil {
		log.Fatalf("arwebui: %v", err)
	}
	for _, d := range []string{"specs", "uploads", "ws", "runs"} {
		if err := os.MkdirAll(filepath.Join(rt, d), 0o755); err != nil {
			log.Fatalf("arwebui: %v", err)
		}
	}

	// `New worktree` checkouts live under the shared data root (same root as the
	// daemon store: $XDG_DATA_HOME/agentrunner or ~/.local/share/agentrunner),
	// NOT under the webui cwd's runtime/ — so worktrees of any repo land in a
	// stable, discoverable place named after the repo, not a bare timestamp.
	wtDir := filepath.Join(dataDir(), "worktrees")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		log.Fatalf("arwebui: worktree dir: %v", err)
	}

	s := &server{arPath: *arPath, version: version, runtimeDir: rt, worktreeDir: wtDir, daemonManage: !*noDaemon, runs: newRunRegistry(), meta: newMetaStore(filepath.Join(rt, "webui-meta.json"))}
	s.warnOnARVersionSkew()
	if s.daemonManage {
		if err := s.spawnDaemon(); err != nil {
			log.Printf("arwebui: daemon spawn: %v (continuing; maybe an external daemon is up)", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: *addr, Handler: s.routes()}
	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shctx)
	}()

	log.Printf("arwebui listening on http://%s (ar=%s, runtime=%s)", *addr, *arPath, rt)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("arwebui: %v", err)
	}
	s.runs.stopAll()
	s.stopDaemon()
}

// spawnDaemon starts `ar daemon` with our environment. If it exits within
// 700ms we assume an external daemon already owns the socket and step back.
func (s *server) spawnDaemon() error {
	logf, err := os.OpenFile(filepath.Join(s.runtimeDir, "daemon.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	fmt.Fprintf(logf, "--- arwebui spawn %s ---\n", time.Now().Format(time.RFC3339))
	cmd := exec.Command(s.arPath, "daemon")
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		return err
	}
	startedAt := time.Now()
	s.mu.Lock()
	s.daemonCmd, s.daemonAlive = cmd, true
	s.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		_ = logf.Close()
		ranFor := time.Since(startedAt)
		s.mu.Lock()
		s.daemonAlive = false
		stopping := s.stopping
		// A near-instant exit means an external daemon already owns the
		// socket (bind failed). That is not a crash to recover from —
		// respawning just churns and spams the log (UX-04). Step back:
		// mark external, stop managing, announce it once.
		external := ranFor < 3*time.Second
		firstExternal := external && !s.daemonExternal
		if external {
			s.daemonExternal = true
		}
		now := time.Now()
		var recent []time.Time
		for _, t := range s.respawns {
			if now.Sub(t) < time.Minute {
				recent = append(recent, t)
			}
		}
		allow := !stopping && !external && s.daemonManage && len(recent) < 3
		if allow {
			recent = append(recent, now)
		}
		s.respawns = recent
		s.mu.Unlock()
		if firstExternal {
			log.Printf("arwebui: an external daemon already owns the socket — using it, not managing our own")
		}
		if allow {
			time.Sleep(time.Second)
			if err := s.spawnDaemon(); err != nil {
				log.Printf("arwebui: daemon auto-respawn failed: %v", err)
			} else {
				log.Printf("arwebui: managed daemon died; auto-respawned")
			}
		}
	}()
	time.Sleep(700 * time.Millisecond)
	s.mu.Lock()
	alive := s.daemonAlive
	s.mu.Unlock()
	if !alive {
		return fmt.Errorf("`ar daemon` exited immediately (external daemon already running? see runtime/daemon.log)")
	}
	return nil
}

func (s *server) stopDaemon() {
	s.mu.Lock()
	s.stopping = true
	cmd, alive := s.daemonCmd, s.daemonAlive
	s.mu.Unlock()
	if cmd != nil && alive && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
}

// daemonStatus reports (managedAlive, reachable). Reachability is probed
// through the CLI: `ar interrupt <bogus>` fails with "is the daemon
// running?" iff the socket dial failed (zero side effects).
func (s *server) daemonStatus(ctx context.Context) (alive, reachable, external bool) {
	s.mu.Lock()
	alive = s.daemonAlive
	external = s.daemonExternal
	s.mu.Unlock()
	res := s.runAR(ctx, 5*time.Second, "interrupt", "__arwebui_probe__")
	reachable = !daemonUnreachable(res.Stderr)
	return alive, reachable, external
}

// arVersion returns the trimmed `ar --version` output ("agentrunner <stamp>
// (go...)"), or "" if the binary could not be run.
func (s *server) arVersion(ctx context.Context) string {
	res := s.runAR(ctx, 5*time.Second, "--version")
	if res.Err != nil {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

// versionMatch reports whether the `ar` binary we drive was built from the same
// stamp as this webui. Both are stamped with the same commit by
// scripts/deploy.sh, so a match means `ar --version` contains our stamp. Plain
// "dev" builds always match (contains "dev") — no false alarm for local `go
// build`. An empty arVer (ar unrunnable) is reported as a mismatch.
func versionMatch(webuiVer, arVer string) bool {
	if arVer == "" {
		return false
	}
	return strings.Contains(arVer, webuiVer)
}

// warnOnARVersionSkew logs a loud line at startup when the `ar` binary was built
// from a different commit than this webui. This is the cheap mechanical guard
// for the stale-`ar` class: a new dist (new webui flags/features) driving an old
// `ar` produces cryptic "flag provided but not defined" send failures. The
// warning names both stamps so the fix (redeploy via scripts/deploy.sh) is
// obvious. "dev" builds are skipped — they are indistinguishable by design.
func (s *server) warnOnARVersionSkew() {
	if s.version == "dev" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	arVer := s.arVersion(ctx)
	if arVer == "" {
		log.Printf("arwebui: WARNING could not read `ar --version` (%s) — cannot verify binary parity", s.arPath)
		return
	}
	if !versionMatch(s.version, arVer) {
		log.Printf("arwebui: WARNING version skew — webui %q but ar=%q reports %q; "+
			"the ar binary is out of date. Redeploy both from the same commit (scripts/deploy.sh) "+
			"or new webui features (e.g. --steer) will fail with cryptic 'flag not defined' errors.",
			s.version, s.arPath, arVer)
	}
}

// dataDir is the harness state root, mirroring internal/runtime.DataDir:
// $XDG_DATA_HOME/agentrunner, else ~/.local/share/agentrunner (same rule on
// macOS — not ~/Library). arwebui is its own Go module and can't import the
// internal package, so the rule is duplicated here; keep the two in sync.
func dataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "agentrunner")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// No home: fall back to a cwd-relative dir so we never panic. This only
		// happens in exotic environments; normal desktop/CLI use always has one.
		return filepath.Join("runtime", "data")
	}
	return filepath.Join(home, ".local", "share", "agentrunner")
}

// loadEnvFile applies KEY=VALUE lines (comments/blank lines skipped).
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if err := os.Setenv(strings.TrimSpace(k), v); err != nil {
			return err
		}
	}
	return sc.Err()
}

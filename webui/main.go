// arwebui is a self-contained local web cockpit for AgentRunner with a
// Codex-cloud-style UI. It talks to the system exclusively through the public
// `ar` CLI contract (os/exec) and serves a Vite-built React frontend embedded
// into the binary. Like web/, it is a test/QA cockpit, not a product surface.
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

type server struct {
	arPath     string
	runtimeDir string

	mu           sync.Mutex
	daemonCmd    *exec.Cmd // the daemon we spawned; nil when unmanaged
	daemonAlive  bool      // our child is still running
	daemonManage bool      // we are supposed to manage one
	stopping     bool      // shutdown in progress: do not respawn
	respawns     []time.Time

	runs *runRegistry // background submit/drive runs
	meta *metaStore   // sid → workspace/title we know from creating it
}

func main() {
	arPath := flag.String("ar", "ar", "path to the agentrunner binary")
	addr := flag.String("addr", "127.0.0.1:8788", "listen address (keep it loopback)")
	envFile := flag.String("env-file", "", "KEY=VALUE file loaded into the environment before spawning anything")
	noDaemon := flag.Bool("no-daemon", false, "do not spawn `ar daemon`; use an external one")
	runtimeDir := flag.String("runtime", "runtime", "scratch dir for specs/uploads/workspaces/daemon.log")
	flag.Parse()

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

	s := &server{arPath: *arPath, runtimeDir: rt, daemonManage: !*noDaemon, runs: newRunRegistry(), meta: newMetaStore()}
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
	s.mu.Lock()
	s.daemonCmd, s.daemonAlive = cmd, true
	s.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		_ = logf.Close()
		s.mu.Lock()
		s.daemonAlive = false
		stopping := s.stopping
		now := time.Now()
		var recent []time.Time
		for _, t := range s.respawns {
			if now.Sub(t) < time.Minute {
				recent = append(recent, t)
			}
		}
		allow := !stopping && s.daemonManage && len(recent) < 3
		if allow {
			recent = append(recent, now)
		}
		s.respawns = recent
		s.mu.Unlock()
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
func (s *server) daemonStatus(ctx context.Context) (bool, bool) {
	s.mu.Lock()
	alive := s.daemonAlive
	s.mu.Unlock()
	res := s.runAR(ctx, 5*time.Second, "interrupt", "__arwebui_probe__")
	reachable := !daemonUnreachable(res.Stderr)
	return alive, reachable
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

// INC-50 (G14/UJ-12): `ar hook` manages the webhook ingress capabilities.
// A hook binds a session to an unguessable id + bearer token; POSTing to the
// daemon's /hooks/<id> endpoint delivers an external event into that
// session's durable inbox as source:"machine" / trust:"untrusted". The
// registry is a plain data-dir file — the daemon re-reads it per request, so
// create/revoke need no daemon restart. The token prints exactly once here;
// only its hash is stored, and it never enters a journal.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/runtime"
)

func hooksPath() string {
	data, err := runtime.DataDir()
	if err != nil {
		return "hooks.json" // DataDir failure surfaces on the actual write
	}
	return filepath.Join(data, "hooks.json")
}

func hookAddrPath() string {
	data, err := runtime.DataDir()
	if err != nil {
		return "daemon.http"
	}
	return filepath.Join(data, "daemon.http")
}

func hookCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, `usage: agentrunner hook <create|list|revoke> ...`)
		return ExitUsage
	}
	switch args[0] {
	case "create":
		return hookCreateCmd(args[1:], stdout, stderr)
	case "list":
		return hookListCmd(args[1:], stdout, stderr)
	case "revoke":
		return hookRevokeCmd(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "hook: unknown subcommand %q (known: create, list, revoke)\n", args[0])
		return ExitUsage
	}
}

func hookCreateCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hook create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "a short sender label; deliveries journal as principal \"hook:<name>\"")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, `usage: agentrunner hook create <session-id-or-prefix> [--name ci]`)
		return ExitUsage
	}
	// Validate the session exists BEFORE minting a hook: a lenient resolve
	// would happily create a hook bound to a nonexistent session, which then
	// fails delivery with a misleading 502 "could not be resumed" (QA Wave3
	// judy-02). Refuse up front with the canonical not-found.
	session, aerr := resolveAddress(rest[0])
	if aerr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", aerr)
		return ExitUsage
	}
	hk, token, err := daemon.CreateHook(hooksPath(), session, *name)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fmt.Fprintf(stdout, "hook %s → session %s\n", hk.ID, session)
	fmt.Fprintf(stdout, "token (shown ONCE, store it now): %s\n", token)
	if addr, err := os.ReadFile(hookAddrPath()); err == nil {
		fmt.Fprintf(stdout, "deliver: curl -X POST http://%s/hooks/%s -H 'Authorization: Bearer <token>' -d '<event text>'\n",
			strings.TrimSpace(string(addr)), hk.ID)
	} else {
		// The ingress is a boot-time choice: a second `daemon --http` can't add
		// it to an already-running daemon (it refuses on the store lock), so the
		// old "start the ingress with: daemon --http" hint dead-ended (QA Wave3
		// judy-01). Tell the user the honest path: the running daemon must be
		// restarted WITH --http for this hook to be deliverable.
		fmt.Fprintf(stdout, "deliver: POST /hooks/%s with 'Authorization: Bearer <token>'\n", hk.ID)
		fmt.Fprintln(stdout, "  note: the daemon is running WITHOUT the ingress. Enabling it is a boot-time choice —")
		fmt.Fprintln(stdout, "  restart the daemon with --http (e.g. stop it, then `agentrunner daemon --detach --http 127.0.0.1:4177`).")
	}
	return ExitOK
}

func hookListCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hook list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) > 1 {
		fmt.Fprintln(stderr, `usage: agentrunner hook list [<session-id-or-prefix>]`)
		return ExitUsage
	}
	hooks, err := daemon.LoadHooks(hooksPath())
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	filter := ""
	if len(rest) == 1 {
		filter = resolvePrefixLenient(rest[0])
	}
	shown := 0
	for _, h := range hooks {
		if filter != "" && h.Session != filter {
			continue
		}
		if shown == 0 {
			fmt.Fprintf(stdout, "%-18s %-14s %-22s %s\n", "HOOK-ID", "NAME", "CREATED", "SESSION")
		}
		fmt.Fprintf(stdout, "%-18s %-14s %-22s %s\n", h.ID, h.Name, h.CreatedAt, h.Session)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(stdout, "no hooks")
	}
	return ExitOK
}

func hookRevokeCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, `usage: agentrunner hook revoke <hook-id>`)
		return ExitUsage
	}
	found, err := daemon.RevokeHook(hooksPath(), args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if !found {
		fmt.Fprintf(stderr, "agentrunner: no hook %s\n", args[0])
		return ExitUsage
	}
	fmt.Fprintf(stdout, "hook %s revoked\n", args[0])
	return ExitOK
}

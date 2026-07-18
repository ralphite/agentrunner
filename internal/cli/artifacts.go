package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// artifactsCmd is the human/pipeline consumption face for published
// artifacts (INC-40): `ar artifacts <session> [list]` tabulates the fold's
// Published truth; `ar artifacts <session> read <stream>[@vN]` writes one
// version's raw content to stdout (full content — paging is the model
// face's concern, a pipe can `head` its own).
func artifactsCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("artifacts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "list as JSON")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(stderr, `usage: agentrunner artifacts <session> [list|read <stream>[@vN]]`)
		return ExitUsage
	}
	dir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	s, err := state.Fold(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: fold: %v\n", err)
		return ExitRun
	}
	as, err := store.OpenArtifactStore(filepath.Join(dir, "artifacts"))
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	verb := "list"
	if len(rest) > 1 {
		verb = rest[1]
	}
	switch verb {
	case "list":
		return artifactsList(s, as, *jsonOut, stdout)
	case "read":
		if len(rest) != 3 {
			fmt.Fprintln(stderr, `usage: agentrunner artifacts <session> read <stream>[@vN]`)
			return ExitUsage
		}
		return artifactsRead(s, as, rest[2], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "agentrunner: unknown artifacts verb %q (list|read)\n", verb)
		return ExitUsage
	}
}

func artifactsList(s state.State, as *store.ArtifactStore, jsonOut bool, stdout io.Writer) int {
	streams, _ := as.Streams()
	type row struct {
		Stream  string `json:"stream"`
		Version int    `json:"version"`
		Bytes   int    `json:"bytes"`
	}
	rows := make([]row, 0, len(s.Session.Published))
	for stream, version := range s.Session.Published {
		r := row{Stream: stream, Version: version}
		for _, v := range streams[stream] {
			if v.Version == version {
				r.Bytes = v.Bytes
			}
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Stream < rows[j].Stream })
	if jsonOut {
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return ExitOK
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no published artifacts")
		return ExitOK
	}
	fmt.Fprintf(stdout, "%-30s %8s %10s\n", "STREAM", "LATEST", "BYTES")
	for _, r := range rows {
		fmt.Fprintf(stdout, "%-30s %8s %10d\n", r.Stream, "v"+strconv.Itoa(r.Version), r.Bytes)
	}
	return ExitOK
}

func artifactsRead(s state.State, as *store.ArtifactStore, spec string, stdout, stderr io.Writer) int {
	stream, version := spec, 0
	if at := strings.LastIndex(spec, "@v"); at > 0 {
		// Parse the version strictly and consistently: a non-numeric (@vabc),
		// zero (@v0), or negative (@v-1) suffix used to each behave differently
		// — literal stream name, silent latest, and range error respectively
		// (QA Wave3 ivan-03). All are now one clear error; omit @vN for latest.
		n, err := strconv.Atoi(spec[at+2:])
		if err != nil || n < 1 {
			fmt.Fprintf(stderr, "agentrunner: bad version in %q — use <stream>@vN with N ≥ 1, or omit @vN for the latest\n", spec)
			return ExitUsage
		}
		stream, version = spec[:at], n
	}
	latest, ok := s.Session.Published[stream]
	if !ok {
		fmt.Fprintf(stderr, "agentrunner: no published artifact stream %q (try: artifacts list)\n", stream)
		return ExitUsage
	}
	if version == 0 {
		version = latest
	}
	if version < 1 || version > latest {
		fmt.Fprintf(stderr, "agentrunner: stream %q has versions 1..%d\n", stream, latest)
		return ExitUsage
	}
	streams, err := as.Streams()
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	for _, v := range streams[stream] {
		if v.Version == version {
			content, gerr := as.Get(v.Ref)
			if gerr != nil {
				fmt.Fprintf(stderr, "agentrunner: %v\n", gerr)
				return ExitRun
			}
			_, _ = stdout.Write(content)
			return ExitOK
		}
	}
	fmt.Fprintf(stderr, "agentrunner: version %d of %q is not in the store\n", version, stream)
	return ExitRun
}

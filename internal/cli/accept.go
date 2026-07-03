package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/ralphite/agentrunner/internal/accept"
)

// acceptCmd runs the stage acceptance scenarios (PLAN 0.6): TUI on a TTY,
// plain text otherwise, acceptance-report.json always.
func acceptCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("accept", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stage := fs.Int("stage", 0, "stage number to accept")
	plain := fs.Bool("plain", false, "force plain output (no TUI)")
	report := fs.String("report", "acceptance-report.json", "JSON report path")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *stage <= 0 {
		fmt.Fprintln(stderr, "usage: agentrunner accept --stage <n> [--plain]")
		return ExitUsage
	}

	scenarios, err := accept.LoadStage(*stage)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}

	bin, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	runner := &accept.Runner{Bin: bin}

	var results []accept.Result
	if !*plain && term.IsTerminal(int(os.Stdout.Fd())) {
		results, err = accept.RunTUI(runner, *stage, scenarios)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return ExitRun
		}
	} else {
		for _, s := range scenarios {
			results = append(results, runner.Run(s))
		}
	}

	rep := accept.BuildReport(*stage, results)
	accept.RenderPlain(stdout, rep)
	if err := rep.WriteJSON(*report); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	fmt.Fprintf(stderr, "report written to %s\n", *report)

	if rep.Fail > 0 {
		return ExitRun
	}
	return ExitOK
}

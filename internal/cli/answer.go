package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// answerCmd answers a structured ask_user park (INC-47):
//
//	agentrunner answer <session> 1:2 2:1,3 3:text=custom words
//	agentrunner answer <session> --skip
//
// Each positional argument answers one question: <q>:<choices> with
// 1-based question and option numbers, or <q>:text=<free text>. The local
// precheck reads the park's questions from the journal so a typo fails
// here, not silently in the loop.
func answerCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("answer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	skip := fs.Bool("skip", false, "skip the question(s): the model is told the user declined to answer")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) < 1 || (!*skip && len(rest) < 2) {
		fmt.Fprintln(stderr, `usage: agentrunner answer <session> <q>:<choices>... | agentrunner answer <session> --skip
  e.g. answer <sid> 1:2         pick option 2 of question 1
       answer <sid> 1:1,3       multi-select options 1 and 3
       answer <sid> 2:text=...  free-text answer to question 2`)
		return ExitUsage
	}
	dir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	questions, perr := parkedQuestions(dir)
	if perr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", perr)
		return ExitUsage
	}
	cmd := daemon.Command{Cmd: "answer", Session: resolvePrefixLenient(rest[0]),
		CommandID: event.NewCommandID(), Principal: "local-user", Source: "cli", Trust: "local"}
	if *skip {
		cmd.Cancelled = true
		return oneShot(stderr, cmd, stdout)
	}
	answers, aerr := parseAnswerSpecs(rest[1:], questions)
	if aerr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", aerr)
		return ExitUsage
	}
	cmd.Answers = answers
	return oneShot(stderr, cmd, stdout)
}

// parkedQuestions folds the journal and returns the current ask park's
// structured questions.
func parkedQuestions(dir string) ([]event.AskQuestion, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	s, err := state.Fold(events)
	if err != nil {
		return nil, fmt.Errorf("fold: %w", err)
	}
	if s.Waiting == nil || s.Waiting.Kind != event.WaitInput || len(s.Waiting.Detail) == 0 {
		return nil, fmt.Errorf("the session is not waiting on a question (inspect shows its state)")
	}
	var d struct {
		Questions []event.AskQuestion `json:"questions"`
		Question  string              `json:"question"`
	}
	if err := json.Unmarshal(s.Waiting.Detail, &d); err != nil || (len(d.Questions) == 0 && d.Question == "") {
		return nil, fmt.Errorf("the session's wait is not an ask_user park")
	}
	if len(d.Questions) == 0 {
		return nil, fmt.Errorf("this ask is a plain question — answer it with: agentrunner send <session> \"...\"")
	}
	return d.Questions, nil
}

// parseAnswerSpecs turns "1:2,3" / "2:text=..." specs into validated
// AskAnswers against the park's questions (1-based on the wire, 0-based in
// the event).
func parseAnswerSpecs(specs []string, questions []event.AskQuestion) ([]event.AskAnswer, error) {
	var out []event.AskAnswer
	for _, spec := range specs {
		qs, rest, ok := strings.Cut(spec, ":")
		if !ok {
			return nil, fmt.Errorf("bad answer %q: want <question>:<choices>", spec)
		}
		qn, err := strconv.Atoi(qs)
		if err != nil || qn < 1 || qn > len(questions) {
			return nil, fmt.Errorf("bad question number %q (this ask has %d)", qs, len(questions))
		}
		q := questions[qn-1]
		a := event.AskAnswer{Question: qn - 1}
		if text, isText := strings.CutPrefix(rest, "text="); isText {
			if len(q.Options) > 0 && !q.AllowFreeText {
				return nil, fmt.Errorf("question %d does not accept free text", qn)
			}
			a.Text = text
		} else {
			for _, c := range strings.Split(rest, ",") {
				on, cerr := strconv.Atoi(strings.TrimSpace(c))
				if cerr != nil || on < 1 || on > len(q.Options) {
					return nil, fmt.Errorf("question %d has options 1..%d (got %q)", qn, len(q.Options), c)
				}
				a.Selected = append(a.Selected, q.Options[on-1].Label)
			}
			if len(a.Selected) > 1 && !q.MultiSelect {
				return nil, fmt.Errorf("question %d is single-select", qn)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

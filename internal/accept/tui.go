package accept

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// RunTUI executes scenarios with a live checklist (TTY path). Returns the
// results in scenario order.
func RunTUI(runner *Runner, stage int, scenarios []Scenario) ([]Result, error) {
	m := &model{stage: stage, scenarios: scenarios, runner: runner,
		results: make([]Result, len(scenarios))}
	prog := tea.NewProgram(m)
	if _, err := prog.Run(); err != nil {
		return nil, err
	}
	return m.results, nil
}

type doneMsg struct {
	index  int
	result Result
}

type model struct {
	stage     int
	scenarios []Scenario
	runner    *Runner
	results   []Result
	current   int
	finished  bool
}

func (m *model) Init() tea.Cmd {
	return m.runNext()
}

func (m *model) runNext() tea.Cmd {
	idx := m.current
	if idx >= len(m.scenarios) {
		return tea.Quit
	}
	return func() tea.Msg {
		return doneMsg{index: idx, result: m.runner.Run(m.scenarios[idx])}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case doneMsg:
		m.results[msg.index] = msg.result
		m.current++
		if m.current >= len(m.scenarios) {
			m.finished = true
			return m, tea.Quit
		}
		return m, m.runNext()
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) View() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "acceptance — stage %d\n\n", m.stage)
	for i, s := range m.scenarios {
		switch {
		case i < m.current:
			r := m.results[i]
			mark := map[Status]string{StatusPass: "✓", StatusFail: "✗", StatusSkipped: "–"}[r.Status]
			fmt.Fprintf(&sb, " %s %-28s %s (%.1fs)\n", mark, s.ID, s.Title, r.Duration.Seconds())
			if r.Status == StatusFail {
				fmt.Fprintf(&sb, "   %s\n", firstLine(r.Detail))
			}
		case i == m.current && !m.finished:
			fmt.Fprintf(&sb, " ⠿ %-28s %s …\n", s.ID, s.Title)
		default:
			fmt.Fprintf(&sb, " · %-28s %s\n", s.ID, s.Title)
		}
	}
	sb.WriteString("\n(q to abort)\n")
	return sb.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

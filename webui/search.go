package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// handleSearch is the thin shell over `ar search --json` (G44): the command
// palette can finally answer "which conversation mentioned this?" instead of
// substring-filtering the titles of whatever happened to be loaded.
//
// Deliberately NOT exposing --include-tools. Tool arguments and results carry
// file contents and command output; reaching them takes a considered decision
// and belongs to a surface that can explain what it is about to disclose, not
// to a query string anyone can put in a URL.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		badRequest(w, "q is required")
		return
	}
	args := []string{"search", "--json", q}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 200 {
			badRequest(w, "limit must be an integer in [1, 200]")
			return
		}
		args = append(args, "--limit", strconv.Itoa(n))
	}
	// The scan is bounded and the bounds ride the payload, so a slow store
	// degrades into a declared-partial answer rather than a hung palette.
	res := s.runAR(r.Context(), 20*time.Second, args...)
	if res.Err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ar search: " + res.Err.Error()})
		return
	}
	var out struct {
		Query           string `json:"query"`
		Matches         []any  `json:"matches"`
		SessionsScanned int    `json:"sessions_scanned"`
		SessionsSkipped int    `json:"sessions_skipped"`
		Truncated       bool   `json:"truncated"`
		Limit           int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "decode ar search --json: " + err.Error()})
		return
	}
	if out.Matches == nil {
		out.Matches = []any{}
	}
	// Keep the CLI's snake_case bounds AND add the frontend's camelCase, the
	// same dual-key contract handleSessions uses. The bounds are part of the
	// answer: a palette that hides them turns "we stopped early" into "there
	// is nothing there".
	writeJSON(w, http.StatusOK, map[string]any{
		"query":            out.Query,
		"matches":          out.Matches,
		"sessions_scanned": out.SessionsScanned,
		"sessionsScanned":  out.SessionsScanned,
		"sessions_skipped": out.SessionsSkipped,
		"sessionsSkipped":  out.SessionsSkipped,
		"truncated":        out.Truncated,
		"limit":            out.Limit,
	})
}

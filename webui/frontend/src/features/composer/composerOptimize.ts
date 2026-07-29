// Pure controller for the composer's prompt-optimization + single-step undo
// (INC-56, HANDA-PARITY #19). Framework-free so it can be unit-tested with
// mocked ar calls; the React state setters are injected.

export interface OptimizeIO {
  // optimize is AR.optimize(draft, context) → the rewritten prompt.
  optimize: (draft: string, context: string) => Promise<{ text: string }>;
  setText: (t: string) => void;
  // setUndo stores the draft to restore (string) or clears the affordance (null).
  setUndo: (original: string | null) => void;
  toast: (msg: string, kind?: "info" | "error") => void;
  onError: (msg: string) => void;
}

// runOptimize rewrites `draft` and, on success, swaps the result into the
// composer while stashing `restoreTo` for a single-step undo. `restoreTo` is
// what the user gets back on undo — the whole textarea for the Sparkles button,
// or the text typed after "/optimize" for the slash. The original is never
// mutated en route, so undo is always exact.
export async function runOptimize(
  io: OptimizeIO,
  draft: string,
  restoreTo: string,
  context: string,
): Promise<void> {
  const d = draft.trim();
  if (!d) return;
  try {
    const { text } = await io.optimize(d, context);
    const result = (text || "").trim();
    if (!result) {
      io.toast("Optimizer returned nothing — draft unchanged", "info");
      return;
    }
    io.setText(result);
    io.setUndo(restoreTo);
    io.toast("Prompt optimized — Undo restores your draft", "info");
  } catch (e: any) {
    io.onError(e?.message || String(e));
  }
}

// undoOptimize restores the stashed draft and clears the affordance.
export function undoOptimize(
  io: Pick<OptimizeIO, "setText" | "setUndo">,
  original: string,
): void {
  io.setText(original);
  io.setUndo(null);
}

// One turn of the session's conversation, as the context sections see it.
export interface HelperMessage {
  role: "user" | "assistant";
  text: string;
}

export interface HelperContextInput {
  // Where the session runs — the workspace path.
  workspace?: string | null;
  // The session's recent turns, oldest first. Absent on Home (no session yet).
  recent?: HelperMessage[] | null;
  // What's already typed in the composer.
  draft?: string | null;
}

// Per-section budgets. The assembled context rides in an argv `--context` and
// then inside a system prompt, so it has to stay small on both counts: an agent
// reply can run to thousands of characters, and a few of those would both crowd
// the exec arg limit and bury the handful of proper nouns that actually help
// under paragraphs of prose. Roughly 3KB assembled, worst case.
const MAX_WORKSPACE = 200;
const MAX_DRAFT = 800;
const MAX_RECENT_TOTAL = 1500;
const MAX_USER_MSG = 300;
const MAX_ASSISTANT_MSG = 200;
const MAX_RECENT_COUNT = 6;

// clipInline collapses whitespace and caps the length — for values that must
// occupy exactly one line (a workspace path, one conversation turn).
function clipInline(s: string | null | undefined, max: number): string {
  const t = (s || "").replace(/\s+/g, " ").trim();
  return t.length <= max ? t : t.slice(0, max).trimEnd() + "…";
}

// clipBlock caps the length but keeps the line structure — for the draft, which
// is the user's own in-progress text and reads better unmangled.
function clipBlock(s: string | null | undefined, max: number): string {
  const t = (s || "").trim();
  return t.length <= max ? t : t.slice(0, max).trimEnd() + "…";
}

// recentSection renders the conversation turns, newest-first while spending the
// budget so a long tail of old chatter can never crowd out what was just said,
// then flipped back into reading order.
function recentSection(recent: HelperMessage[]): string {
  const lines: string[] = [];
  let used = 0;
  for (const m of recent.slice(-MAX_RECENT_COUNT).reverse()) {
    const body = clipInline(m.text, m.role === "user" ? MAX_USER_MSG : MAX_ASSISTANT_MSG);
    if (!body) continue;
    const line = `${m.role}: ${body}`;
    if (used + line.length > MAX_RECENT_TOTAL) break;
    used += line.length;
    lines.push(line);
  }
  return lines.reverse().join("\n");
}

// helperContext assembles the labelled context the dictate/optimize helpers use
// to spell proper nouns right and resolve vague references. The sections are
// labelled because the model otherwise can't tell a workspace path from a half-
// typed draft — they used to arrive as bare newline-joined lines. `ar dictate`
// appends its own "# Terms" section from the workspace's terms file, so this
// side deliberately owns only what the browser knows. Empty in → empty out (the
// command treats an empty context as "no hint").
export function helperContext(input: HelperContextInput): string {
  const sections: string[] = [];
  const ws = clipInline(input.workspace, MAX_WORKSPACE);
  if (ws) sections.push("# Project\n" + ws);
  const recent = recentSection(input.recent || []);
  if (recent) sections.push("# Recent conversation\n" + recent);
  const draft = clipBlock(input.draft, MAX_DRAFT);
  if (draft) sections.push("# Draft so far\n" + draft);
  return sections.join("\n\n");
}

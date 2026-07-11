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

// helperContext joins the non-empty context fragments (session label, workspace,
// current draft…) the dictate/optimize helpers use to disambiguate proper nouns
// and resolve vague references. Empty in → empty out (the command treats an
// empty context as "no hint").
export function helperContext(parts: (string | undefined | null)[]): string {
  return parts
    .map((p) => (p || "").trim())
    .filter(Boolean)
    .join("\n");
}

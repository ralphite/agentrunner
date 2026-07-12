// SC-13 — a scheduled row's title has to be a NAME, not the prompt.
//
// Every row on the Scheduled hub is titled with the text that created it, which
// is a paragraph of instructions ("Append one line with the current timestamp to
// notes.md, then commit it (use write_file or bash)."). Live, two of the three
// rows began with the same eight words, so the list could not be scanned at all:
// clamping the title to one line (RS-1) only moved the ellipsis, it did not make
// the rows distinguishable. Codex names its rows in 2–4 words ("Weekly status
// update draft", "cloc").
//
// We cannot invent a name — nothing in the journal carries one — but a prompt's
// FIRST CLAUSE is very nearly one, once the boilerplate tail is off it. So:
//
//   1. first sentence only          — the rest is elaboration, not identity
//   2. drop a trailing parenthetical — "(use write_file or bash)" names a tool,
//                                      not the task
//   3. drop trailing punctuation     — a name does not end in a full stop
//   4. cap at 48 chars on a word boundary
//
// The derivation is display-only and lossless in the UI: the caller keeps the
// raw text in the row's `title=`, so hovering still shows the whole prompt, and
// search still runs against the full string. A user rename beats all of it.

// Codex's titles run ~2–4 words; 48 characters is about the width the row's copy
// column can show before the ellipsis anyway, so nothing is lost visually.
export const SCHEDULED_TITLE_MAX = 48;

// A sentence ends at ".", "!" or "?" followed by whitespace/end-of-string — the
// whitespace requirement is what keeps "notes.md" and "v1.2" in one piece. CJK
// stops (。！？) need no such guard: they are never internal to a token.
const FIRST_SENTENCE = /^[\s\S]*?(?:[.!?](?=\s|$)|[。！？])/;

// Under this many characters, a "sentence" is more likely an abbreviation ("e.g.")
// than a clause worth cutting at.
const MIN_SENTENCE = 8;

const TRAILING_PUNCT = /[\s.,;:!?…。，、；：！？]+$/u;
const TRAILING_PAREN = /\s*[(（[【][^)）\]】]*[)）\]】]\s*$/u;

// scheduledTitle derives the row's display name from the raw prompt/label.
// `fallback` (the id) is used when there is no text at all.
export function scheduledTitle(raw: string | undefined, fallback = ""): string {
  let value = (raw || "").replace(/\s+/g, " ").trim();
  if (!value) return (fallback || "").trim();

  const sentence = value.match(FIRST_SENTENCE);
  if (sentence && sentence[0].trim().length >= MIN_SENTENCE) value = sentence[0];

  // "…, then commit it (use write_file or bash)." needs both strippers, in both
  // orders: the stop sits outside the parenthesis, and the parenthesis may leave
  // a comma behind it.
  for (let i = 0; i < 3; i++) {
    const before = value;
    value = value.replace(TRAILING_PUNCT, "").replace(TRAILING_PAREN, "");
    if (value === before) break;
  }
  value = value.trim();
  if (!value) return (raw || "").replace(/\s+/g, " ").trim() || (fallback || "").trim();

  if (value.length <= SCHEDULED_TITLE_MAX) return value;

  // Cut on a word boundary when there is one worth cutting at; CJK has no spaces,
  // so it takes the hard cut (a character is a unit there, unlike in English).
  const window = value.slice(0, SCHEDULED_TITLE_MAX);
  const space = window.lastIndexOf(" ");
  const body = space >= SCHEDULED_TITLE_MAX / 2 ? window.slice(0, space) : window;
  return body.replace(/[\s.,;:·，、；：-]+$/u, "") + "…";
}

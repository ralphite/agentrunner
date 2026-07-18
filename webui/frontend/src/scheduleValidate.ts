// Inline schedule validation (G36 余项): the backend is the real judge — these
// mirrors only catch the two shapes users actually mistype (prose instead of a
// Go duration, 6-field/quartz instead of 5-field cron) BEFORE a round-trip, so
// the form can say what's wrong next to the field instead of a post-submit
// toast. Empty input is "invalid but quiet": the field is required, the
// message only appears once something was typed.

// Go time.ParseDuration shape: one or more <number><unit> segments, decimals
// allowed, units ns/us/µs/ms/s/m/h (e.g. "5m", "1h30m", "1.5h").
export function validGoDuration(s: string): boolean {
  return /^([0-9]+(\.[0-9]+)?(ns|us|µs|μs|ms|s|m|h))+$/.test(s.trim());
}

// 5-field cron (min hour dom mon dow). Field syntax is left to the backend's
// parser — this only pins the field COUNT and a sane character set, which is
// where "every monday at 9" and 6-field quartz strings die.
export function validCron(s: string): boolean {
  const fields = s.trim().split(/\s+/);
  return fields.length === 5 && fields.every((f) => /^[0-9*,\-/A-Za-z]+$/.test(f));
}

// scheduleFieldError renders the inline message for a schedule form: "" when
// the value is valid OR still untouched (empty) — required-ness is the submit
// button's job, nagging an empty field helps nobody.
export function scheduleFieldError(kind: "interval" | "cron", value: string): string {
  const v = value.trim();
  if (!v) return "";
  if (kind === "interval") {
    return validGoDuration(v) ? "" : "Not a Go duration — try 30s, 5m, 1h or 1h30m.";
  }
  return validCron(v) ? "" : "Needs 5 space-separated cron fields (min hour dom mon dow), e.g. 0 8 * * 1-5.";
}

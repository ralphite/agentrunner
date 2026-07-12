export type RunPreset = "one-time" | "goal" | "repeating" | "best-of-n";

// The schedule kinds the run launcher can start (mirrors webui/schedule.go's
// driver spec kinds; `self_paced` is a driver's own choice, never one the UI
// hands out).
export type ScheduleKind = "immediate" | "interval" | "cron" | "parallel";

// CadenceSpec is a RHYTHM, stated the only way the backend understands one: the
// driver-spec fields themselves. SC-18 — the Scheduled page's suggestion cards
// used to carry their cadence as a hand-typed English string ("Weekdays at 8:00
// AM") while clicking one opened the launcher on the preset default of
// `interval: 5m`. The screen said one thing and built another. So a suggestion
// now owns a CadenceSpec, the card's words are RENDERED from it (cadenceText
// below), and the same spec prefills the modal — one fact, two views of it.
export interface CadenceSpec {
  schedule: ScheduleKind;
  interval?: string; // Go duration ("30m", "6h") — schedule: interval
  cron?: string; // five-field cron ("0 8 * * 1-5") — schedule: cron
  n?: number; // attempts — schedule: parallel
}

// The launcher's fallbacks when nothing prefills a field. They are what the
// Repeating preset has always opened on.
export const DEFAULT_INTERVAL = "5m";
export const DEFAULT_CRON = "0 * * * *";
export const DEFAULT_ATTEMPTS = 3;

export function runPresetDefaults(preset: RunPreset): { kind: "submit" | "drive"; schedule: ScheduleKind } {
  if (preset === "one-time") return { kind: "submit", schedule: "immediate" };
  if (preset === "repeating") return { kind: "drive", schedule: "interval" };
  if (preset === "best-of-n") return { kind: "drive", schedule: "parallel" };
  return { kind: "drive", schedule: "immediate" };
}

export interface RunFormDefaults {
  kind: "submit" | "drive";
  schedule: ScheduleKind;
  interval: string;
  cron: string;
  n: number;
}

// runFormDefaults is the launcher's opening state. Without a cadence it is the
// preset's (unchanged). WITH one — a suggestion card was clicked — the form
// opens on exactly the rhythm the card advertised: a cadence always describes a
// driver, so the run type follows too. The user can still change any of it; what
// they cannot get any more is a form that silently disagrees with what they
// clicked.
export function runFormDefaults(preset: RunPreset, cadence?: CadenceSpec): RunFormDefaults {
  const base = runPresetDefaults(preset);
  const form: RunFormDefaults = {
    ...base,
    interval: DEFAULT_INTERVAL,
    cron: DEFAULT_CRON,
    n: DEFAULT_ATTEMPTS,
  };
  if (!cadence) return form;
  form.kind = "drive";
  form.schedule = cadence.schedule;
  if (cadence.interval) form.interval = cadence.interval;
  if (cadence.cron) form.cron = cadence.cron;
  if (cadence.n && cadence.n >= 2) form.n = cadence.n;
  return form;
}

// ---- cadenceText: the spec rendered as the phrase a human reads ----
//
// A presentation-side mirror of webui/schedule.go's cadenceOf/cronPhrase, kept
// semantically identical (same five-field dialect, same phrases) so a card, a
// row, and the schedule the server ultimately reports all say the same words for
// the same spec. It decides nothing about when anything runs.

export function cadenceText(spec: CadenceSpec): string {
  switch (spec.schedule) {
    case "interval": {
      const sec = goDurationSeconds(spec.interval || "");
      if (sec === null || sec <= 0) return "Continuously";
      return "Every " + humanDuration(sec);
    }
    case "cron":
      return cronPhrase(spec.cron || "");
    case "parallel":
      return spec.n && spec.n >= 2 ? `Best of ${spec.n}` : "Best of N";
    default:
      return "Runs once";
  }
}

// goDurationSeconds parses the Go duration dialect the driver spec uses
// ("90s", "30m", "1h30m", "6h"). null = not a duration.
export function goDurationSeconds(d: string): number | null {
  const src = d.trim();
  if (!src) return null;
  const unit: Record<string, number> = { s: 1, m: 60, h: 3600, ms: 0.001, us: 1e-6, "µs": 1e-6, ns: 1e-9 };
  const re = /(\d+(?:\.\d+)?)(ns|us|µs|ms|h|m|s)/g;
  let total = 0;
  let consumed = 0;
  let match: RegExpExecArray | null;
  while ((match = re.exec(src)) !== null) {
    total += Number(match[1]) * unit[match[2]];
    consumed += match[0].length;
  }
  if (consumed !== src.length) return null; // trailing junk: not a duration
  return total;
}

// humanDuration renders a cadence duration compactly: 30m, 2h, 1h30m, 1d.
function humanDuration(sec: number): string {
  const s = Math.round(sec);
  const day = 86400;
  if (s % day === 0) return `${s / day}d`;
  if (s % 3600 === 0) return `${s / 3600}h`;
  if (s % 60 === 0) {
    const min = s / 60;
    if (min > 60) return `${Math.floor(min / 60)}h${min % 60}m`;
    return `${min}m`;
  }
  return `${s}s`;
}

// ---- cron (five fields: minute hour dom month dow; *, n, a-b, a,b, */n) ----

interface CronSchedule {
  minute: number[];
  hour: number[];
  dom: number[];
  month: number[];
  dow: number[];
  domStar: boolean;
  dowStar: boolean;
}

function cronField(f: string, min: number, max: number): { set: number[]; star: boolean } | null {
  const set = new Set<number>();
  let star = false;
  for (const rawPart of f.split(",")) {
    let part = rawPart.trim();
    if (!part) return null;
    let step = 1;
    const slash = part.indexOf("/");
    if (slash >= 0) {
      const n = Number(part.slice(slash + 1));
      if (!Number.isInteger(n) || n < 1) return null;
      step = n;
      part = part.slice(0, slash);
    }
    let lo = min;
    let hi = max;
    if (part !== "*") {
      const dash = part.indexOf("-");
      const a = Number(part.slice(0, dash >= 0 ? dash : undefined).trim());
      if (!Number.isInteger(a)) return null;
      lo = a;
      hi = a;
      if (dash >= 0) {
        const b = Number(part.slice(dash + 1).trim());
        if (!Number.isInteger(b)) return null;
        hi = b;
      } else if (step > 1) {
        hi = max; // "n/step" means n..max by step (Vixie cron, as internal/cron parses it)
      }
      if (lo < min || hi > max || lo > hi) return null;
    } else if (step === 1) {
      star = true;
    }
    for (let v = lo; v <= hi; v += step) set.add(v);
  }
  if (set.size === 0) return null;
  return { set: [...set].sort((a, b) => a - b), star: star && !f.includes(",") };
}

export function parseCron(expr: string): CronSchedule | null {
  const f = expr.trim().split(/\s+/).filter(Boolean);
  if (f.length !== 5) return null;
  const minute = cronField(f[0], 0, 59);
  const hour = cronField(f[1], 0, 23);
  const dom = cronField(f[2], 1, 31);
  const month = cronField(f[3], 1, 12);
  const dow = cronField(f[4], 0, 7);
  if (!minute || !hour || !dom || !month || !dow) return null;
  // 7 and 0 both mean Sunday.
  const dowSet = dow.set.filter((d) => d !== 7);
  if (dow.set.includes(7) && !dowSet.includes(0)) dowSet.unshift(0);
  return {
    minute: minute.set,
    hour: hour.set,
    dom: dom.set,
    month: month.set,
    dow: dowSet.sort((a, b) => a - b),
    domStar: dom.star,
    dowStar: dow.star,
  };
}

const DAY_NAMES = ["Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays"];
const DAY_ABBREV = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

// cronPhrase turns a cron expression into the sentence the Scheduled rows carry
// ("Saturdays at 4:00 AM", "Weekdays at 8:00 AM", "Every 6 hours"). Shapes it
// cannot phrase degrade to `Cron <expr>` — honest, never wrong.
export function cronPhrase(expr: string): string {
  const raw = expr.trim().split(/\s+/).filter(Boolean).join(" ");
  const sc = parseCron(expr);
  if (!sc) return ("Cron " + raw).trim();
  const fallback = "Cron " + raw;
  const mins = sc.minute;
  const hours = sc.hour;
  const monthStar = sc.month.length === 12;
  const calendarStar = sc.domStar && sc.dowStar && monthStar;

  if (calendarStar && hours.length === 24) {
    if (mins.length === 60) return "Every minute";
    const step = cronStep(mins, 60);
    if (step) return `Every ${step} minutes`;
    if (mins.length === 1) return `Hourly at :${String(mins[0]).padStart(2, "0")}`;
  }
  if (calendarStar && mins.length === 1 && hours.length > 1) {
    const step = cronStep(hours, 24);
    if (step && mins[0] === 0) return `Every ${step} hours`;
    return fallback;
  }
  if (mins.length !== 1 || hours.length !== 1) return fallback;
  const at = " at " + clockPhrase(hours[0], mins[0]);

  if (sc.domStar && !sc.dowStar && monthStar) {
    // weekly shapes
    const days = sc.dow;
    if (sameInts(days, [1, 2, 3, 4, 5])) return "Weekdays" + at;
    if (sameInts(days, [0, 6])) return "Weekends" + at;
    if (days.length === 7) return "Daily" + at;
    if (days.length === 1) return DAY_NAMES[days[0]] + at;
    return days.map((d) => DAY_ABBREV[d]).join(", ") + at;
  }
  if (sc.dowStar && !sc.domStar && monthStar) {
    // monthly shape
    const days = sc.dom;
    if (days.length === 1) return `Monthly on the ${ordinal(days[0])}${at}`;
    return fallback;
  }
  if (calendarStar) return "Daily" + at;
  return fallback;
}

// cronStep reports the stride of an evenly spaced set starting at 0 that wraps
// at `size` (what `*/n` produces), e.g. {0,15,30,45} over 60 → 15.
function cronStep(vals: number[], size: number): number | null {
  if (vals.length < 2 || vals[0] !== 0 || size % vals.length !== 0) return null;
  const step = size / vals.length;
  for (let i = 0; i < vals.length; i++) {
    if (vals[i] !== i * step) return null;
  }
  return step;
}

function sameInts(a: number[], b: number[]): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i]);
}

// clockPhrase renders a 24h wall time as "4:00 AM" / "12:30 PM".
function clockPhrase(h: number, m: number): string {
  const suffix = h >= 12 ? "PM" : "AM";
  const hh = h % 12 === 0 ? 12 : h % 12;
  return `${hh}:${String(m).padStart(2, "0")} ${suffix}`;
}

function ordinal(n: number): string {
  let suffix = "th";
  if (n % 100 < 11 || n % 100 > 13) {
    if (n % 10 === 1) suffix = "st";
    else if (n % 10 === 2) suffix = "nd";
    else if (n % 10 === 3) suffix = "rd";
  }
  return `${n}${suffix}`;
}

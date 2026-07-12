import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowClockwise,
  ArrowUp,
  ArrowUUpLeft,
  CaretDown,
  ChartBar,
  Code,
  Cpu,
  Desktop,
  Eye,
  File,
  Folder,
  GearSix,
  GitBranch,
  Image,
  Lightning,
  ListChecks,
  LockOpen,
  MagnifyingGlass,
  Microphone,
  PencilSimple,
  Plus,
  ShieldCheck,
  Sparkle,
  Stop as StopIcon,
  Target,
  UserCircle,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import "../styles.composer.css";
import { AR, uploadURL } from "../api";
import { useStore } from "../store";
import {
  ACCESS_LEVELS,
  accessById,
  buildBestOfNDriver,
  buildDriverAgent,
  buildLoopDriver,
  buildSpec,
  DEFAULT_ACCESS,
  DEFAULT_EFFORT,
  DEFAULT_MODEL,
  DEFAULT_PERSONA,
  DEFAULT_WORKER,
  EFFORT_LEVELS,
  effortById,
  effortFromSpec,
  MODELS,
  modelById,
  modelFromSpec,
  PERSONAS,
  personaById,
  personaFromSpec,
  replaceModel,
  runtimeModeTarget,
  type AccessId,
  type EffortId,
} from "../specs";
import { Popover, PopItem, PopSection } from "./Popover";
import { useVoice } from "./useVoice";
import { useDictation } from "./useDictation";
import { helperContext, runOptimize, undoOptimize } from "./composerOptimize";
import { parseSlash, SLASH, type SlashCmd } from "./slash";
import { recallAccess, recallDraft, recallSpec, rememberAccess, rememberDraft, rememberSpec } from "./sessionSpecs";
import { projectLabel, projectSubtitles } from "../viewModels";

// Actions the session variant wires in so slash commands can reach SessionView
// state (view switches, interrupt, fork…) that lives above the composer.
export interface SessionActions {
  interrupt?: () => void;
  showDiff?: () => void;
  fork?: () => void;
  switchAgentAdvanced?: () => void;
  resume?: () => void;
}

type ComposerProps =
  | { variant: "home"; onError: (m: string) => void; onProjectChange?: (label: string | null) => void }
  | {
      variant: "session";
      sid: string;
      workspace?: string;
      mode?: string; // the session's LIVE approval mode (SessionView lifts it from inspect; /mode switches it — INC-42)
      running?: boolean;
      onSend: (text: string, images: string[], files: string[], delivery?: "steer" | "queue") => Promise<void>;
      actions?: SessionActions;
      onError: (m: string) => void;
    };

interface Attachment {
  path: string;
  name: string;
  isImage: boolean;
}

// Last project used from the landing composer (RH-1). Codex opens New task with
// your previous repo already selected, so a task can go out in one keystroke —
// and, crucially, so the greeting headline and the project chip can never name
// different places. localStorage (not "the newest session") is the durable
// source: a session list can reorder under you (background runs, scratch
// sessions), while an explicit choice is what the user actually meant. Absent
// key = never chosen here (we seed from history once, below); "" = the user
// explicitly picked "Don't work in a project" and we must not re-seed over it.
const PROJECT_KEY = "arwebui.lastProject";
function recallProject(): string | null {
  try {
    return localStorage.getItem(PROJECT_KEY);
  } catch {
    return null;
  }
}
function rememberProject(workspace: string) {
  try {
    localStorage.setItem(PROJECT_KEY, workspace);
  } catch {
    /* ignore quota */
  }
}

const riskDot = (risk: string) => <span className={"risk-dot w-[7px] h-[7px] rounded-full shrink-0 inline-block " + risk} />;
// High-risk (Full access) reads as an amber warning glyph rather than a dot —
// Codex parity; low/med keep the quieter colored dot.
const riskGlyph = (risk: string) =>
  risk === "high" ? <WarningCircle size={15} weight="regular" className="shrink-0" style={{ color: "var(--amber)" }} /> : riskDot(risk);

export function Composer(props: ComposerProps) {
  const { select, selectRun, refreshSessions, refreshRuns, openModal, openPrompt, toast } = useStore();
  const allSessions = useStore((s) => s.sessions);
  // Recently used workspaces, newest first — picking an existing project must
  // be one click, not a hand-typed absolute path (W14).
  const recentWorkspaces = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const s of [...allSessions].sort((a, b) => b.id.localeCompare(a.id))) {
      const w = (s.workspace || "").trim().replace(/\/+$/, "");
      if (!w || seen.has(w)) continue;
      seen.add(w);
      out.push(w);
      if (out.length >= 5) break;
    }
    return out;
  }, [allSessions]);
  const isSession = props.variant === "session";

  // Per-session draft: initialize from what was typed here last time (the
  // component remounts on task switch), keep it saved as you type.
  const draftKey = isSession ? ((props as any).sid as string) : "~home";
  const [text, setText] = useState(() => recallDraft(draftKey));
  useEffect(() => rememberDraft(draftKey, text), [draftKey, text]);
  const [atts, setAtts] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  // Delivery mode for a send to a RUNNING session (INC-43, Codex parity):
  // "queue" (default) lands the message in the next turn; "steer" folds it into
  // the current turn at its next safe boundary. The toggle sets the default;
  // ⌘⏎ sends with the opposite mode for one message.
  const running = isSession && !!(props as { running?: boolean }).running;
  const [deliveryMode, setDeliveryMode] = useState<"steer" | "queue">("queue");

  // model + reasoning effort + access posture + persona
  const [provider, setProvider] = useState(DEFAULT_MODEL.provider);
  const [model, setModel] = useState(DEFAULT_MODEL.id);
  const [effort, setEffort] = useState<EffortId>(DEFAULT_EFFORT);
  // Advanced → thinking-budget override: an exact budget the effort presets
  // don't cover. null = use the effort preset. Chosen effort clears it.
  const [budgetOverride, setBudgetOverride] = useState<number | null>(null);
  // Model menu is a compact drill-in (root → Model/Effort) plus an Advanced
  // collapsible, mirroring the project menu's page-swap pattern.
  const [modelMenuPage, setModelMenuPage] = useState<"root" | "model" | "effort">("root");
  const [modelAdvancedOpen, setModelAdvancedOpen] = useState(false);
  // The home composer remembers the last chosen access level (W15); session
  // composers show the session's fixed posture instead and never read this.
  const [access, setAccessState] = useState<AccessId>(() => {
    try {
      const saved = localStorage.getItem("arwebui.lastAccess") as AccessId | null;
      if (saved && ACCESS_LEVELS.some((a) => a.id === saved)) return saved;
    } catch {
      /* ignore */
    }
    return DEFAULT_ACCESS;
  });
  const setAccess = (a: AccessId) => {
    setAccessState(a);
    try {
      localStorage.setItem("arwebui.lastAccess", a);
    } catch {
      /* ignore quota */
    }
  };
  const [persona, setPersona] = useState(DEFAULT_PERSONA);

  // Narrow phones (≤480px) can't fit the full "…, or type / for commands"
  // placeholder — it wraps to a second line that the single-row textarea clips
  // (review sw-m-02). Swap in a short placeholder there instead.
  const [narrow, setNarrow] = useState(() => window.matchMedia("(max-width: 480px)").matches);
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 480px)");
    const sync = () => setNarrow(mq.matches);
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);

  // home-only context. `ws` is the ONE source of truth for "where does this task
  // run" — the chip renders it, the send path uses it, and Home's headline
  // mirrors it through onProjectChange (RH-1). It opens on the last project the
  // user chose here.
  const [ws, setWs] = useState(() => recallProject() || "");
  const [kind, setKind] = useState<"chat" | "background">("chat");
  const [runLocation, setRunLocation] = useState<"worktree" | "local">("worktree");
  const [startingBranch, setStartingBranch] = useState("");
  const [projectQuery, setProjectQuery] = useState("");
  const [branchQuery, setBranchQuery] = useState("");
  const [projectMenuPage, setProjectMenuPage] = useState<"projects" | "new">("projects");
  const [branchInfo, setBranchInfo] = useState<{ isRepo: boolean; current: string; branches: string[]; dirty: number } | null>(null);

  // goal / loop / best-of-N launcher panel
  const [launcher, setLauncher] = useState<null | { mode: "goal" | "loop" | "best"; task: string }>(null);

  // slash menu
  const [slashOpen, setSlashOpen] = useState(false);
  const [slashIdx, setSlashIdx] = useState(0);

  // @-mention file picker (Codex file references): "@<query>" before the caret
  // opens a picker over the session workspace's files.
  const [atQuery, setAtQuery] = useState<string | null>(null);
  const [atFiles, setAtFiles] = useState<string[]>([]);
  const [atIdx, setAtIdx] = useState(0);
  const [atKnown, setAtKnown] = useState(true);

  const taRef = useRef<HTMLTextAreaElement>(null);
  const imgRef = useRef<HTMLInputElement>(null);
  const anyRef = useRef<HTMLInputElement>(null);

  // Prompt optimization (INC-56 · HANDA #19): the Sparkles button / `/optimize`
  // slash rewrites the draft via `ar optimize`; the pre-optimize draft is kept
  // here for a single-step undo. Cleared on send/reset.
  const [undoDraft, setUndoDraft] = useState<string | null>(null);
  const [optimizing, setOptimizing] = useState(false);

  const appendText = (t: string) => {
    setText((prev) => (prev ? prev + " " + t : t));
    taRef.current?.focus();
  };
  // Browser SpeechRecognition dictation — the fallback when server-side
  // dictation (MediaRecorder + `ar dictate`) isn't available.
  const voice = useVoice(appendText);
  // Context that helps the helpers spell proper nouns / resolve references: the
  // session's workspace label and whatever is already typed.
  const helperCtx = () => helperContext([isSession ? (props as any).workspace : ws, text]);
  const dictation = useDictation(appendText, helperCtx, (m) => props.onError(m));
  // Prefer the server path (better accuracy + context); fall back to the
  // browser's SpeechRecognition when the machine can't record+upload.
  const micActive = dictation.supported ? dictation.recording || dictation.busy : voice.listening;
  const micVisible = dictation.supported || voice.supported;
  const toggleMic = () => (dictation.supported ? dictation.toggle() : voice.toggle());

  // doOptimize rewrites `draft` and swaps it in, stashing `restoreTo` for undo.
  const doOptimize = async (draft: string, restoreTo: string) => {
    if (optimizing || !draft.trim()) return;
    setOptimizing(true);
    try {
      await runOptimize(
        {
          optimize: (d, c) => AR.optimize(d, c),
          setText: (t) => {
            setText(t);
            requestAnimationFrame(() => {
              const ta = taRef.current;
              if (ta) {
                ta.focus();
                grow(ta);
              }
            });
          },
          setUndo: setUndoDraft,
          toast,
          onError: props.onError,
        },
        draft,
        restoreTo,
        helperCtx(),
      );
    } finally {
      setOptimizing(false);
    }
  };
  const undoOptimizeNow = () => {
    if (undoDraft === null) return;
    undoOptimize({ setText, setUndo: setUndoDraft }, undoDraft);
    taRef.current?.focus();
  };

  // Seed model + persona pills from the session's remembered spec (if we made it).
  useEffect(() => {
    if (!isSession) return;
    const sp = recallSpec((props as any).sid);
    const m = sp ? modelFromSpec(sp) : null;
    if (m) {
      setProvider(m.provider);
      setModel(m.id);
    }
    const p = sp ? personaFromSpec(sp) : null;
    if (p) setPersona(p);
    if (sp) setEffort(effortFromSpec(sp));
  }, [isSession, isSession ? (props as any).sid : ""]);

  // Cold start (nothing remembered yet): seed the project from the most recent
  // real project in history, so a first-time visitor still lands on a selected
  // repo instead of a chip that says "Select project" under a headline that
  // names one (RH-1). Runs at most once, and never when the user has an explicit
  // stored choice — including the explicit "no project" ("").
  const seeded = useRef(false);
  useEffect(() => {
    if (isSession || seeded.current || recallProject() !== null) return;
    const candidate = recentWorkspaces.find((w) => {
      const label = projectLabel(w);
      return label !== "Scratch" && label !== "Other sessions";
    });
    if (!candidate) return; // sessions may still be loading — try again when they land
    seeded.current = true;
    setWs(candidate);
  }, [isSession, recentWorkspaces]);

  // Home: discover the workspace's branches when a real repo path is set.
  useEffect(() => {
    if (isSession) return;
    const dir = ws.trim();
    if (!dir || !dir.startsWith("/")) {
      setBranchInfo(null);
      return;
    }
    let alive = true;
    AR.gitBranches(dir)
      .then((b) => {
        if (!alive) return;
        setBranchInfo(b);
        setStartingBranch((b.current === "HEAD" ? "" : b.current) || b.branches[0] || "");
        if (!b.isRepo) setRunLocation("local");
      })
      .catch(() => alive && setBranchInfo(null));
    return () => {
      alive = false;
    };
  }, [isSession, ws]);

  // Report the selected project's display name up to Home so the welcome
  // headline can track the composer's single source of truth (`ws`) without
  // Home duplicating the selection state (W1).
  useEffect(() => {
    if (isSession) return;
    (props as Extract<ComposerProps, { variant: "home" }>).onProjectChange?.(ws.trim() ? projectLabel(ws) : null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ws, isSession]);

  // Same-basename projects ("ws", "Scratch") get a short disambiguating
  // subtitle in the picker; uniquely-named ones stay clean (W4).
  const projectSubs = useMemo(() => projectSubtitles(recentWorkspaces), [recentWorkspaces]);

  const modelLabel = modelById(provider, model)?.label || model;
  const effortLevel = effortById(effort);
  const accessLevel = isSession ? undefined : accessById(access);
  const remembered = isSession ? recallAccess((props as any).sid) : undefined;
  // Pill truth order (INC-42): a LIVE fold mode that names an access level
  // (acceptEdits/plan) always wins — /mode can change it mid-session. Live
  // "default" can't tell Full from Ask, so the remembered launch choice fills
  // in only while it doesn't contradict the live mode; a contradiction (e.g.
  // launched acceptEdits, later switched to default) reads as unknown rather
  // than a stale lie (QA Round1 F-C3 honesty rule).
  const liveMode = isSession ? ((props as any).mode as string | undefined) : undefined;
  const rememberedAccess = remembered ? accessById(remembered) : undefined;
  const sessionAccess = isSession
    ? (accessByMode(liveMode) ??
      (liveMode === undefined || (rememberedAccess && rememberedAccess.mode === "" && liveMode === "default")
        ? rememberedAccess
        : undefined))
    : undefined;

  const filteredSlash = useMemo(() => {
    const m = text.match(/^\/(\S*)$/);
    if (!m) return [];
    const q = m[1].toLowerCase();
    return SLASH.filter((c) => c.variants.includes(props.variant) && c.name.startsWith(q));
  }, [text, props.variant]);

  useEffect(() => {
    const show = filteredSlash.length > 0 && /^\/\S*$/.test(text);
    setSlashOpen(show);
    setSlashIdx(0);
  }, [filteredSlash.length, text]);

  // Detect "@query" at the caret (session composer only — needs a workspace).
  const atDetect = () => {
    if (!isSession) return null;
    const ta = taRef.current;
    if (!ta) return null;
    const upto = text.slice(0, ta.selectionStart ?? text.length);
    const m = upto.match(/(^|\s)@([\w./-]*)$/);
    return m ? m[2] : null;
  };
  const atSeq = useRef(0);
  useEffect(() => {
    const q = atDetect();
    setAtQuery(q);
    if (q === null) return;
    setAtIdx(0);
    const seq = ++atSeq.current; // drop out-of-order responses (stale query)
    const t = setTimeout(() => {
      AR.files((props as any).sid, q)
        .then((r) => {
          if (seq !== atSeq.current) return;
          setAtKnown(r.known);
          setAtFiles(r.files);
        })
        .catch(() => seq === atSeq.current && setAtFiles([]));
    }, 120);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text, isSession]);

  // Replace the "@query" token at the caret with the chosen path.
  const applyAt = (path: string) => {
    const ta = taRef.current;
    const pos = ta?.selectionStart ?? text.length;
    const upto = text.slice(0, pos);
    const start = upto.lastIndexOf("@");
    if (start === -1) return;
    const next = text.slice(0, start) + path + " " + text.slice(pos);
    setText(next);
    setAtQuery(null);
    requestAnimationFrame(() => {
      ta?.focus();
      const caret = start + path.length + 1;
      ta?.setSelectionRange(caret, caret);
    });
  };

  const ensureWs = async (): Promise<string> => {
    if (ws.trim()) return ws.trim();
    const p = (await AR.makeWorkspace()).path;
    setWs(p);
    return p;
  };

  const resolveHomeWorkspace = async (): Promise<string> => {
    const source = await ensureWs();
    if (runLocation !== "worktree" || !branchInfo?.isRepo) return source;
    return (await AR.makeWorktree(source, "", startingBranch || branchInfo.current)).path;
  };

  const resetInput = () => {
    setText("");
    setAtts([]);
    setUndoDraft(null);
    if (taRef.current) taRef.current.style.height = "auto";
  };

  // ---- attachments ----
  // Images ride --image; everything else (PDF, text, binary) rides --file
  // (INC-9). Both go through the CAS upload; the ≤10MB cap is the server's.
  const pick = async (file: File, isImage: boolean) => {
    if (file.size > 10 * 1024 * 1024) {
      props.onError(`${file.name} is larger than the 10 MB attachment limit.`);
      return;
    }
    try {
      const r = await AR.upload(file);
      setAtts((p) => [...p, { path: r.path, name: r.name, isImage }]);
    } catch (e: any) {
      props.onError(e.message);
    }
  };

  const onPaste = (e: React.ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    for (const it of items) {
      if (it.type.startsWith("image/")) {
        const f = it.getAsFile();
        if (f) {
          e.preventDefault();
          pick(f, true);
        }
      }
    }
  };

  // Drag a file (or image) onto the composer to attach it — images ride
  // --image, everything else --file, same as the picker.
  const dragDepth = useRef(0);
  const onDragEnter = (e: React.DragEvent) => {
    if (![...(e.dataTransfer?.types || [])].includes("Files")) return;
    e.preventDefault();
    dragDepth.current += 1;
    setDragging(true);
  };
  const onDragOver = (e: React.DragEvent) => {
    if (![...(e.dataTransfer?.types || [])].includes("Files")) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
  };
  const onDragLeave = () => {
    dragDepth.current = Math.max(0, dragDepth.current - 1);
    if (dragDepth.current === 0) setDragging(false);
  };
  const onDrop = (e: React.DragEvent) => {
    const files = e.dataTransfer?.files;
    if (!files || !files.length) return;
    e.preventDefault();
    dragDepth.current = 0;
    setDragging(false);
    for (const f of files) pick(f, f.type.startsWith("image/"));
  };

  // ---- submit / send ----
  // opposite (INC-43): ⌘⏎ sends with the opposite delivery mode for one message
  // (Codex parity). Only meaningful for a running session; ignored otherwise.
  const doSubmit = async (opposite = false) => {
    const t = text.trim();
    if (!t || busy) return;

    // Slash command? Run it instead of sending.
    const cmd = parseSlash(t, props.variant);
    if (cmd) {
      await runSlash(cmd.cmd, cmd.rest);
      return;
    }

    setBusy(true);
    try {
      if (isSession) {
        const imgs = atts.filter((a) => a.isImage).map((a) => a.path);
        const files = atts.filter((a) => !a.isImage).map((a) => a.path);
        // Delivery mode only matters while a turn is running; at idle a send just
        // starts the next turn either way, so leave it undefined then.
        const effective: "steer" | "queue" | undefined = running
          ? (opposite ? (deliveryMode === "queue" ? "steer" : "queue") : deliveryMode)
          : undefined;
        resetInput();
        await (props as Extract<ComposerProps, { variant: "session" }>).onSend(t, imgs, files, effective);
      } else if (kind === "chat") {
        const workspace = await resolveHomeWorkspace();
        const spec = buildSpec({ provider, model, access, persona, effort, budgetOverride });
        const imgs = atts.filter((a) => a.isImage).map((a) => a.path);
        const files = atts.filter((a) => !a.isImage).map((a) => a.path);
        const r = await AR.newSession({
          spec,
          extraSpecs: personaById(persona).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : [],
          workspace,
          message: t,
          mode: accessById(access).mode,
        });
        rememberSpec(r.sid, spec);
        rememberAccess(r.sid, access);
        resetInput();
        await refreshSessions();
        select(r.sid);
        // The opening message (`ar new`) can't carry attachments (DESIGN §9.1);
        // deliver a first-message attachment on an immediate follow-up so it
        // still works from the landing composer.
        if (imgs.length || files.length) {
          const n = imgs.length + files.length;
          try {
            await AR.send(r.sid, `(see attached file${n > 1 ? "s" : ""})`, imgs, files);
          } catch (e: any) {
            props.onError(e.message);
          }
        }
      } else {
        const workspace = await resolveHomeWorkspace();
        const spec = buildSpec({ provider, model, access, persona, effort, budgetOverride });
        const r = await AR.startRun({ kind: "submit", spec, extraSpecs: [], task: t, workspace, mode: accessById(access).mode, idem: "" });
        resetInput();
        await refreshRuns();
        selectRun(r.runId);
      }
    } catch (e: any) {
      props.onError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // ---- model / effort switch ----
  // Model and reasoning effort both live in the spec's model block, so a change
  // to either rebuilds that block (replaceModel) and, mid-session, re-agents the
  // session (the conversation carries over; it takes effect on the next message).
  const applyModelSpec = async (p: string, id: string, eff: EffortId, budget: number | null = budgetOverride) => {
    if (!isSession) return;
    const sid = (props as any).sid as string;
    try {
      const base = recallSpec(sid) || buildSpec({ provider: p, model: id, access: "full", persona, effort: eff, budgetOverride: budget });
      const spec = replaceModel(base, p, id, eff, budget);
      await AR.switchAgent(sid, spec, personaById(persona).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : []);
      rememberSpec(sid, spec);
    } catch (e: any) {
      props.onError(e.message);
    }
  };

  const chooseModel = async (p: string, id: string) => {
    setProvider(p);
    setModel(id);
    await applyModelSpec(p, id, effort);
    if (isSession) toast(`Model → ${modelById(p, id)?.label || id} (from your next message)`, "info");
  };

  const chooseEffort = async (eff: EffortId) => {
    setEffort(eff);
    // Picking a preset effort clears any custom thinking-budget override — the
    // two feed the same budget and the explicit choice should win.
    setBudgetOverride(null);
    await applyModelSpec(provider, model, eff, null);
    if (isSession) toast(`Reasoning → ${effortById(eff).label} (from your next message)`, "info");
  };

  // Advanced → thinking-budget override: an exact budget (0 / empty ⇒ back to
  // the effort preset). Rebuilds the model block just like an effort switch.
  const chooseBudgetOverride = async (budget: number | null) => {
    setBudgetOverride(budget);
    await applyModelSpec(provider, model, effort, budget);
    if (isSession) toast(budget ? `Thinking budget → ${budget} tokens (from your next message)` : "Thinking budget → effort preset", "info");
  };

  // ---- persona (agent template) switch ----
  // Mid-session this is a full spec swap via `ar agent` (decision #32): the
  // conversation carries over, the new shape takes effect on the next message.
  const choosePersona = async (id: string) => {
    setPersona(id);
    if (!isSession) return;
    const sid = (props as any).sid as string;
    try {
      const acc = (recallAccess(sid) as AccessId) || "full";
      const spec = buildSpec({ provider, model, access: acc, persona: id, effort, budgetOverride });
      const sib = personaById(id).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : [];
      await AR.switchAgent(sid, spec, sib);
      rememberSpec(sid, spec);
      toast(`Agent → ${personaById(id).label} (from your next message)`, "info");
    } catch (e: any) {
      props.onError(e.message);
    }
  };

  // ---- goal / loop ----
  // In-session goal everywhere (INC-D1/INC-10): the goal hangs on a session
  // and its context continues across checks. No verifier = self-certified —
  // the agent calls goal_complete when it's verifiably done (audited at the
  // turn boundary). On Home this creates the session first; the driver-goal
  // (fresh-run, batch) form stays reachable from the Background run modal.
  const startGoal = async (task: string, verifier: string, iterations: number) => {
    setBusy(true);
    try {
      let sid: string;
      if (isSession) {
        sid = (props as any).sid as string;
      } else {
        const workspace = await ensureWs();
        if (!workspace) return props.onError("a workspace is required to start a goal");
        const spec = buildSpec({ provider, model, access, persona, effort, budgetOverride });
        const r = await AR.newSession({
          spec,
          extraSpecs: personaById(persona).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : [],
          workspace,
          message: task,
          mode: accessById(access).mode,
        });
        rememberSpec(r.sid, spec);
        rememberAccess(r.sid, access);
        sid = r.sid;
      }
      try {
        await AR.goal(sid, { action: "attach", goal: task, verifier, maxChecks: iterations });
        toast(
          verifier
            ? "Goal attached — a verifier checks at each pause"
            : "Goal attached — the agent self-certifies once it's verifiably done",
          "info",
        );
      } catch (e: any) {
        // The Home path already created the session — surface it anyway so
        // the just-created (goal-less) session isn't an orphan.
        props.onError("goal attach failed: " + e.message);
      }
      setLauncher(null);
      resetInput();
      if (!isSession) {
        await refreshSessions();
        select(sid);
      }
    } catch (e: any) {
      props.onError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const startLoop = async (task: string, interval: string, iterations: number) => {
    const workspace = isSession ? (props as any).workspace || (await ensureWs()) : await ensureWs();
    if (!workspace) return props.onError("a workspace is required to start a loop");
    setBusy(true);
    try {
      const r = await AR.startRun({
        kind: "drive",
        spec: buildLoopDriver({ task, interval, maxIterations: iterations, provider, model }),
        extraSpecs: [{ name: "agent.yaml", content: buildDriverAgent({ provider, model }) }],
        task: "",
        workspace,
        mode: "",
        idem: "",
      });
      setLauncher(null);
      resetInput();
      await refreshRuns();
      selectRun(r.runId);
    } catch (e: any) {
      props.onError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // Best-of-N (schedule: parallel): N isolated attempts from one snapshot,
  // the verifier judges, the best result wins.
  const startBest = async (task: string, verifier: string, attempts: number) => {
    const workspace = isSession ? (props as any).workspace || (await ensureWs()) : await ensureWs();
    if (!workspace) return props.onError("a workspace is required to start a best-of-N run");
    setBusy(true);
    try {
      const r = await AR.startRun({
        kind: "drive",
        spec: buildBestOfNDriver({ task, n: attempts, verifier, provider, model }),
        extraSpecs: [{ name: "agent.yaml", content: buildDriverAgent({ provider, model }) }],
        task: "",
        workspace,
        mode: "",
        idem: "",
      });
      setLauncher(null);
      resetInput();
      await refreshRuns();
      selectRun(r.runId);
    } catch (e: any) {
      props.onError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // ---- runtime approval-mode switch (INC-42 chain; shared by `/mode` and the
  // session mode pill INC-54) ----
  // One command path so the pill and the slash produce identical durable
  // ControlMode commands, live folds, rejected receipts and toasts. The toast
  // is a delivery ack only — a busy session applies the switch at the next safe
  // boundary and the timeline chip (accepted) or rejected receipt is the truth.
  const switchMode = async (target: "default" | "acceptEdits") => {
    const sid = (props as any).sid as string;
    try {
      await AR.mode(sid, target);
      toast("Mode change requested — the timeline shows the outcome", "info");
    } catch (e: any) {
      props.onError(e.message);
    }
  };

  // ---- slash execution ----
  const runSlash = async (name: string, rest: string) => {
    const sid = isSession ? ((props as any).sid as string) : "";
    const act = isSession ? ((props as any).actions as SessionActions | undefined) : undefined;
    switch (name) {
      case "goal":
        // Codex parity: `/goal <text>` goes straight to work — no form. The
        // "+ menu → Goal" panel (or a bare `/goal`) still opens the launcher
        // for verifier / budget configuration.
        if (rest.trim()) {
          await startGoal(rest.trim(), "", 10);
          return;
        }
        setLauncher({ mode: "goal", task: rest });
        setText("");
        return;
      case "loop":
        setLauncher({ mode: "loop", task: rest });
        setText("");
        return;
      case "bestof":
        setLauncher({ mode: "best", task: rest });
        setText("");
        return;
      case "optimize":
        // `/optimize <draft>` rewrites the given draft; undo restores exactly
        // what the user typed after the command (not the "/optimize " prefix).
        await doOptimize(rest, rest);
        return;
      case "plan":
        setAccess("plan");
        setText("");
        toast("Plan mode — the next task runs read-only", "info");
        return;
      case "model": {
        const m = MODELS.find((x) => x.id === rest.trim()) || MODELS.find((x) => x.id.includes(rest.trim()));
        setText("");
        if (m) await chooseModel(m.provider, m.id);
        else toast(`No model matching "${rest}". Try: ${MODELS.map((x) => x.id).join(", ")}`, "info");
        return;
      }
      case "reasoning": {
        const q = rest.trim().toLowerCase().replace(/\s+/g, "");
        const aliases: Record<string, EffortId> = { off: "off", none: "off", low: "light", light: "light", medium: "medium", med: "medium", high: "high", xhigh: "xhigh", extrahigh: "xhigh", max: "xhigh" };
        const eff = aliases[q];
        setText("");
        if (eff) await chooseEffort(eff);
        else toast(`Unknown effort "${rest}". Try: ${EFFORT_LEVELS.map((e) => e.id).join(", ")}`, "info");
        return;
      }
      case "compact":
        setText("");
        try {
          await AR.compact(sid);
          // Delivery ack, not an outcome: a busy session applies it at the
          // next boundary and an empty prefix is a no-op (QA Round1 F-C6).
          toast("Compact requested — the timeline shows the outcome", "info");
        } catch (e: any) {
          props.onError(e.message);
        }
        return;
      case "clear":
        setText("");
        try {
          await AR.clear(sid);
          toast("Clear requested — the timeline shows the outcome", "info");
        } catch (e: any) {
          props.onError(e.message);
        }
        return;
      case "mode": {
        // plan/bypass are start-time choices; the loop is the final judge —
        // this just normalizes the two runtime targets.
        const q = rest.trim().toLowerCase().replace(/\s+/g, "");
        const target = q === "default" ? ("default" as const) : q === "acceptedits" ? ("acceptEdits" as const) : null;
        setText("");
        if (!target) {
          toast(`Unknown mode "${rest}". Try: default, acceptEdits`, "info");
          return;
        }
        await switchMode(target);
        return;
      }
      case "diff":
        setText("");
        act?.showDiff?.();
        return;
      case "fork":
        setText("");
        act?.fork?.();
        return;
      case "interrupt":
        setText("");
        act?.interrupt?.();
        return;
      case "resume":
        setText("");
        act?.resume?.();
        return;
    }
  };

  // ---- keyboard ----
  const onKey = (e: React.KeyboardEvent) => {
    if (atQuery !== null && atFiles.length) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setAtIdx((i) => (i + 1) % atFiles.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setAtIdx((i) => (i - 1 + atFiles.length) % atFiles.length);
        return;
      }
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
        e.preventDefault();
        applyAt(atFiles[atIdx]);
        return;
      }
      if (e.key === "Escape") {
        setAtQuery(null);
        return;
      }
    }
    if (slashOpen && filteredSlash.length) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSlashIdx((i) => (i + 1) % filteredSlash.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setSlashIdx((i) => (i - 1 + filteredSlash.length) % filteredSlash.length);
        return;
      }
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
        e.preventDefault();
        applySlash(filteredSlash[slashIdx]);
        return;
      }
      if (e.key === "Escape") {
        setSlashOpen(false);
        return;
      }
    }
    // ⌘⏎ / Ctrl+⏎ while running sends with the OPPOSITE delivery mode for one
    // message (INC-43, Codex parity). Stop it here so it doesn't bubble to the
    // global ⌘↵ = approve handler; an EMPTY composer lets it through to approve.
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      if (running && text.trim()) {
        e.preventDefault();
        e.stopPropagation();
        doSubmit(true);
      }
      return;
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      doSubmit();
    }
  };

  const applySlash = (c: SlashCmd) => {
    if (c.needsArgs) {
      setText(`/${c.name} `);
      setSlashOpen(false);
      taRef.current?.focus();
    } else {
      runSlash(c.name, "");
    }
  };

  const grow = (el: HTMLTextAreaElement) => {
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 220) + "px";
  };

  // Match Codex's quiet primary prompts exactly. Slash commands remain
  // discoverable by typing `/`; the placeholder should describe the user's
  // job, not advertise implementation mechanics.
  const placeholder = isSession
    ? "Ask for follow-up changes"
    : kind === "chat"
      ? "Do anything"
      : narrow
        ? "Describe a background task"
        : "Describe a background task";

  // Pill label: friendly name for a chosen workspace; before one exists, say
  // what will actually happen instead of the ambiguous "auto-created" (W2).
  const wsShort = ws ? projectLabel(ws) : "New scratch workspace";
  const normalizedWs = ws.trim().replace(/\/+$/, "");
  const filteredProjects = recentWorkspaces.filter((workspace) => {
    const query = projectQuery.trim().toLowerCase();
    return !query || projectLabel(workspace).toLowerCase().includes(query) || workspace.toLowerCase().includes(query);
  });
  const branchLabel = startingBranch || branchInfo?.current || (branchInfo?.isRepo ? "No commits yet" : "No branch");
  const filteredBranches = (branchInfo?.branches || []).filter((branch) =>
    branch.toLowerCase().includes(branchQuery.trim().toLowerCase()),
  );

  const chooseProject = (workspace: string) => {
    setWs(workspace);
    rememberProject(workspace);
    seeded.current = true;
    setProjectQuery("");
    setProjectMenuPage("projects");
    AR.gitBranches(workspace)
      .then((info) => {
        setBranchInfo(info);
        setStartingBranch((info.current === "HEAD" ? "" : info.current) || info.branches[0] || "");
        setRunLocation(info.isRepo ? "worktree" : "local");
      })
      .catch(() => {
        setBranchInfo(null);
        setStartingBranch("");
        setRunLocation("local");
      });
  };

  return (
    <div className={"cx " + (isSession ? "cx-session" : "cx-home")}>
      {launcher && (
        <GoalLoopLauncher
          mode={launcher.mode}
          initialTask={launcher.task}
          busy={busy}
          onCancel={() => setLauncher(null)}
          onStart={(task, a, b) =>
            launcher.mode === "goal" ? startGoal(task, a, b) : launcher.mode === "loop" ? startLoop(task, a, b) : startBest(task, a, b)
          }
        />
      )}

      <div
        className={"cx-card" + (dragging ? " dropping" : "")}
        onDragEnter={onDragEnter}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
      >
        {/* Codex exposes project, run location, environment and branch as four
            separate controls. They share submit state, but never share a menu:
            each choice has one meaning and remains scannable before typing. The
            chip row is the top of ONE composer card (P2): the card owns the
            single outer border + shadow; the strip only draws a hairline divider
            down to the input, so there's no double-rounded seam. */}
        {!isSession && (
          <div className="cx-env-strip">
          <Popover
            align="left"
            trigger={(open, toggle) => (
              <button className={"cx-env-control project" + (open ? " active" : "")} onClick={toggle} title="Select project" aria-haspopup="menu" aria-expanded={open}>
                <FolderIcon />
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">{ws ? wsShort : "Select project"}</span>
              </button>
            )}
            panelClass="cx-project-popover"
            onOpen={() => {
              setProjectQuery("");
              setProjectMenuPage("projects");
            }}
          >
            {(close) => (
              <div className="cx-menu project-menu">
                {projectMenuPage === "projects" ? (
                  <>
                    <label className="cx-project-search">
                      <MagnifyingGlass size={16} />
                      <input
                        data-popover-autofocus
                        aria-label="Search projects"
                        placeholder="Search projects"
                        value={projectQuery}
                        onChange={(event) => setProjectQuery(event.target.value)}
                      />
                    </label>
                    <div className="cx-project-list max-h-[180px] overflow-y-auto pb-[4px] border-b border-line-2">
                      {filteredProjects.map((workspace) => (
                        <PopItem
                          key={workspace}
                          icon={<FolderIcon />}
                          title={projectLabel(workspace)}
                          desc={projectSubs.get(workspace)}
                          active={workspace === normalizedWs}
                          onClick={() => { chooseProject(workspace); close(); }}
                        />
                      ))}
                      {filteredProjects.length === 0 && <div className="pop-empty">No projects found</div>}
                    </div>
                    <PopSection>
                      <PopItem icon={<PlusIcon />} title="New project" right={<span aria-hidden>›</span>} onClick={() => setProjectMenuPage("new")} />
                      <PopItem icon={<X size={15} />} title="Don't work in a project" active={!ws} onClick={() => { setWs(""); rememberProject(""); seeded.current = true; setBranchInfo(null); setStartingBranch(""); setRunLocation("local"); close(); }} />
                    </PopSection>
                  </>
                ) : (
                  <>
                    <div className="pop-menu-title">
                      <button className="pop-back" onClick={() => setProjectMenuPage("projects")} aria-label="Back to projects">‹</button>
                      <b>New project</b>
                    </div>
                    <PopItem icon={<Sparkle size={15} />} title="Start from scratch" desc="Create a fresh local workspace" onClick={async () => {
                      try { chooseProject((await AR.makeWorkspace()).path); } catch (error: any) { props.onError(error.message); }
                      close();
                    }} />
                    <PopItem icon={<FolderIcon />} title="Use an existing folder" desc="Choose an absolute local path" onClick={() => {
                      close();
                      openPrompt({ title: "Add project", label: "absolute folder path", initial: ws, placeholder: "/path/to/project", onSubmit: chooseProject });
                    }} />
                  </>
                )}
              </div>
            )}
          </Popover>

          <Popover
            align="left"
            trigger={(open, toggle) => (
              <button className={"cx-env-control" + (open ? " active" : "")} onClick={toggle} title="Choose where this task runs" aria-haspopup="menu" aria-expanded={open}>
                {runLocation === "local" ? <Desktop size={17} /> : <GitBranch size={17} />}
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">{runLocation === "local" ? "Local" : "New worktree"}</span>
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu">
                <PopSection label="Run location">
                  <PopItem icon={<GitBranch size={15} />} title="New worktree" desc={branchInfo?.isRepo ? "Isolated checkout; your project stays untouched" : "Select a Git project first"} active={runLocation === "worktree"} onClick={() => {
                    if (!branchInfo?.isRepo) { props.onError("New worktree needs a Git project."); return; }
                    setRunLocation("worktree"); close();
                  }} />
                  <PopItem icon={<Desktop size={15} />} title="Local" desc="Work directly in the selected project" active={runLocation === "local"} onClick={() => { setRunLocation("local"); close(); }} />
                </PopSection>
                <PopSection label="Task type">
                  <PopItem icon={<StartIcon />} title="Interactive session" active={kind === "chat"} onClick={() => { setKind("chat"); close(); }} />
                  <PopItem icon={<Lightning size={15} />} title="Background task" active={kind === "background"} onClick={() => { setKind("background"); close(); }} />
                </PopSection>
              </div>
            )}
          </Popover>

          <Popover
            align="left"
            trigger={(open, toggle) => (
              <button className={"cx-env-control" + (open ? " active" : "")} onClick={toggle} title="Select local environment" aria-haspopup="menu" aria-expanded={open}>
                <GearSix size={17} />
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">No environment</span>
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu environment-menu">
                <PopSection label="Local environment">
                  <PopItem icon={<GearSix size={15} />} title="No environment" desc="Use AgentRunner's current local runtime" active onClick={close} />
                </PopSection>
              </div>
            )}
          </Popover>

          <Popover
            align="left"
            panelClass="cx-branch-popover"
            onOpen={() => {
              setBranchQuery("");
              if (ws.trim()) AR.gitBranches(ws.trim()).then((info) => {
                setBranchInfo(info);
                if (!startingBranch) setStartingBranch((info.current === "HEAD" ? "" : info.current) || info.branches[0] || "");
              }).catch(() => {});
            }}
            trigger={(open, toggle) => (
              <button className={"cx-env-control branch" + (open ? " active" : "")} onClick={toggle} title={branchInfo?.isRepo ? "Choose starting branch" : "No Git branch available"} disabled={!branchInfo?.isRepo} aria-haspopup="menu" aria-expanded={open}>
                <BranchIcon />
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">{branchLabel}</span>
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu branch-menu">
                <label className="cx-project-search cx-branch-search">
                  <MagnifyingGlass size={16} />
                  <input
                    data-popover-autofocus
                    aria-label="Search branches"
                    placeholder="Search branches"
                    value={branchQuery}
                    onChange={(event) => setBranchQuery(event.target.value)}
                  />
                </label>
                <PopSection label={runLocation === "worktree" ? "Start worktree from" : `Local branch${branchInfo?.dirty ? ` · ${branchInfo.dirty} uncommitted` : ""}`}>
                  {filteredBranches.map((branch) => (
                    <PopItem key={branch} icon={<BranchIcon />} title={branch} active={branch === branchLabel} onClick={async () => {
                      if (runLocation === "worktree") {
                        setStartingBranch(branch);
                        close();
                        return;
                      }
                      try {
                        await AR.gitCheckout(ws.trim(), branch, false);
                        setBranchInfo((current) => current ? { ...current, current: branch } : current);
                        setStartingBranch(branch);
                        toast(`Switched to ${branch}`, "info");
                        close();
                      } catch (error: any) {
                        props.onError(error.message);
                      }
                    }} />
                  ))}
                  {branchInfo?.branches.length === 0 && <div className="pop-empty">No branches yet</div>}
                  {(branchInfo?.branches.length || 0) > 0 && filteredBranches.length === 0 && <div className="pop-empty">No branches found</div>}
                </PopSection>
              </div>
            )}
          </Popover>
        </div>
        )}

        {dragging && (
          <div className="cx-drop absolute inset-0 z-[5] grid place-items-center rounded-[22px] border-2 border-dashed border-blue text-blue text-[13.5px] font-medium pointer-events-none">
            <span>Drop files to attach</span>
          </div>
        )}
        {atts.length > 0 && (
          <div className="cx-atts flex flex-wrap gap-[6px] pt-[12px] px-[14px]">
            {atts.map((a, i) => (
              <span className="cx-att cx-att-codex" key={i} onClick={() => setAtts((p) => p.filter((_, j) => j !== i))} title="Remove attachment">
                {a.isImage ? (
                  <img className="cx-att-thumb" src={uploadURL(a.path)} alt={a.name} />
                ) : (
                  <span className="cx-att-ico"><File size={14} /></span>
                )}
                <span className="cx-att-name">{a.name}</span>
                <span className="cx-att-x" aria-hidden><X size={11} weight="bold" /></span>
              </span>
            ))}
          </div>
        )}

        <div className="cx-input-wrap">
          <textarea
            ref={taRef}
            value={text}
            placeholder={placeholder}
            onChange={(e) => {
              setText(e.target.value);
              grow(e.target);
            }}
            onKeyDown={onKey}
            onPaste={onPaste}
            rows={1}
          />
        </div>

        {atQuery !== null && (
          <div className="cx-slash cx-at">
            <div className="cx-slash-hd">{atKnown ? "Files · @" + atQuery : "Workspace unknown"}</div>
            {!atKnown && <div className="cx-at-empty">This session's workspace isn't known to arwebui, so files can't be listed.</div>}
            {atKnown && atFiles.length === 0 && <div className="cx-at-empty">No matching files</div>}
            {atFiles.map((f, i) => (
              <button
                key={f}
                className={"cx-slash-item" + (i === atIdx ? " on" : "")}
                onMouseEnter={() => setAtIdx(i)}
                onClick={() => applyAt(f)}
              >
                <span className="cx-slash-name mono">{f}</span>
              </button>
            ))}
          </div>
        )}

        {slashOpen && filteredSlash.length > 0 && (
          <div className="cx-slash cx-slash-codex">
            <div className="cx-slash-hd">Commands</div>
            {filteredSlash.map((c, i) => (
              <button
                key={c.name}
                className={"cx-slash-item" + (i === slashIdx ? " on" : "")}
                onMouseEnter={() => setSlashIdx(i)}
                onClick={() => applySlash(c)}
              >
                <span className="cx-slash-text">
                  <span className="cx-slash-name">/{c.name}</span>
                  <span className="cx-slash-desc">{c.desc}</span>
                </span>
                {c.arg && <span className="cx-slash-hint">{c.arg}</span>}
              </button>
            ))}
          </div>
        )}

        {/* ---- control bar ---- */}
        <div className="cx-bar">
          {/* One Codex-style `+` menu (C1): Codex folds attachments and task
              actions behind a single `+`. We do the same — grouped sections
              (Add / Task options / Agent / Advanced), each row a one-line gray
              description — instead of a second quiet control. */}
          <Popover
            align="left"
            panelClass="cx-pop-codex"
            trigger={(open, toggle) => (
              <button className={"cx-icon" + (open ? " active" : "")} onClick={toggle} title="Add & task options" aria-label="Add and task options" aria-haspopup="menu" aria-expanded={open}>
                <PlusIcon />
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu wide cx-add-menu">
                <PopSection label="Add">
                  <PopItem icon={<Image size={16} />} title="Images" desc="Paste, drop, or pick one or more images" onClick={() => { close(); imgRef.current?.click(); }} />
                  <PopItem icon={<File size={16} />} title="Files" desc="PDF, text, or any files (≤10MB each)" onClick={() => { close(); anyRef.current?.click(); }} />
                </PopSection>
                <PopSection label="Task options">
                  <PopItem icon={<GoalIcon />} title="Goal" desc="Keep working until the goal is met" onClick={() => { close(); setLauncher({ mode: "goal", task: text.trim() }); }} />
                  <PopItem icon={<LoopIcon />} title="Loop" desc="Repeat on a fixed cadence" onClick={() => { close(); setLauncher({ mode: "loop", task: text.trim() }); }} />
                  <PopItem icon={<BestIcon />} title="Best of N" desc="Run isolated attempts and keep the best" onClick={() => { close(); setLauncher({ mode: "best", task: text.trim() }); }} />
                  {!isSession && <PopItem icon={<PlanIcon />} title="Plan mode" desc="Read-only planning — no changes" active={access === "plan"} onClick={() => { close(); setAccess("plan"); }} />}
                </PopSection>
                <PopSection label="Agent">
                  {PERSONAS.map((item) => (
                    <PopItem key={item.id} icon={<PersonaIcon />} title={item.label} desc={item.desc} active={persona === item.id} onClick={() => { choosePersona(item.id); close(); }} />
                  ))}
                </PopSection>
                <PopSection label="Advanced">
                  <PopItem icon={<Code size={16} />} title="Edit agent spec (YAML)…" onClick={() => { close(); openModal(isSession ? { kind: "agent", sid: (props as any).sid } : { kind: "new", message: text }); }} />
                </PopSection>
              </div>
            )}
          </Popover>

          {/* permission mode pill */}
          {isSession ? (
            /* Session mode switch (INC-54): the live approval-mode pill is a
               click-to-switch selector, not a read-only badge. Only the two
               runtime transitions the daemon accepts (Ask↔Auto-accept edits,
               INC-42 ValidTransition) are clickable; Full access and Plan are
               listed disabled with the reason so the menu structure matches
               Home's while staying honest about what's switchable at runtime.
               Every switch runs the same ControlMode command chain as `/mode`,
               so live folds, rejected receipts and toasts are identical. The
               active row follows the pill's truth-ordered value (live >
               non-contradicting remembered > honest unknown); when the live
               mode is unknown, nothing is highlighted. */
            <Popover
              align="left"
              panelClass="cx-pop-codex"
              trigger={(open, toggle) => (
                <button
                  className={"cx-pill cx-mode " + (sessionAccess?.risk || "unknown") + (open ? " active" : "")}
                  onClick={toggle}
                  aria-haspopup="menu"
                  aria-expanded={open}
                  title={
                    sessionAccess
                      ? "The session's live approval mode — click to switch Ask ↔ Auto-accept edits"
                      : "This session's approval posture comes from its spec's permission rules; switch Ask ↔ Auto-accept edits here, and approvals surface when a gate asks"
                  }
                >
                  {riskGlyph(sessionAccess?.risk || "unknown")}
                  {sessionAccess?.label || "Access: set by agent spec"}
                  <Caret />
                </button>
              )}
            >
              {(close) => (
                <div className="cx-menu wide cx-access-menu">
                  <PopSection label="Switch approval mode">
                    {ACCESS_LEVELS.map((a) => {
                      const target = runtimeModeTarget(a.id);
                      const desc =
                        a.id === "full"
                          ? "Set at launch — mid-session switching only toggles Ask ↔ Auto-accept edits"
                          : a.id === "plan"
                            ? "Plan mode exits through an approval, not this switch"
                            : a.desc;
                      return (
                        <PopItem
                          key={a.id}
                          icon={<AccessIcon id={a.id} risk={a.risk} />}
                          title={a.label}
                          desc={desc}
                          active={sessionAccess?.id === a.id}
                          disabled={target === null}
                          onClick={target ? () => { switchMode(target); close(); } : undefined}
                        />
                      );
                    })}
                  </PopSection>
                  <div className="cx-pop-note">Approvals still surface here whenever a gate asks. Full access and Plan are fixed once the task starts.</div>
                </div>
              )}
            </Popover>
          ) : (
            <Popover
              align="left"
              panelClass="cx-pop-codex"
              trigger={(open, toggle) => (
                <button className={"cx-pill cx-mode " + (accessLevel?.risk || "low") + (open ? " active" : "")} onClick={toggle} title="How the agent's actions are approved" aria-haspopup="menu" aria-expanded={open}>
                  {riskGlyph(accessLevel?.risk || "low")}
                  {accessLevel?.label}
                  <Caret />
                </button>
              )}
            >
              {(close) => (
                <div className="cx-menu wide cx-access-menu">
                  <PopSection label="How should actions be approved?">
                    {ACCESS_LEVELS.map((a) => (
                      <PopItem
                        key={a.id}
                        icon={<AccessIcon id={a.id} risk={a.risk} />}
                        title={a.label}
                        desc={a.desc}
                        active={access === a.id}
                        onClick={() => { setAccess(a.id); close(); }}
                      />
                    ))}
                  </PopSection>
                  <div className="cx-pop-note">Approvals still surface here whenever a gate asks; the posture is fixed once the task starts.</div>
                </div>
              )}
            </Popover>
          )}

          <span className="cx-spacer" />

          {/* model pill — shows "<model> · <effort>" like Codex's "5.6 Sol Extra High" */}
          <Popover
            align="right"
            panelClass="cx-pop-codex"
            onOpen={() => { setModelMenuPage("root"); setModelAdvancedOpen(false); }}
            trigger={(open, toggle) => (
              <button className={"cx-pill cx-model" + (open ? " active" : "")} onClick={toggle} title="Model & effort" aria-haspopup="menu" aria-expanded={open}>
                {modelLabel}
                {(budgetOverride || effort !== "off") && <span className="cx-pill-sub">{budgetOverride ? "Custom" : effortLevel.label}</span>}
                <Caret />
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu wide cx-model-menu">
                {/* Compact drill-in: Model / Effort each show label + current
                    value + chevron and open a choice list; an Advanced
                    collapsible holds the overflow controls. */}
                {modelMenuPage === "root" ? (
                  <>
                    <div className="cx-model-rows">
                      <button className="cx-model-row" onClick={() => setModelMenuPage("model")} aria-label="Choose model">
                        <span className="cx-model-row-label">Model</span>
                        <span className="cx-model-row-value">{modelLabel}</span>
                        <CaretDown className="cx-model-row-chev" size={13} />
                      </button>
                      <button className="cx-model-row" onClick={() => setModelMenuPage("effort")} aria-label="Choose reasoning effort">
                        <span className="cx-model-row-label">Effort</span>
                        <span className="cx-model-row-value">{budgetOverride ? `Custom · ${budgetOverride}` : effortLevel.label}</span>
                        <CaretDown className="cx-model-row-chev" size={13} />
                      </button>
                    </div>
                    <div className="cx-model-advanced">
                      <button
                        className="cx-model-adv-toggle"
                        onClick={() => setModelAdvancedOpen((v) => !v)}
                        aria-expanded={modelAdvancedOpen}
                      >
                        <span>Advanced</span>
                        <CaretDown className={"cx-model-adv-chev" + (modelAdvancedOpen ? " open" : "")} size={13} />
                      </button>
                      {modelAdvancedOpen && (
                        <div className="cx-model-adv-body">
                          <PopItem
                            icon={<Code size={15} />}
                            title="Custom model id…"
                            desc={`provider stays ${provider}`}
                            onClick={() => {
                              close();
                              openPrompt({
                                title: "Custom model id",
                                label: "model id (provider stays " + provider + ")",
                                initial: model,
                                onSubmit: (id) => chooseModel(provider, id),
                              });
                            }}
                          />
                          <PopItem
                            icon={<ChartBar size={15} />}
                            title="Thinking budget override…"
                            desc={budgetOverride ? `${budgetOverride} tokens` : "use the effort preset"}
                            onClick={() => {
                              close();
                              openPrompt({
                                title: "Thinking budget override",
                                label: "budget tokens (0 or empty = use the effort preset)",
                                initial: budgetOverride != null ? String(budgetOverride) : "",
                                onSubmit: (v) => {
                                  const n = Number(v.trim());
                                  chooseBudgetOverride(Number.isFinite(n) && n > 0 ? Math.floor(n) : null);
                                },
                              });
                            }}
                          />
                        </div>
                      )}
                    </div>
                  </>
                ) : modelMenuPage === "model" ? (
                  <>
                    <div className="pop-menu-title">
                      <button className="pop-back" onClick={() => setModelMenuPage("root")} aria-label="Back">‹</button>
                      <b>Model</b>
                    </div>
                    {MODELS.map((m) => (
                      <PopItem
                        key={m.provider + m.id}
                        icon={<ModelIcon provider={m.provider} />}
                        title={m.label}
                        desc={m.sub}
                        active={provider === m.provider && model === m.id}
                        onClick={() => { chooseModel(m.provider, m.id); setModelMenuPage("root"); close(); }}
                      />
                    ))}
                  </>
                ) : (
                  <>
                    <div className="pop-menu-title">
                      <button className="pop-back" onClick={() => setModelMenuPage("root")} aria-label="Back">‹</button>
                      <b>Effort</b>
                    </div>
                    {EFFORT_LEVELS.map((e) => (
                      <PopItem
                        key={e.id}
                        icon={<EffortIcon level={e.id} />}
                        title={e.label}
                        desc={e.desc}
                        active={!budgetOverride && effort === e.id}
                        onClick={() => { chooseEffort(e.id); setModelMenuPage("root"); close(); }}
                      />
                    ))}
                  </>
                )}
              </div>
            )}
          </Popover>

          {/* optimize (INC-56 · HANDA #19): rewrite the draft via `ar optimize`.
              After a rewrite an Undo button restores the original draft in one
              step. Both are hidden when there's nothing to act on. */}
          {undoDraft !== null ? (
            <button className="cx-icon cx-undo" onClick={undoOptimizeNow} title="Undo optimize — restore your original draft">
              <UndoIcon />
            </button>
          ) : (
            text.trim() && (
              <button
                className={"cx-icon cx-optimize" + (optimizing ? " working" : "")}
                onClick={() => doOptimize(text, text)}
                disabled={optimizing}
                title="Optimize prompt — rewrite this draft to be clearer"
              >
                <Sparkle size={15} weight={optimizing ? "fill" : "regular"} />
              </button>
            )
          )}

          {/* dictation (INC-56 · HANDA #18): server-side `ar dictate` when the
              browser can record+upload, else browser SpeechRecognition. */}
          {micVisible && (
            <button
              className={"cx-icon cx-mic" + (micActive ? " listening" : "") + (dictation.busy ? " working" : "")}
              onClick={toggleMic}
              disabled={dictation.busy}
              title={dictation.busy ? "Transcribing…" : micActive ? "Stop dictation" : "Dictate"}
            >
              <MicIcon />
            </button>
          )}

          {/* delivery mode (INC-43, Codex parity): while a turn is running, choose
              whether this message steers the current turn or queues for the next.
              ⌘⏎ sends with the opposite mode for one message. */}
          {running && (
            <div className="cx-delivery" role="group" aria-label="Delivery mode">
              <button
                type="button"
                className={"cx-deliv" + (deliveryMode === "queue" ? " on" : "")}
                onClick={() => setDeliveryMode("queue")}
                title="Queue: deliver after the current turn ends (⌘⏎ to steer this one)"
              >
                Queue
              </button>
              <button
                type="button"
                className={"cx-deliv" + (deliveryMode === "steer" ? " on" : "")}
                onClick={() => setDeliveryMode("steer")}
                title="Steer: fold into the current turn at its next safe boundary (⌘⏎ to queue this one)"
              >
                Steer
              </button>
            </div>
          )}

          {/* send — or Stop while a turn is running and nothing is typed
              (W30: stopping shouldn't require finding the topbar button) */}
          {isSession && (props as { running?: boolean }).running && !text.trim() ? (
            <button
              className="cx-send cx-stop"
              onClick={() => (props as { actions?: SessionActions }).actions?.interrupt?.()}
              title="Stop the active turn"
            >
              <StopIcon size={15} weight="fill" />
            </button>
          ) : (
            <button
              className="cx-send"
              onClick={() => doSubmit()}
              disabled={busy || !text.trim()}
              title={running ? `Send · ${deliveryMode} (⌘⏎ to ${deliveryMode === "queue" ? "steer" : "queue"})` : "Send (Enter)"}
            >
              <ArrowUp />
            </button>
          )}
        </div>

      </div>

      {isSession && <div className="cx-status">{(props as any).running ? "running…" : "ready"}</div>}

      {/* hidden file inputs — multi-select (mirror the drag-drop multi-file
          loop): loop over every chosen file instead of taking files[0]. */}
      <input type="file" accept="image/*" multiple ref={imgRef} style={{ display: "none" }} onChange={(e) => { for (const f of Array.from(e.target.files || [])) pick(f, true); e.target.value = ""; }} />
      <input type="file" multiple ref={anyRef} style={{ display: "none" }} onChange={(e) => { for (const f of Array.from(e.target.files || [])) pick(f, f.type.startsWith("image/")); e.target.value = ""; }} />
    </div>
  );
}

// ---- goal / loop / best-of-N launcher ---------------------------------------
function GoalLoopLauncher({
  mode,
  initialTask,
  busy,
  onCancel,
  onStart,
}: {
  mode: "goal" | "loop" | "best";
  initialTask: string;
  busy: boolean;
  onCancel: () => void;
  onStart: (task: string, second: string, iterations: number) => void;
}) {
  const [task, setTask] = useState(initialTask);
  const [second, setSecond] = useState(mode === "loop" ? "5m" : ""); // interval | verifier
  const [iters, setIters] = useState(mode === "goal" ? 10 : mode === "loop" ? 5 : 3); // rounds | attempts
  const meta = {
    goal: { icon: <GoalIcon />, label: "Goal", hint: "iterate until the goal is met", start: "Start goal" },
    loop: { icon: <LoopIcon />, label: "Loop", hint: "repeat on a fixed cadence", start: "Start loop" },
    best: { icon: <BestIcon />, label: "Best of N", hint: "N isolated attempts, the verifier picks the best", start: "Start best-of-N" },
  }[mode];
  return (
    <div className="cx-launcher">
      <div className="cx-launcher-hd">
        {meta.icon}
        <b>{meta.label}</b>
        <span className="dim">{meta.hint}</span>
        <span className="cx-spacer" />
        <button className="ghost sm" onClick={onCancel} aria-label="Close launcher"><X size={13} /></button>
      </div>
      <textarea className="cx-launcher-task" rows={2} placeholder={mode === "goal" ? "What goal should the agent keep working toward?" : mode === "loop" ? "What should each iteration do?" : "What should each attempt try to do?"} value={task} onChange={(e) => setTask(e.target.value)} />
      <div className="cx-launcher-row">
        {mode === "loop" ? (
          <label className="cx-launcher-field" title="How often to run (Go duration, e.g. 30s, 5m, 1h)">
            <span>Every</span>
            <input placeholder="5m" value={second} onChange={(e) => setSecond(e.target.value)} />
          </label>
        ) : (
          <label className="cx-launcher-field" title={mode === "goal" ? "A shell command that must exit 0 for the goal to count as met. Optional — leave it empty and the agent self-certifies: it calls goal_complete when the goal is verifiably done (audited at the turn boundary)" : "A shell command that judges each attempt — exit 0 = pass (optional; without it the earliest attempt wins)"}>
            <span>{mode === "goal" ? "Done when (command)" : "Judge with (command)"}</span>
            <input placeholder={mode === "goal" ? "e.g. go test ./…  (empty = agent self-certifies)" : "e.g. go test ./…  (optional)"} value={second} onChange={(e) => setSecond(e.target.value)} />
          </label>
        )}
        <label className="cx-launcher-field small" title={mode === "best" ? "How many isolated attempts to run" : "Safety cap on iterations"}>
          <span>{mode === "best" ? "Attempts" : "Max rounds"}</span>
          <input type="number" min={mode === "best" ? 2 : 1} value={iters} onChange={(e) => setIters(Math.max(mode === "best" ? 2 : 1, Number(e.target.value) || 1))} />
        </label>
        <button className="primary cx-launcher-go" disabled={busy || !task.trim() || (mode === "loop" && !second.trim())} onClick={() => onStart(task.trim(), second.trim(), iters)}>
          {meta.start}
        </button>
      </div>
    </div>
  );
}

// ---- helpers ----
function accessByMode(mode?: string) {
  if (mode === "plan") return ACCESS_LEVELS.find((a) => a.id === "plan")!;
  if (mode === "acceptEdits") return ACCESS_LEVELS.find((a) => a.id === "acceptEdits")!;
  // mode "default" cannot tell Full access from Ask — the difference lives
  // in the spec's permissions rules. Guessing "ask" misled users into
  // thinking a free-running session was gated (QA Round1 F-C3): report
  // unknown instead and let the pill say so honestly.
  return undefined;
}

// Phosphor's regular weight is the closest match to Codex's quiet line icons.
const Caret = () => <CaretDown className="cx-caret text-dim shrink-0" size={10} />;
const PlusIcon = () => <Plus size={16} />;
const MicIcon = () => <Microphone size={15} />;
const UndoIcon = () => <ArrowUUpLeft size={15} />;
// Provider-aware model glyph: Gemini (primary) gets the sparkle, Anthropic the
// chip — a quiet family cue in the pill and the model menu.
const ModelIcon = ({ provider }: { provider?: string }) => (provider === "anthropic" ? <Cpu size={14} /> : <Sparkle size={14} />);
const FolderIcon = () => <Folder size={13} />;
const BranchIcon = () => <GitBranch size={13} />;
const StartIcon = () => <Desktop size={13} />;
const GoalIcon = () => <Target size={14} />;
const LoopIcon = () => <ArrowClockwise size={14} />;
const PlanIcon = () => <ListChecks size={14} />;
const EffortIcon = ({ level }: { level: EffortId }) => <ChartBar size={14} weight={level === "off" ? "regular" : "fill"} />;
const BestIcon = () => <ChartBar size={14} />;
const PersonaIcon = () => <UserCircle size={13} />;

// Access-mode icons (C2): each approval posture gets a distinct Codex-style
// line icon, tinted by its risk so the menu keeps the pill's risk language.
const ACCESS_ICONS: Record<AccessId, typeof LockOpen> = {
  full: LockOpen,
  ask: ShieldCheck,
  acceptEdits: PencilSimple,
  plan: Eye,
};
const AccessIcon = ({ id, risk }: { id: AccessId; risk: string }) => {
  const I = ACCESS_ICONS[id] || ShieldCheck;
  return (
    <span className={"cx-access-ico " + risk}>
      <I size={16} />
    </span>
  );
};

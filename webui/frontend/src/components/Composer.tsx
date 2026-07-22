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
  GitBranch,
  GitFork,
  Lightning,
  ListChecks,
  LockOpen,
  MagnifyingGlass,
  Microphone,
  Paperclip,
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
import { AR, uploadURL } from "../api";
import { scheduleFieldError } from "../scheduleValidate";
import { useStore, type NewSessionProject } from "../store";
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
import { isScratchWorkspace, projectLabel, projectSubtitles } from "../viewModels";

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
  | {
      variant: "home";
      onError: (m: string) => void;
      onProjectChange?: (label: string | null) => void;
      projectSeed?: NewSessionProject;
    }
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

// Last project used from the landing composer (RH-1). Codex opens New session with
// your previous repo already selected, so a session can go out in one keystroke —
// and, crucially, so the greeting headline and the project chip can never name
// different places. localStorage (not "the newest session") is the durable
// source: a session list can reorder under you (background runs, scratch
// sessions), while an explicit choice is what the user actually meant. Absent
// key = never chosen here (we seed from history once, below); "" = the user
// explicitly picked "Don't work in a project" and we must not re-seed over it.
const PROJECT_KEY = "arwebui.lastProject";
// How many projects the picker lists before you type (HM-9). Only the *view* is
// capped; the search runs over every workspace in history.
const RECENT_PROJECTS = 5;
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
  // EVERY workspace the history knows, newest first, deduped — picking an
  // existing project must be one click, not a hand-typed absolute path (W14).
  //
  // HM-9: this list used to stop at 5, and the picker's "Search projects" box
  // filtered *that* truncated list. With 202 workspaces in the store the box was
  // therefore a lie: it looked like a global project search, but typing the name
  // of a project sitting *visibly in the sidebar of the same frame* answered
  // "No projects found", and the user's own main repo was unreachable except by
  // typing an absolute path. The cap belongs to the default *view* (below), not
  // to the searchable set.
  const allWorkspaces = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const s of [...allSessions].sort((a, b) => b.id.localeCompare(a.id))) {
      const w = (s.workspace || "").trim().replace(/\/+$/, "");
      if (!w || seen.has(w)) continue;
      seen.add(w);
      out.push(w);
    }
    return out;
  }, [allSessions]);
  const isSession = props.variant === "session";

  // Per-session draft: initialize from what was typed here last time (the
  // component remounts on session switch), keep it saved as you type.
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
  // The compact root mirrors Codex's Model / Effort / Advanced summary. Each
  // dimension swaps to its own page, so short phones never have to scroll past
  // the full model list just to reach effort or advanced settings.
  const [modelMenuPage, setModelMenuPage] = useState<"root" | "model" | "effort" | "speed" | "advanced">("root");
  // The `+` menu is a small drawer, not a settings panel (INC-41 CP-1). Its root
  // page stays ≤7 single-line rows; the five agent personas (and the raw YAML
  // editor, which is the same subject) live one level down, reusing the model
  // menu's page-swap pattern.
  const [addMenuPage, setAddMenuPage] = useState<"root" | "advanced" | "agent">("root");
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

  // home-only context. `ws` is the ONE source of truth for "where does this session
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
  const [branchInfo, setBranchInfo] = useState<{ isRepo: boolean; current: string; branches: string[]; dirty: number; hasCommits?: boolean } | null>(null);

  // goal / loop / best-of-N launcher panel
  const [launcher, setLauncher] = useState<null | { mode: "goal" | "loop" | "best"; prompt: string }>(null);

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
  const anyRef = useRef<HTMLInputElement>(null);
  const seeded = useRef(false);

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
  // A project-row New chat shortcut changes the Home composer's project in
  // place. Do not remount: a user already on Home may have a draft, attachments,
  // model or access choice that must survive the navigation intent.
  const projectSeed = !isSession ? (props as Extract<ComposerProps, { variant: "home" }>).projectSeed : undefined;
  useEffect(() => {
    const workspace = projectSeed?.workspace.trim();
    const requestId = projectSeed?.requestId;
    if (isSession || !workspace || requestId === undefined) return;
    seeded.current = true;
    setWs(workspace);
    rememberProject(workspace);
    setProjectQuery("");
    setProjectMenuPage("projects");
    requestAnimationFrame(() => taRef.current?.focus());
    useStore.getState().consumeNewSessionProject(requestId);
  }, [isSession, projectSeed?.requestId]);

  useEffect(() => {
    if (isSession || seeded.current || recallProject() !== null) return;
    const candidate = allWorkspaces.find((w) => {
      const label = projectLabel(w);
      // Scratch dirs never seed the composer (their label is per-workspace
      // since INC-78, so test the judgement, not the old aggregate string).
      return !isScratchWorkspace(w) && label !== "Other sessions";
    });
    if (!candidate) return; // sessions may still be loading — try again when they land
    seeded.current = true;
    setWs(candidate);
  }, [isSession, allWorkspaces]);

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
        // Non-repo OR a repo with no commits yet (unborn branch) can't host a
        // worktree — fall back to Local so the user never hits git's raw
        // "invalid starting ref" error (phone report 2026-07-12).
        if (!b.isRepo || b.hasCommits === false) setRunLocation("local");
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

  // Home lands ready to type (INC-41 HM-1). Codex opens New session with the caret
  // already in the input; ours left document.activeElement === BODY, so a
  // blind-typed "hello" fell on the floor. Focus on mount — but never rip focus
  // out of an overlay that owns the window (command palette, settings, a modal)
  // or out of a field someone is already typing in. Session composers keep the
  // old behavior: opening a session focuses the transcript, not the input.
  useEffect(() => {
    if (isSession) return;
    if (document.querySelector("[role='dialog']")) return;
    const active = document.activeElement as HTMLElement | null;
    if (active && (active.tagName === "INPUT" || active.tagName === "TEXTAREA" || active.isContentEditable)) return;
    taRef.current?.focus();
  }, [isSession]);

  // Same-basename projects ("ws", "Scratch") get a short disambiguating
  // subtitle in the picker; uniquely-named ones stay clean (W4). Computed over
  // the whole searchable set, so two same-named hits in a search result can be
  // told apart — the picker prints a bold basename plus that gray parent-path
  // hint, never one long smear of an absolute path.
  const projectSubs = useMemo(() => projectSubtitles(allWorkspaces), [allWorkspaces]);

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
    // No worktree when it's not a repo, or the repo has no commits (unborn
    // branch): git worktree needs a real starting commit. Run local instead.
    if (runLocation !== "worktree" || !branchInfo?.isRepo || branchInfo.hasCommits === false) return source;
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
        const r = await AR.startRun({ kind: "submit", spec, extraSpecs: [], prompt: t, workspace, mode: accessById(access).mode, idem: "" });
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
  const startGoal = async (goalText: string, verifier: string, iterations: number) => {
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
          message: goalText,
          mode: accessById(access).mode,
        });
        rememberSpec(r.sid, spec);
        rememberAccess(r.sid, access);
        sid = r.sid;
      }
      try {
        await AR.goal(sid, { action: "attach", goal: goalText, verifier, maxChecks: iterations });
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

  // A drive run's user-facing object is its series SESSION (INC-80.3):
  // land there as soon as the daemon assigns it; the transient run stream
  // is only the fallback while the id is still unknown.
  const landInSeries = async (runId: string) => {
    for (let i = 0; i < 10; i++) {
      try {
        const rs = await AR.runs();
        const sid = rs.find((x) => x.id === runId)?.sessionId;
        if (sid) {
          await refreshSessions();
          select(sid);
          return;
        }
      } catch {
        /* transient — keep polling */
      }
      await new Promise((res) => setTimeout(res, 300));
    }
    selectRun(runId);
  };

  const startLoop = async (prompt: string, interval: string, iterations: number) => {
    const workspace = isSession ? (props as any).workspace || (await ensureWs()) : await ensureWs();
    if (!workspace) return props.onError("a workspace is required to start a loop");
    setBusy(true);
    try {
      const r = await AR.startRun({
        kind: "drive",
        spec: buildLoopDriver({ prompt, interval, maxIterations: iterations, provider, model }),
        extraSpecs: [{ name: "agent.yaml", content: buildDriverAgent({ provider, model }) }],
        prompt: "",
        workspace,
        mode: "",
        idem: "",
      });
      setLauncher(null);
      resetInput();
      await refreshRuns();
      await landInSeries(r.runId);
    } catch (e: any) {
      props.onError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // Best-of-N (schedule: parallel): N isolated attempts from one snapshot,
  // the verifier judges, the best result wins.
  const startBest = async (prompt: string, verifier: string, attempts: number) => {
    const workspace = isSession ? (props as any).workspace || (await ensureWs()) : await ensureWs();
    if (!workspace) return props.onError("a workspace is required to start a best-of-N run");
    setBusy(true);
    try {
      const r = await AR.startRun({
        kind: "drive",
        spec: buildBestOfNDriver({ prompt, n: attempts, verifier, provider, model }),
        extraSpecs: [{ name: "agent.yaml", content: buildDriverAgent({ provider, model }) }],
        prompt: "",
        workspace,
        mode: "",
        idem: "",
      });
      setLauncher(null);
      resetInput();
      await refreshRuns();
      await landInSeries(r.runId);
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
        setLauncher({ mode: "goal", prompt: rest });
        setText("");
        return;
      case "loop":
        setLauncher({ mode: "loop", prompt: rest });
        setText("");
        return;
      case "bestof":
        setLauncher({ mode: "best", prompt: rest });
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
        toast("Plan mode — the next session runs read-only", "info");
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
        const aliases: Record<string, EffortId> = { off: "light", none: "light", low: "light", light: "light", medium: "medium", med: "medium", high: "high", xhigh: "xhigh", extrahigh: "xhigh", max: "xhigh" };
        const eff = aliases[q];
        setText("");
        if (eff) await chooseEffort(eff);
        else toast(`Unknown effort "${rest}". Try: ${EFFORT_LEVELS.map((e) => e.id).join(", ")}`, "info");
        return;
      }
      case "compact":
        setText("");
        try {
          await AR.compact(sid, rest);
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

  // On short phones the focused input can be visible while the action row is
  // just below the viewport. Keep Codex's single composer card intact and
  // reveal its bottom edge only when it is actually clipped.
  const revealMobileActions = () => {
    if (isSession || !narrow) return;
    requestAnimationFrame(() => {
      const card = taRef.current?.closest<HTMLElement>(".cx-card");
      const viewportHeight = window.visualViewport?.height ?? window.innerHeight;
      if (card && card.getBoundingClientRect().bottom > viewportHeight) {
        card.scrollIntoView({ block: "end", inline: "nearest" });
      }
    });
  };

  // Match Codex's quiet primary prompts exactly. Slash commands remain
  // discoverable by typing `/`; the placeholder should describe the user's
  // job, not advertise implementation mechanics.
  const placeholder = isSession
    ? "Ask for follow-up changes"
    : kind === "chat"
      ? "Do anything"
      : narrow
        ? "Describe a background run"
        : "Describe a background run";

  // Pill label: friendly name for a chosen workspace; before one exists, say
  // what will actually happen instead of the ambiguous "auto-created" (W2).
  const wsShort = ws ? projectLabel(ws) : "New scratch workspace";
  const normalizedWs = ws.trim().replace(/\/+$/, "");
  // The picker's two states (HM-9, Codex parity):
  //   · idle  — the few most recent projects, so opening it doesn't dump 202 rows;
  //   · typing — a filter over EVERY known workspace, so anything the sidebar can
  //     show, the search box can find. "No projects found" now only ever means it.
  // The current selection is pinned into the idle view even when it has aged out
  // of the recent window; otherwise reopening the picker shows no checked row and
  // reads as "nothing selected" while the chip says otherwise.
  //
  // Ranking matters as much as reach: a *path* substring match is a real match
  // (a worktree under ~/dev2/agentrunner belongs to that repo) but a weak one —
  // searching "agentrunner" on the live store hits 107 workspaces, and without
  // ranking the repo you actually meant sits ~40 rows down among its own
  // scratch worktrees. So: name-prefix hits, then name-substring hits, then
  // path-only hits; recency (the natural order of `allWorkspaces`) breaks ties.
  const filteredProjects = useMemo(() => {
    const query = projectQuery.trim().toLowerCase();
    if (query) {
      const rank = (workspace: string): number => {
        const label = projectLabel(workspace).toLowerCase();
        if (label === query) return 0;
        if (label.startsWith(query)) return 1;
        if (label.includes(query)) return 2;
        return workspace.toLowerCase().includes(query) ? 3 : -1;
      };
      return allWorkspaces
        .map((workspace) => ({ workspace, rank: rank(workspace) }))
        .filter((hit) => hit.rank >= 0)
        .sort((a, b) => a.rank - b.rank) // Array#sort is stable → recency survives inside a rank
        .map((hit) => hit.workspace);
    }
    const recent = allWorkspaces.slice(0, RECENT_PROJECTS);
    if (normalizedWs && !recent.includes(normalizedWs)) return [normalizedWs, ...recent.slice(0, RECENT_PROJECTS - 1)];
    return recent;
  }, [allWorkspaces, projectQuery, normalizedWs]);
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
          initialPrompt={launcher.prompt}
          busy={busy}
          onCancel={() => setLauncher(null)}
          onStart={(prompt, a, b) =>
            launcher.mode === "goal" ? startGoal(prompt, a, b) : launcher.mode === "loop" ? startLoop(prompt, a, b) : startBest(prompt, a, b)
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
        {/* Project, run-location and branch chips. (A Codex-style environment
            chip was a disabled placeholder with no backend and was removed —
            re-add one only when an environment concept actually exists.) */}
        {!isSession && (
          <div className="cx-env-strip">
          <Popover
            align="left"
            wrapClass="cx-env-project-wrap"
            trigger={(open, toggle) => (
              <button className={"cx-env-control project" + (open ? " active" : "")} onClick={toggle} title="Select project" aria-haspopup="menu" aria-expanded={open}>
                <FolderIcon size={17} />
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

          {/* Start-in chip (INC-41 CP-4): ONE meaning — where the session runs.
              Run type (interactive vs background) used to ride along in this
              popover, which both broke the one-choice-per-menu rule above and
              left the chip lying: picking Background changed nothing here, so a
              headless run looked like a chat session. Session type now lives in the
              `+` menu's Advanced options, and the chip names the background state. */}
          <Popover
            align="left"
            trigger={(open, toggle) => (
              <button className={"cx-env-control" + (open ? " active" : "")} onClick={toggle} title={kind === "background" ? "Runs in the background — choose where it runs" : "Choose where this session runs"} aria-haspopup="menu" aria-expanded={open}>
                {kind === "background" ? <Lightning size={17} /> : runLocation === "local" ? <Desktop size={17} /> : <GitFork size={17} />}
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">
                  {(kind === "background" ? "Background · " : "") + (runLocation === "local" ? "Local" : "New worktree")}
                </span>
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu">
                <PopSection label="Start in">
                  <PopItem icon={<GitBranch size={15} />} title="New worktree" desc={!branchInfo?.isRepo ? "Select a Git project first" : branchInfo.hasCommits === false ? "Repo has no commits yet — commit one first" : "Isolated checkout; your project stays untouched"} active={runLocation === "worktree"} onClick={() => {
                    if (!branchInfo?.isRepo) { props.onError("New worktree needs a Git project."); return; }
                    if (branchInfo.hasCommits === false) { props.onError("This repo has no commits yet — a worktree needs a starting commit."); return; }
                    setRunLocation("worktree"); close();
                  }} />
                  <PopItem icon={<Desktop size={15} />} title="Local" desc="Work directly in the selected project" active={runLocation === "local"} onClick={() => { setRunLocation("local"); close(); }} />
                </PopSection>
              </div>
            )}
          </Popover>

          <Popover
            align="left"
            wrapClass={narrow ? "min-w-0 flex-1" : ""}
            panelClass="cx-branch-popover"
            onOpen={() => {
              setBranchQuery("");
              if (ws.trim()) AR.gitBranches(ws.trim()).then((info) => {
                setBranchInfo(info);
                if (!startingBranch) setStartingBranch((info.current === "HEAD" ? "" : info.current) || info.branches[0] || "");
              }).catch(() => {});
            }}
            trigger={(open, toggle) => (
              <button className={"cx-env-control branch" + (narrow ? " w-full" : "") + (open ? " active" : "")} onClick={toggle} title={branchInfo?.isRepo ? "Choose starting branch" : "No Git branch available"} disabled={!branchInfo?.isRepo} aria-haspopup="menu" aria-expanded={open}>
                <BranchIcon size={17} />
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis [direction:rtl] text-left">{branchLabel}</span>
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
            onFocus={revealMobileActions}
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
          {/* Codex-style `+`: root stays about adding context or switching the
              immediate mode. AgentRunner-specific automation remains available,
              but one level down so the first screen matches Codex's Add menu. */}
          <Popover
            align="left"
            panelClass="cx-pop-codex"
            onOpen={() => setAddMenuPage("root")}
            trigger={(open, toggle) => (
              <button type="button" className={"cx-icon" + (open ? " active" : "")} onClick={toggle} title="Add & advanced options" aria-label="Add and advanced options" aria-haspopup="menu" aria-expanded={open}>
                <PlusIcon />
              </button>
            )}
          >
            {(close) => (
              <div
                className={
                  "cx-menu cx-add-menu [&_.pop-body]:flex-row [&_.pop-body]:items-baseline [&_.pop-body]:gap-2 " +
                  "[&_.pop-title]:shrink-0 [&_.pop-desc]:min-w-0 [&_.pop-desc]:truncate" +
                  (addMenuPage === "agent" ? " cx-add-agent" : "")
                }
                style={{ width: 320, maxWidth: "calc(100vw - 32px)" }}
                onClick={(event) => event.preventDefault()}
              >
                {addMenuPage === "root" ? (
                  <>
                    <PopSection label="Add">
                      <PopItem icon={<Paperclip size={16} />} title="Files and folders" onClick={() => { close(); anyRef.current?.click(); }} />
                      <PopItem icon={<GoalIcon />} title="Goal" desc="Set a goal to keep pursuing" onClick={() => { close(); setLauncher({ mode: "goal", prompt: text.trim() }); }} />
                      <PopItem
                        icon={<PlanIcon />}
                        title="Plan mode"
                        desc="Turn plan mode on"
                        active={!isSession && access === "plan"}
                        disabled={isSession}
                        onClick={!isSession ? () => { close(); setAccess("plan"); } : undefined}
                      />
                    </PopSection>
                    <PopSection label="Advanced">
                      <PopItem
                        icon={<Lightning size={16} />}
                        title="Automation"
                        desc={kind === "background" ? "Background run" : personaById(persona).label}
                        right={<span aria-hidden>›</span>}
                        onClick={() => setAddMenuPage("advanced")}
                      />
                    </PopSection>
                  </>
                ) : addMenuPage === "advanced" ? (
                  <>
                    <div className="pop-menu-title">
                      <button className="pop-back" onClick={() => setAddMenuPage("root")} aria-label="Back to add menu">‹</button>
                      <b>Automation</b>
                    </div>
                    <PopItem icon={<LoopIcon />} title="Loop" desc="Repeat on a cadence" onClick={() => { close(); setLauncher({ mode: "loop", prompt: text.trim() }); }} />
                    <PopItem icon={<BestIcon />} title="Best of N" desc="Keep the best of N tries" onClick={() => { close(); setLauncher({ mode: "best", prompt: text.trim() }); }} />
                    {!isSession && (
                      <PopItem
                        icon={<Lightning size={16} />}
                        title="Background run"
                        desc="Run headless, no chat"
                        active={kind === "background"}
                        onClick={() => { setKind(kind === "background" ? "chat" : "background"); close(); }}
                      />
                    )}
                    <PopItem
                      icon={<PersonaIcon />}
                      title="Agent"
                      desc={personaById(persona).label}
                      right={<span aria-hidden>›</span>}
                      onClick={() => setAddMenuPage("agent")}
                    />
                  </>
                ) : (
                  <>
                    <div className="pop-menu-title">
                      <button className="pop-back" onClick={() => setAddMenuPage("advanced")} aria-label="Back to automation menu">‹</button>
                      <b>Agent</b>
                    </div>
                    {PERSONAS.map((item) => (
                      <PopItem key={item.id} icon={<PersonaIcon />} title={item.label} desc={item.desc} active={persona === item.id} onClick={() => { choosePersona(item.id); close(); }} />
                    ))}
                    <PopSection>
                      <PopItem icon={<Code size={16} />} title="Edit agent spec (YAML)…" onClick={() => { close(); openModal(isSession ? { kind: "agent", sid: (props as any).sid } : { kind: "new", message: text }); }} />
                    </PopSection>
                  </>
                )}
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
                  <div className="cx-pop-note">Approvals still surface here whenever a gate asks. Full access and Plan are fixed once the session starts.</div>
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
                  <div className="cx-pop-note">Approvals still surface here whenever a gate asks; the posture is fixed once the session starts.</div>
                </div>
              )}
            </Popover>
          )}

          <span className="cx-spacer" />

          {/* model pill — shows "<model> <effort>" like Codex's "5.6 Sol Extra High"
              (effort "Off" stays unwritten: the default posture is not news). */}
          <Popover
            align="right"
            panelClass="cx-pop-codex"
            onOpen={() => setModelMenuPage("root")}
            trigger={(open, toggle) => (
              <button type="button" className={"cx-pill cx-model" + (open ? " active" : "")} onClick={toggle} title="Model & effort" aria-haspopup="menu" aria-expanded={open}>
                <span className="cx-model-name">{modelLabel}</span>
                <span className="cx-pill-sub">{budgetOverride ? "Custom" : effortLevel.label}</span>
                <Caret />
              </button>
            )}
          >
            {(close) => (
              <div
                className="cx-menu wide cx-model-menu"
                style={{ width: 320, maxWidth: "calc(100vw - 32px)" }}
                onClick={(event) => event.preventDefault()}
              >
                {modelMenuPage === "root" ? (
                  <>
                    <div className="cx-model-roots">
                      <PopItem
                        title="Model"
                        right={<span className="inline-flex max-w-[210px] items-center gap-2"><span className="truncate">{modelLabel}</span><span aria-hidden>›</span></span>}
                        onClick={() => setModelMenuPage("model")}
                      />
                      <PopItem
                        title="Effort"
                        right={<span className="inline-flex max-w-[210px] items-center gap-2"><span className="truncate">{budgetOverride ? "Custom" : effortLevel.label}</span><span aria-hidden>›</span></span>}
                        onClick={() => setModelMenuPage("effort")}
                      />
                      <PopItem
                        title="Speed"
                        right={<span className="inline-flex max-w-[210px] items-center gap-2"><span className="truncate">Standard</span><span aria-hidden>›</span></span>}
                        onClick={() => setModelMenuPage("speed")}
                      />
                    </div>
                    <div className="cx-model-advanced">
                      <PopItem title={<span className="inline-flex items-center gap-1">Advanced <CaretDown size={14} className="cx-model-adv-chev open" aria-hidden="true" /></span>} onClick={() => setModelMenuPage("advanced")} />
                    </div>
                  </>
                ) : modelMenuPage === "model" ? (
                  <>
                    <div className="pop-menu-title">
                      <button type="button" className="pop-back" onClick={() => setModelMenuPage("root")} aria-label="Back to model menu">‹</button>
                      <b>Model</b>
                    </div>
                    <div className="cx-model-list">
                      {MODELS.map((m) => (
                        <PopItem
                          key={m.provider + m.id}
                          icon={<ModelIcon provider={m.provider} />}
                          title={m.label}
                          desc={m.sub}
                          active={provider === m.provider && model === m.id}
                          onClick={() => { chooseModel(m.provider, m.id); close(); }}
                        />
                      ))}
                    </div>
                  </>
                ) : modelMenuPage === "effort" ? (
                  <>
                    <div className="pop-menu-title">
                      <button type="button" className="pop-back" onClick={() => setModelMenuPage("root")} aria-label="Back to model menu">‹</button>
                      <b>Effort</b>
                    </div>
                    {EFFORT_LEVELS.map((level) => (
                      <PopItem
                        key={level.id}
                        title={level.label}
                        desc={level.desc}
                        active={budgetOverride == null && effort === level.id}
                        onClick={() => { chooseEffort(level.id); close(); }}
                      />
                    ))}
                  </>
                ) : modelMenuPage === "speed" ? (
                  <>
                    <div className="pop-menu-title">
                      <button type="button" className="pop-back" onClick={() => setModelMenuPage("root")} aria-label="Back to model menu">‹</button>
                      <b>Speed</b>
                    </div>
                    <PopItem title="Standard" desc="Balanced response speed" active onClick={close} />
                  </>
                ) : (
                  <>
                    <div className="pop-menu-title">
                      <button type="button" className="pop-back" onClick={() => setModelMenuPage("root")} aria-label="Back to model menu">‹</button>
                      <b>Advanced</b>
                    </div>
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
                <ListChecks size={14} />
                <span className="cx-deliv-label">Queue</span>
              </button>
              <button
                type="button"
                className={"cx-deliv" + (deliveryMode === "steer" ? " on" : "")}
                onClick={() => setDeliveryMode("steer")}
                title="Steer: fold into the current turn at its next safe boundary (⌘⏎ to queue this one)"
              >
                <Lightning size={14} />
                <span className="cx-deliv-label">Steer</span>
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

      {/* hidden file input — ONE picker for everything (CP-1). Multi-select
          (mirrors the drag-drop loop) and mime-routed: images ride --image,
          everything else --file, exactly like paste and drop. The old
          image-only input existed solely to back a separate "Images" row. */}
      <input type="file" multiple ref={anyRef} style={{ display: "none" }} onChange={(e) => { for (const f of Array.from(e.target.files || [])) pick(f, f.type.startsWith("image/")); e.target.value = ""; }} />
    </div>
  );
}

// ---- goal / loop / best-of-N launcher ---------------------------------------
function GoalLoopLauncher({
  mode,
  initialPrompt,
  busy,
  onCancel,
  onStart,
}: {
  mode: "goal" | "loop" | "best";
  initialPrompt: string;
  busy: boolean;
  onCancel: () => void;
  onStart: (prompt: string, second: string, iterations: number) => void;
}) {
  const [prompt, setPrompt] = useState(initialPrompt);
  const [second, setSecond] = useState(mode === "loop" ? "5m" : ""); // interval | verifier
  // Inline cadence validation (G36 余项): same mirror as the Schedule modal.
  const intervalError = mode === "loop" ? scheduleFieldError("interval", second) : "";
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
      <textarea className="cx-launcher-prompt" rows={2} placeholder={mode === "goal" ? "What goal should the agent keep working toward?" : mode === "loop" ? "What should each iteration do?" : "What should each attempt try to do?"} value={prompt} onChange={(e) => setPrompt(e.target.value)} />
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
        <button
          className="primary cx-launcher-go"
          disabled={busy || !prompt.trim() || (mode === "loop" && (!second.trim() || intervalError !== ""))}
          onClick={() => onStart(prompt.trim(), second.trim(), iters)}
        >
          {meta.start}
        </button>
      </div>
      {intervalError !== "" && (
        <div className="mt-1 text-[12px] leading-5 text-red" role="alert">
          {intervalError}
        </div>
      )}
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
const FolderIcon = ({ size = 13 }: { size?: number }) => <Folder size={size} />;
const BranchIcon = ({ size = 13 }: { size?: number }) => <GitBranch size={size} />;
const GoalIcon = () => <Target size={14} />;
const LoopIcon = () => <ArrowClockwise size={14} />;
const PlanIcon = () => <ListChecks size={14} />;
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

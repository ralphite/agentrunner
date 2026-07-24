import { useEffect, useMemo, useRef, useState } from "react";
import { sessionImageURL, type ForkDraft } from "../api";
import { useAppServices } from "../app/appServices";
import { useAppStoreApi, useStore, type NewSessionProject } from "../store";
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
  personaById,
  personaFromSpec,
  replaceModel,
  type AccessId,
  type EffortId,
} from "../specs";
import type { ComposerAttachment } from "./ComposerParts";
import { useVoice } from "./useVoice";
import { useDictation } from "./useDictation";
import { helperContext, runOptimize, undoOptimize } from "./composerOptimize";
import { parseSlash, SLASH, type SlashCmd } from "./slash";
import { recallAccess, recallDraft, recallSpec, rememberAccess, rememberDraft, rememberSpec } from "./sessionSpecs";
import { isScratchWorkspace, projectLabel, projectSubtitles } from "../viewModels";
import { ComposerView } from "../features/composer/ComposerView";

export {
  ComposerView,
  GoalLoopLauncher,
} from "../features/composer/ComposerView";

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
      onDraftChange?: (draft: string) => void;
      projectSeed?: NewSessionProject;
    }
  | {
      variant: "session";
      sid: string;
      workspace?: string;
      mode?: string; // the session's LIVE approval mode (SessionView lifts it from inspect; /mode switches it — INC-42)
      running?: boolean;
      seed?: ForkDraft | null;
      seedReleasedAt?: number;
      focusOnMount?: boolean;
      onSend: (text: string, images: string[], files: string[], delivery?: "steer" | "queue",
        draft?: { draftId: string; sendRequestId: string;
          parts: Array<{ kind: "image" | "file"; ref?: string; path?: string; ordinal?: number }>;
          replayOriginal: boolean }) => Promise<void>;
      actions?: SessionActions;
      onError: (m: string) => void;
    };

function forkSendRequestID(
  storage: Storage,
  createID: () => string,
  sid: string,
  draftID: string,
): string {
  const key = `arwebui.fork-send.${sid}.${draftID}`;
  try {
    const prior = storage.getItem(key);
    if (prior) return prior;
    const id = createID();
    storage.setItem(key, id);
    return id;
  } catch {
    return createID();
  }
}

function forgetForkSendRequest(storage: Storage, sid: string, draftID: string) {
  try { storage.removeItem(`arwebui.fork-send.${sid}.${draftID}`); } catch { /* ignore */ }
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
function recallProject(storage: Storage): string | null {
  try {
    return storage.getItem(PROJECT_KEY);
  } catch {
    return null;
  }
}
function rememberProject(storage: Storage, workspace: string) {
  try {
    storage.setItem(PROJECT_KEY, workspace);
  } catch {
    /* ignore quota */
  }
}

export function Composer(props: ComposerProps) {
  const { api, clock, ids, storage } = useAppServices();
  const store = useAppStoreApi();
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
  const [text, setText] = useState(() => recallDraft(draftKey, storage.session));
  useEffect(() => rememberDraft(draftKey, text, storage.session), [draftKey, text, storage.session]);
  const onHomeDraftChange = !isSession
    ? (props as Extract<ComposerProps, { variant: "home" }>).onDraftChange
    : undefined;
  useEffect(() => onHomeDraftChange?.(text), [onHomeDraftChange, text]);
  const [atts, setAtts] = useState<ComposerAttachment[]>([]);
  const forkSeeded = useRef<string | null>(null);
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
  const [modelMenuPage, setModelMenuPage] = useState<"root" | "model" | "effort" | "advanced">("root");
  // The `+` menu is a small drawer, not a settings panel (INC-41 CP-1). Its root
  // page stays ≤7 single-line rows; the five agent personas (and the raw YAML
  // editor, which is the same subject) live one level down, reusing the model
  // menu's page-swap pattern.
  const [addMenuPage, setAddMenuPage] = useState<"root" | "advanced" | "agent">("root");
  // The home composer remembers the last chosen access level (W15); session
  // composers show the session's fixed posture instead and never read this.
  const [access, setAccessState] = useState<AccessId>(() => {
    try {
      const saved = storage.local.getItem("arwebui.lastAccess") as AccessId | null;
      if (saved && ACCESS_LEVELS.some((a) => a.id === saved)) return saved;
    } catch {
      /* ignore */
    }
    return DEFAULT_ACCESS;
  });
  const lastNonPlanAccess = useRef<AccessId>(access === "plan" ? DEFAULT_ACCESS : access);
  const setAccess = (a: AccessId) => {
    if (a !== "plan") lastNonPlanAccess.current = a;
    setAccessState(a);
    try {
      storage.local.setItem("arwebui.lastAccess", a);
    } catch {
      /* ignore quota */
    }
  };
  const homeAccessTriggerRef = useRef<HTMLButtonElement>(null);
  const chooseHomeAccess = (next: AccessId, close: () => void) => {
    if (next === "full" && access !== "full") {
      close();
      openModal({
        kind: "confirm",
        title: "Turn on Full Access?",
        body: "The agent can act without asking, including:",
        details: [
          { icon: "files", title: "Files and folders", body: "Read, create, modify, upload, or delete files anywhere on this computer" },
          { icon: "terminal", title: "Terminal commands", body: "Run commands, install software, and change system settings" },
          { icon: "internet", title: "Internet access", body: "Access websites and send data to enabled services" },
        ],
        note: "This increases the risk of sensitive-data exposure and prompt injection. You can turn Full Access off at any time.",
        confirmLabel: "Turn on Full Access",
        danger: true,
        onConfirm: () => setAccess("full"),
        onClose: () => homeAccessTriggerRef.current?.focus(),
      });
      return;
    }
    setAccess(next);
    close();
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
  const [ws, setWs] = useState(() => recallProject(storage.local) || "");
  const [kind, setKind] = useState<"chat" | "background">("chat");
  const [runLocation, setRunLocation] = useState<"worktree" | "local">("worktree");
  const [startingBranch, setStartingBranch] = useState("");
  const [projectQuery, setProjectQuery] = useState("");
  const [branchQuery, setBranchQuery] = useState("");
  const [projectMenuPage, setProjectMenuPage] = useState<"projects" | "new">("projects");
  const [branchInfo, setBranchInfo] = useState<{ isRepo: boolean; current: string; branches: string[]; dirty: number; hasCommits?: boolean } | null>(null);

  // goal / loop / best-of-N launcher panel
  const [launcher, setLauncher] = useState<null | { mode: "goal" | "loop" | "best"; prompt: string }>(null);
  const [goalVerifier, setGoalVerifier] = useState("");
  const [goalRounds, setGoalRounds] = useState(10);
  const goalMode = launcher?.mode === "goal";
  const togglePlanMode = () => setAccess(access === "plan" ? lastNonPlanAccess.current : "plan");

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

  // A message-fork seed comes from the child journal. Apply it once per
  // draft id so polling never overwrites the user's edits, and focus only
  // after the child composer is mounted.
  const forkSeed = isSession ? (props as Extract<ComposerProps, { variant: "session" }>).seed : null;
  const forkSeedReleasedAt = isSession ? (props as Extract<ComposerProps, { variant: "session" }>).seedReleasedAt : 0;
  const handledSeedRelease = useRef(0);
  useEffect(() => {
	if (forkSeed && forkSeedReleasedAt && handledSeedRelease.current !== forkSeedReleasedAt) {
		handledSeedRelease.current = forkSeedReleasedAt;
		forkSeeded.current = "";
		forgetForkSendRequest(storage.session, (props as Extract<ComposerProps, { variant: "session" }>).sid, forkSeed.draft_id);
	}
    if (!isSession || !forkSeed || forkSeeded.current === forkSeed.draft_id) return;
    forkSeeded.current = forkSeed.draft_id;
    setText(forkSeed.text || "");
    const sid = (props as Extract<ComposerProps, { variant: "session" }>).sid;
    const ordered = (forkSeed.content || []).filter((part) =>
      (part.kind === "image" || part.kind === "file") && part.ref).map((part, ordinal) => ({ ...part, draftOrdinal: ordinal }));
    const legacy = ordered.length > 0 ? ordered : [
      ...(forkSeed.images || []).map((part) => ({ ...part, kind: "image" as const })),
      ...(forkSeed.files || []).map((part) => ({ ...part, kind: "file" as const })),
    ].map((part, ordinal) => ({ ...part, draftOrdinal: ordinal }));
    setAtts(legacy.map((a) => ({
      path: a.kind === "image" ? sessionImageURL(sid, a.ref!) : "", ref: a.ref,
      name: a.name || `${a.kind}-${a.ref!.slice(-8)}`, isImage: a.kind === "image", partId: a.part_id,
      draftOrdinal: a.draftOrdinal,
    })));
    requestAnimationFrame(() => {
      taRef.current?.focus();
      if (taRef.current) grow(taRef.current);
    });
  }, [isSession, forkSeed, forkSeedReleasedAt, props]);
  useEffect(() => {
	if (!isSession || forkSeed || !forkSeeded.current) return;
	forgetForkSendRequest(storage.session, (props as Extract<ComposerProps, { variant: "session" }>).sid, forkSeeded.current);
	forkSeeded.current = "";
  }, [isSession, forkSeed, props]);

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
          optimize: (d, c) => api.optimize(d, c),
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
    const sp = recallSpec((props as any).sid, storage.local);
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
    rememberProject(storage.local, workspace);
    setProjectQuery("");
    setProjectMenuPage("projects");
    requestAnimationFrame(() => taRef.current?.focus());
    store.getState().consumeNewSessionProject(requestId);
  }, [isSession, projectSeed?.requestId]);

  useEffect(() => {
    if (isSession || seeded.current || recallProject(storage.local) !== null) return;
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
    api.gitBranches(dir)
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

  useEffect(() => {
    if (!isSession || !(props as Extract<ComposerProps, { variant: "session" }>).focusOnMount) return;
    requestAnimationFrame(() => taRef.current?.focus());
  }, [isSession, isSession ? (props as Extract<ComposerProps, { variant: "session" }>).sid : ""]);

  // Same-basename projects ("ws", "Scratch") get a short disambiguating
  // subtitle in the picker; uniquely-named ones stay clean (W4). Computed over
  // the whole searchable set, so two same-named hits in a search result can be
  // told apart — the picker prints a bold basename plus that gray parent-path
  // hint, never one long smear of an absolute path.
  const projectSubs = useMemo(() => projectSubtitles(allWorkspaces), [allWorkspaces]);

  const modelLabel = modelById(provider, model)?.label || model;
  const effortLevel = effortById(effort);
  const accessLevel = isSession ? undefined : accessById(access);
  const remembered = isSession ? recallAccess((props as any).sid, storage.local) : undefined;
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
    const t = clock.setTimeout(() => {
      api.files((props as any).sid, q)
        .then((r) => {
          if (seq !== atSeq.current) return;
          setAtKnown(r.known);
          setAtFiles(r.files);
        })
        .catch(() => seq === atSeq.current && setAtFiles([]));
    }, 120);
    return () => clock.clearTimeout(t);
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
    const p = (await api.makeWorkspace()).path;
    setWs(p);
    return p;
  };

  const resolveHomeWorkspace = async (): Promise<string> => {
    const source = await ensureWs();
    // No worktree when it's not a repo, or the repo has no commits (unborn
    // branch): git worktree needs a real starting commit. Run local instead.
    if (runLocation !== "worktree" || !branchInfo?.isRepo || branchInfo.hasCommits === false) return source;
    return (await api.makeWorktree(source, "", startingBranch || branchInfo.current)).path;
  };

  const resetInput = () => {
    // Clear synchronously before Home navigation can unmount this Composer;
    // relying only on the state effect can leave the just-sent draft in
    // sessionStorage and resurrect it on reload.
    rememberDraft(draftKey, "", storage.session);
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
      const r = await api.upload(file);
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
    if ((!t && atts.length === 0) || busy) return;

    // Slash command? Run it instead of sending.
    const cmd = parseSlash(t, props.variant);
    if (cmd) {
      await runSlash(cmd.cmd, cmd.rest);
      return;
    }

    if (goalMode) {
      await startGoal(t, goalVerifier.trim(), goalRounds);
      return;
    }

    setBusy(true);
    try {
      if (isSession) {
        const imgs = atts.filter((a) => a.isImage && !a.ref).map((a) => a.path);
        const files = atts.filter((a) => !a.isImage && !a.ref).map((a) => a.path);
        // Delivery mode only matters while a turn is running; at idle a send just
        // starts the next turn either way, so leave it undefined then.
        const effective: "steer" | "queue" | undefined = running
          ? (opposite ? (deliveryMode === "queue" ? "steer" : "queue") : deliveryMode)
          : undefined;
        const sessionID = (props as Extract<ComposerProps, { variant: "session" }>).sid;
        const draftParts: Array<{
          kind: "image" | "file";
          ref?: string;
          path?: string;
          ordinal?: number;
        }> = atts.map((a) => ({
          kind: a.isImage ? "image" : "file",
          ...(a.ref ? { ref: a.ref, ordinal: a.draftOrdinal } : { path: a.path }),
        }));
        const originalParts = forkSeed
          ? (forkSeed.content || [])
              .filter((part) => (part.kind === "image" || part.kind === "file") && part.ref)
              .map((part, ordinal) => ({
                kind: part.kind as "image" | "file",
                ref: part.ref!,
                ordinal,
              }))
          : [];
        const legacyOriginal = forkSeed && originalParts.length === 0
          ? [
              ...(forkSeed.images || []).map((part) => ({ kind: "image" as const, ref: part.ref })),
              ...(forkSeed.files || []).map((part) => ({ kind: "file" as const, ref: part.ref })),
            ].map((part, ordinal) => ({ ...part, ordinal }))
          : originalParts;
        const replayOriginal = !!forkSeed && text === (forkSeed.text || "") &&
          draftParts.length === legacyOriginal.length && draftParts.every((part, i) =>
            !!part.ref && part.kind === legacyOriginal[i].kind && part.ref === legacyOriginal[i].ref &&
            part.ordinal === legacyOriginal[i].ordinal);
        const draftSend = forkSeed ? {
          draftId: forkSeed.draft_id,
          sendRequestId: forkSendRequestID(
            storage.session,
            () => ids.uuid("send"),
            sessionID,
            forkSeed.draft_id,
          ),
          parts: draftParts,
          replayOriginal,
        } : undefined;
        await (props as Extract<ComposerProps, { variant: "session" }>).onSend(
          forkSeed ? text : t,
          imgs,
          files,
          effective,
          draftSend,
        );
        resetInput();
      } else if (kind === "chat") {
        const workspace = await resolveHomeWorkspace();
        const spec = buildSpec({ provider, model, access, persona, effort, budgetOverride });
        const imgs = atts.filter((a) => a.isImage).map((a) => a.path);
        const files = atts.filter((a) => !a.isImage).map((a) => a.path);
        const r = await api.newSession({
          spec,
          extraSpecs: personaById(persona).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : [],
          workspace,
          message: t,
          mode: accessById(access).mode,
        });
        rememberSpec(r.sid, spec, storage.local);
        rememberAccess(r.sid, access, storage.local);
        resetInput();
        // The create response is already the durable navigation fact. Route to
        // it before refreshing the sidebar so a transient list failure cannot
        // strand the user on Home after their draft has been consumed.
        select(r.sid);
        await refreshSessions();
        // The opening message (`ar new`) can't carry attachments (DESIGN §9.1);
        // deliver a first-message attachment on an immediate follow-up so it
        // still works from the landing composer.
        if (imgs.length || files.length) {
          const n = imgs.length + files.length;
          try {
            await api.send(r.sid, `(see attached file${n > 1 ? "s" : ""})`, imgs, files);
          } catch (e: any) {
            props.onError(e.message);
          }
        }
      } else {
        const workspace = await resolveHomeWorkspace();
        const spec = buildSpec({ provider, model, access, persona, effort, budgetOverride });
        const r = await api.startRun({ kind: "submit", spec, extraSpecs: [], prompt: t, workspace, mode: accessById(access).mode, idem: "" });
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
      const base = recallSpec(sid, storage.local) || buildSpec({ provider: p, model: id, access: "full", persona, effort: eff, budgetOverride: budget });
      const spec = replaceModel(base, p, id, eff, budget);
      await api.switchAgent(sid, spec, personaById(persona).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : []);
      rememberSpec(sid, spec, storage.local);
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
      const acc = (recallAccess(sid, storage.local) as AccessId) || "full";
      const spec = buildSpec({ provider, model, access: acc, persona: id, effort, budgetOverride });
      const sib = personaById(id).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : [];
      await api.switchAgent(sid, spec, sib);
      rememberSpec(sid, spec, storage.local);
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
        const r = await api.newSession({
          spec,
          extraSpecs: personaById(persona).withWorker ? [{ name: "worker.yaml", content: DEFAULT_WORKER }] : [],
          workspace,
          message: goalText,
          mode: accessById(access).mode,
        });
        rememberSpec(r.sid, spec, storage.local);
        rememberAccess(r.sid, access, storage.local);
        sid = r.sid;
      }
      try {
        await api.goal(sid, { action: "attach", goal: goalText, verifier, maxChecks: iterations });
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
        const rs = await api.runs();
        const sid = rs.find((x) => x.id === runId)?.sessionId;
        if (sid) {
          await refreshSessions();
          select(sid);
          return;
        }
      } catch {
        /* transient — keep polling */
      }
      await new Promise<void>((resolve) => clock.setTimeout(() => resolve(), 300));
    }
    selectRun(runId);
  };

  const startLoop = async (prompt: string, interval: string, iterations: number) => {
    const workspace = isSession ? (props as any).workspace || (await ensureWs()) : await ensureWs();
    if (!workspace) return props.onError("a workspace is required to start a loop");
    setBusy(true);
    try {
      const r = await api.startRun({
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
      const r = await api.startRun({
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
      await api.mode(sid, target);
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
          await api.compact(sid, rest);
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
          await api.clear(sid);
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
      if (running && (text.trim() || atts.length > 0)) {
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
    el.style.height = Math.min(el.scrollHeight, 320) + "px";
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
    : goalMode
      ? "Describe your goal, define measurable outcomes for best results"
      : access === "plan"
        ? "Describe what to plan…"
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
    rememberProject(storage.local, workspace);
    seeded.current = true;
    setProjectQuery("");
    setProjectMenuPage("projects");
    api.gitBranches(workspace)
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
    <ComposerView
      isSession={isSession}
      launcher={
        launcher
          ? {
              mode: launcher.mode,
              initialPrompt: launcher.prompt,
              busy,
              onCancel: () => setLauncher(null),
              onStart: (prompt, second, iterations) =>
                launcher.mode === "goal"
                  ? startGoal(prompt, second, iterations)
                  : launcher.mode === "loop"
                    ? startLoop(prompt, second, iterations)
                    : startBest(prompt, second, iterations),
            }
          : undefined
      }
      dragging={dragging}
      cardEvents={{ onDragEnter, onDragOver, onDragLeave, onDrop }}
      environment={
        !isSession
          ? {
              projectPicker: {
                label: ws ? wsShort : "Select project",
                query: projectQuery,
                page: projectMenuPage,
                selected: !!ws,
                projects: filteredProjects.map((workspace) => ({
                  workspace,
                  label: projectLabel(workspace),
                  subtitle: projectSubs.get(workspace),
                  active: workspace === normalizedWs,
                })),
                onOpen: () => {
                  setProjectQuery("");
                  setProjectMenuPage("projects");
                },
                onQueryChange: setProjectQuery,
                onSelect: chooseProject,
                onShowNew: () => setProjectMenuPage("new"),
                onBack: () => setProjectMenuPage("projects"),
                onStartScratch: async () => {
                  try {
                    chooseProject((await api.makeWorkspace()).path);
                  } catch (error: any) {
                    props.onError(error.message);
                  }
                },
                onUseExisting: () =>
                  openPrompt({
                    title: "Add project",
                    label: "absolute folder path",
                    initial: ws,
                    placeholder: "/path/to/project",
                    onSubmit: chooseProject,
                  }),
                onClear: () => {
                  setWs("");
                  rememberProject(storage.local, "");
                  seeded.current = true;
                  setBranchInfo(null);
                  setStartingBranch("");
                  setRunLocation("local");
                },
              },
              ...(ws
                ? {
                    runLocationPicker: {
                      kind,
                      location: runLocation,
                      worktreeUnavailableReason: !branchInfo?.isRepo
                        ? "Select a Git project first"
                        : branchInfo.hasCommits === false
                          ? "Repo has no commits yet — commit one first"
                          : undefined,
                      onSelect: setRunLocation,
                      onUnavailableWorktree: () => {
                        props.onError(
                          !branchInfo?.isRepo
                            ? "New worktree needs a Git project."
                            : "This repo has no commits yet — a worktree needs a starting commit.",
                        );
                      },
                    },
                    branchPicker: {
                      label: branchLabel,
                      narrow,
                      isRepo: !!branchInfo?.isRepo,
                      location: runLocation,
                      dirty: branchInfo?.dirty,
                      query: branchQuery,
                      branches: filteredBranches,
                      totalBranches: branchInfo?.branches.length || 0,
                      onOpen: () => {
                        setBranchQuery("");
                        if (ws.trim()) {
                          api
                            .gitBranches(ws.trim())
                            .then((info) => {
                              setBranchInfo(info);
                              if (!startingBranch) {
                                setStartingBranch(
                                  (info.current === "HEAD"
                                    ? ""
                                    : info.current) ||
                                    info.branches[0] ||
                                    "",
                                );
                              }
                            })
                            .catch(() => {});
                        }
                      },
                      onQueryChange: setBranchQuery,
                      onSelect: async (branch, close) => {
                        if (runLocation === "worktree") {
                          setStartingBranch(branch);
                          close();
                          return;
                        }
                        try {
                          await api.gitCheckout(ws.trim(), branch, false);
                          setBranchInfo((current) =>
                            current
                              ? { ...current, current: branch }
                              : current,
                          );
                          setStartingBranch(branch);
                          toast(`Switched to ${branch}`, "info");
                          close();
                        } catch (error: any) {
                          props.onError(error.message);
                        }
                      },
                    },
                  }
                : {}),
            }
          : undefined
      }
      attachments={{
        attachments: atts,
        onRemove: (index) =>
          setAtts((current) =>
            current.filter((_, itemIndex) => itemIndex !== index),
          ),
      }}
      textarea={{
        ref: taRef,
        value: text,
        placeholder,
        onChange: (event) => {
          setText(event.target.value);
          grow(event.target);
        },
        onFocus: revealMobileActions,
        onKeyDown: onKey,
        onPaste,
      }}
      fileMentionMenu={
        atQuery !== null
          ? {
              query: atQuery,
              known: atKnown,
              files: atFiles,
              activeIndex: atIdx,
              onActiveIndexChange: setAtIdx,
              onSelect: applyAt,
            }
          : undefined
      }
      slashCommandMenu={
        slashOpen && filteredSlash.length > 0
          ? {
              commands: filteredSlash,
              activeIndex: slashIdx,
              onActiveIndexChange: setSlashIdx,
              onSelect: applySlash,
            }
          : undefined
      }
      addMenu={{
        page: addMenuPage,
        isSession,
        goalMode,
        planMode: access === "plan",
        kind,
        persona,
        onOpen: () => setAddMenuPage("root"),
        onPageChange: setAddMenuPage,
        onPickFiles: () => anyRef.current?.click(),
        onToggleGoal: () =>
          setLauncher(
            goalMode ? null : { mode: "goal", prompt: text.trim() },
          ),
        onTogglePlan: togglePlanMode,
        onStartLoop: () =>
          setLauncher({ mode: "loop", prompt: text.trim() }),
        onStartBest: () =>
          setLauncher({ mode: "best", prompt: text.trim() }),
        onToggleBackground: () =>
          setKind(kind === "background" ? "chat" : "background"),
        onSelectPersona: choosePersona,
        onEditSpec: () =>
          openModal(
            isSession
              ? { kind: "agent", sid: (props as any).sid }
              : {
                  kind: "new",
                  message: text,
                  spec: buildSpec({
                    provider,
                    model,
                    access,
                    persona,
                    effort,
                    budgetOverride,
                  }),
                  worker: personaById(persona).withWorker
                    ? DEFAULT_WORKER
                    : "",
                },
          ),
      }}
      accessPicker={{
        variant: isSession ? "session" : "home",
        active: isSession ? sessionAccess?.id : access,
        label: isSession
          ? sessionAccess?.label || "Access: set by agent spec"
          : accessLevel?.label,
        risk: isSession
          ? sessionAccess?.risk || "unknown"
          : accessLevel?.risk || "low",
        triggerRef: !isSession ? homeAccessTriggerRef : undefined,
        onHomeSelect: chooseHomeAccess,
        onSessionSelect: (target, close) => {
          switchMode(target);
          close();
        },
      }}
      goalOptions={
        goalMode
          ? {
              verifier: goalVerifier,
              rounds: goalRounds,
              onVerifierChange: setGoalVerifier,
              onRoundsChange: setGoalRounds,
              onExit: () => setLauncher(null),
            }
          : undefined
      }
      modelPicker={{
        provider,
        model,
        modelLabel,
        effort,
        effortLabel: effortLevel.label,
        budgetOverride,
        page: modelMenuPage,
        onOpen: () => setModelMenuPage("root"),
        onPageChange: setModelMenuPage,
        onSelectModel: chooseModel,
        onSelectEffort: chooseEffort,
        onCustomModel: () =>
          openPrompt({
            title: "Custom model id",
            label: "model id (provider stays " + provider + ")",
            initial: model,
            onSubmit: (id) => chooseModel(provider, id),
          }),
        onCustomBudget: () =>
          openPrompt({
            title: "Thinking budget override",
            label: "budget tokens (0 or empty = use the effort preset)",
            initial:
              budgetOverride != null ? String(budgetOverride) : "",
            onSubmit: (value) => {
              const next = Number(value.trim());
              chooseBudgetOverride(
                Number.isFinite(next) && next > 0
                  ? Math.floor(next)
                  : null,
              );
            },
          }),
      }}
      assistActions={{
        hasText: !!text.trim(),
        canUndo: undoDraft !== null,
        optimizing,
        micVisible,
        micActive,
        dictationBusy: dictation.busy,
        onOptimize: () => doOptimize(text, text),
        onUndo: undoOptimizeNow,
        onToggleMic: toggleMic,
      }}
      deliveryModeControl={
        running ? { mode: deliveryMode, onChange: setDeliveryMode } : undefined
      }
      submitButton={
        isSession &&
        (props as { running?: boolean }).running &&
        !text.trim() &&
        atts.length === 0
          ? {
              mode: "stop",
              onSubmit: () =>
                (props as { actions?: SessionActions }).actions?.interrupt?.(),
            }
          : {
              mode: "send",
              onSubmit: () => doSubmit(),
              disabled: busy || (!text.trim() && atts.length === 0),
              running,
              deliveryMode,
            }
      }
      fileInput={{
        ref: anyRef,
        onChange: (event) => {
          for (const file of Array.from(event.target.files || [])) {
            pick(file, file.type.startsWith("image/"));
          }
          event.target.value = "";
        },
      }}
    />
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

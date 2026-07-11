import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowClockwise,
  ArrowUp,
  CaretDown,
  ChartBar,
  Code,
  Cpu,
  Desktop,
  Eye,
  File,
  Folder,
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
  type AccessId,
  type EffortId,
} from "../specs";
import { Popover, PopItem, PopSection } from "./Popover";
import { useVoice } from "./useVoice";
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
      onSend: (text: string, images: string[], files: string[]) => Promise<void>;
      actions?: SessionActions;
      onError: (m: string) => void;
    };

interface Attachment {
  path: string;
  name: string;
  isImage: boolean;
}

// A slash command: what the menu shows and what Enter/click does. `needsArgs`
// commands complete to "/name " and wait; the rest run immediately.
interface SlashCmd {
  name: string;
  arg?: string;
  desc: string;
  variants: ("home" | "session")[];
  needsArgs?: boolean;
}

const SLASH: SlashCmd[] = [
  { name: "goal", arg: "<task>", desc: "Attach a goal — the agent keeps working until it's met", variants: ["home", "session"], needsArgs: true },
  { name: "loop", arg: "<task>", desc: "Start a run that repeats on a fixed cadence", variants: ["home", "session"], needsArgs: true },
  { name: "bestof", arg: "<task>", desc: "Run N isolated attempts, keep the best", variants: ["home", "session"], needsArgs: true },
  { name: "plan", desc: "Read-only planning mode — no changes", variants: ["home"] },
  { name: "compact", desc: "Summarize & shrink this conversation's context", variants: ["session"] },
  { name: "clear", desc: "Drop this conversation's context and start fresh", variants: ["session"] },
  { name: "mode", arg: "<default|acceptEdits>", desc: "Switch permission mode — acceptEdits auto-allows edits", variants: ["session"], needsArgs: true },
  { name: "diff", desc: "Show the workspace changes (git diff)", variants: ["session"] },
  { name: "fork", desc: "Fork into a new worktree from a checkpoint", variants: ["session"] },
  { name: "model", arg: "<id>", desc: "Switch the model", variants: ["home", "session"], needsArgs: true },
  { name: "reasoning", arg: "<level>", desc: "Set reasoning effort (off/light/medium/high/xhigh)", variants: ["home", "session"], needsArgs: true },
  { name: "interrupt", desc: "Stop the in-flight turn", variants: ["session"] },
  { name: "resume", desc: "Recover a crashed / interrupted session", variants: ["session"] },
];

const riskDot = (risk: string) => <span className={"risk-dot w-[7px] h-[7px] rounded-full shrink-0 inline-block " + risk} />;

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

  // model + reasoning effort + access posture + persona
  const [provider, setProvider] = useState(DEFAULT_MODEL.provider);
  const [model, setModel] = useState(DEFAULT_MODEL.id);
  const [effort, setEffort] = useState<EffortId>(DEFAULT_EFFORT);
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

  // home-only context
  const [ws, setWs] = useState("");
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

  const voice = useVoice((t) => {
    setText((prev) => (prev ? prev + " " + t : t));
    taRef.current?.focus();
  });

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
  const doSubmit = async () => {
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
        resetInput();
        await (props as Extract<ComposerProps, { variant: "session" }>).onSend(t, imgs, files);
      } else if (kind === "chat") {
        const workspace = await resolveHomeWorkspace();
        const spec = buildSpec({ provider, model, access, persona, effort });
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
        const spec = buildSpec({ provider, model, access, persona, effort });
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
  const applyModelSpec = async (p: string, id: string, eff: EffortId) => {
    if (!isSession) return;
    const sid = (props as any).sid as string;
    try {
      const base = recallSpec(sid) || buildSpec({ provider: p, model: id, access: "full", persona, effort: eff });
      const spec = replaceModel(base, p, id, eff);
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
    await applyModelSpec(provider, model, eff);
    if (isSession) toast(`Reasoning → ${effortById(eff).label} (from your next message)`, "info");
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
      const spec = buildSpec({ provider, model, access: acc, persona: id, effort });
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
        const spec = buildSpec({ provider, model, access, persona, effort });
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
        try {
          await AR.mode(sid, target);
          // Delivery ack, not an outcome: a busy session applies it at the
          // next safe boundary; the timeline chip is the truth.
          toast("Mode change requested — the timeline shows the outcome", "info");
        } catch (e: any) {
          props.onError(e.message);
        }
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

  const placeholder = narrow
    ? isSession
      ? "Ask for follow-ups"
      : "Describe a task"
    : isSession
      ? "Ask for follow-up changes, or type / for commands"
      : kind === "chat"
        ? "Describe a task, or type / for commands"
        : "Describe a one-shot task, or type / for commands";

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

      {/* Codex exposes project, run location, environment and branch as four
          separate controls. They share submit state, but never share a menu:
          each choice has one meaning and remains scannable before typing. */}
      {!isSession && (
        <div className="cx-env-strip">
          <Popover
            align="left"
            trigger={(open, toggle) => (
              <button className={"cx-env-control project" + (open ? " active" : "")} onClick={toggle} title="Select project">
                <FolderIcon />
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">{ws ? wsShort : "Select project"}</span>
                <Caret />
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
                      <PopItem icon={<X size={15} />} title="Don't work in a project" active={!ws} onClick={() => { setWs(""); setBranchInfo(null); setStartingBranch(""); setRunLocation("local"); close(); }} />
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
              <button className={"cx-env-control" + (open ? " active" : "")} onClick={toggle} title="Choose where this task runs">
                {runLocation === "local" ? <Desktop size={16} /> : <GitBranch size={16} />}
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">{runLocation === "local" ? "Local" : "New worktree"}</span>
                <Caret />
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
              <button className={"cx-env-control" + (open ? " active" : "")} onClick={toggle} title="Select local environment">
                <Code size={16} />
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">No environment</span>
                <Caret />
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu environment-menu">
                <PopSection label="Local environment">
                  <PopItem icon={<Code size={15} />} title="No environment" desc="Use AgentRunner's current local runtime" active onClick={close} />
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
              <button className={"cx-env-control branch" + (open ? " active" : "")} onClick={toggle} title={branchInfo?.isRepo ? "Choose starting branch" : "No Git branch available"} disabled={!branchInfo?.isRepo}>
                <BranchIcon />
                <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">{branchLabel}</span>
                {branchInfo?.isRepo && <Caret />}
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

      <div
        className={"cx-card" + (dragging ? " dropping" : "")}
        onDragEnter={onDragEnter}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
      >
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
              <button className={"cx-icon" + (open ? " active" : "")} onClick={toggle} title="Add & task options" aria-label="Add and task options">
                <PlusIcon />
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu wide cx-add-menu">
                <PopSection label="Add">
                  <PopItem icon={<Image size={16} />} title="Image" desc="Paste, drop, or pick an image" onClick={() => { close(); imgRef.current?.click(); }} />
                  <PopItem icon={<File size={16} />} title="File" desc="PDF, text, or any file (≤10MB)" onClick={() => { close(); anyRef.current?.click(); }} />
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
            <button
              className={"cx-pill cx-mode " + (sessionAccess?.risk || "unknown")}
              title={
                sessionAccess
                  ? "The session's live approval mode — /mode switches between default and acceptEdits"
                  : "This session's approval posture comes from its spec's permission rules; /mode switches default↔acceptEdits, and approvals surface here when a gate asks"
              }
              disabled
            >
              {riskDot(sessionAccess?.risk || "unknown")}
              {sessionAccess?.label || "Access: set by agent spec"}
            </button>
          ) : (
            <Popover
              align="left"
              panelClass="cx-pop-codex"
              trigger={(open, toggle) => (
                <button className={"cx-pill cx-mode " + (accessLevel?.risk || "low") + (open ? " active" : "")} onClick={toggle} title="How the agent's actions are approved">
                  {riskDot(accessLevel?.risk || "low")}
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
            trigger={(open, toggle) => (
              <button className={"cx-pill cx-model" + (open ? " active" : "")} onClick={toggle} title="Model & reasoning effort">
                <ModelIcon provider={provider} />
                {modelLabel}
                {effort !== "off" && <span className="cx-pill-sub">{effortLevel.label}</span>}
                <Caret />
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu wide cx-model-menu">
                <PopSection label="Model">
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
                  <PopItem
                    icon={<Code size={15} />}
                    title="Custom model id…"
                    right={<span aria-hidden>›</span>}
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
                </PopSection>
                <PopSection label="Reasoning effort">
                  {EFFORT_LEVELS.map((e) => (
                    <PopItem
                      key={e.id}
                      icon={<EffortIcon level={e.id} />}
                      title={e.label}
                      desc={e.desc}
                      active={effort === e.id}
                      onClick={() => { chooseEffort(e.id); close(); }}
                    />
                  ))}
                </PopSection>
              </div>
            )}
          </Popover>

          {/* voice */}
          {voice.supported && (
            <button className={"cx-icon cx-mic" + (voice.listening ? " listening" : "")} onClick={voice.toggle} title={voice.listening ? "Stop dictation" : "Dictate"}>
              <MicIcon />
            </button>
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
            <button className="cx-send" onClick={doSubmit} disabled={busy || !text.trim()} title="Send (Enter)">
              <ArrowUp />
            </button>
          )}
        </div>

      </div>

      {isSession && <div className="cx-status">{(props as any).running ? "running…" : "ready"}</div>}

      {/* hidden file inputs */}
      <input type="file" accept="image/*" ref={imgRef} style={{ display: "none" }} onChange={(e) => { const f = e.target.files?.[0]; if (f) pick(f, true); e.target.value = ""; }} />
      <input type="file" ref={anyRef} style={{ display: "none" }} onChange={(e) => { const f = e.target.files?.[0]; if (f) pick(f, false); e.target.value = ""; }} />
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
function parseSlash(text: string, variant: "home" | "session"): { cmd: string; rest: string } | null {
  const m = text.match(/^\/(\w+)(?:\s+([\s\S]*))?$/);
  if (!m) return null;
  const name = m[1].toLowerCase();
  const cmd = SLASH.find((c) => c.name === name && c.variants.includes(variant));
  if (!cmd) return null;
  const rest = (m[2] || "").trim();
  if (cmd.needsArgs && !rest) return null; // "/goal" alone → let the menu complete it
  return { cmd: name, rest };
}

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

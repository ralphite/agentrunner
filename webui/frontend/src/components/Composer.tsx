import { useEffect, useMemo, useRef, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import {
  ACCESS_LEVELS,
  accessById,
  buildDriverAgent,
  buildGoalDriver,
  buildLoopDriver,
  buildSpec,
  DEFAULT_ACCESS,
  DEFAULT_MODEL,
  DEFAULT_WORKER,
  MODELS,
  modelById,
  modelFromSpec,
  replaceModel,
  type AccessId,
} from "../specs";
import { Popover, PopItem, PopSection } from "./Popover";
import { useVoice } from "./useVoice";
import { recallAccess, recallSpec, rememberAccess, rememberSpec } from "./sessionSpecs";

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
  | { variant: "home"; onError: (m: string) => void }
  | {
      variant: "session";
      sid: string;
      workspace?: string;
      mode?: string; // the session's fixed approval mode (display only)
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
  { name: "goal", arg: "<task>", desc: "Start a goal-driven run that iterates until done", variants: ["home", "session"], needsArgs: true },
  { name: "loop", arg: "<task>", desc: "Start a run that repeats on a fixed cadence", variants: ["home", "session"], needsArgs: true },
  { name: "plan", desc: "Read-only planning mode — no changes", variants: ["home"] },
  { name: "compact", desc: "Summarize & shrink this conversation's context", variants: ["session"] },
  { name: "clear", desc: "Drop this conversation's context and start fresh", variants: ["session"] },
  { name: "diff", desc: "Show the workspace changes (git diff)", variants: ["session"] },
  { name: "fork", desc: "Fork into a new worktree from a checkpoint", variants: ["session"] },
  { name: "model", arg: "<id>", desc: "Switch the model", variants: ["home", "session"], needsArgs: true },
  { name: "interrupt", desc: "Stop the in-flight turn", variants: ["session"] },
  { name: "resume", desc: "Recover a crashed / interrupted session", variants: ["session"] },
];

const riskDot = (risk: string) => <span className={"risk-dot " + risk} />;

export function Composer(props: ComposerProps) {
  const { select, selectRun, refreshSessions, refreshRuns, openModal, toast } = useStore();
  const isSession = props.variant === "session";

  const [text, setText] = useState("");
  const [atts, setAtts] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);

  // model + access posture
  const [provider, setProvider] = useState(DEFAULT_MODEL.provider);
  const [model, setModel] = useState(DEFAULT_MODEL.id);
  const [access, setAccess] = useState<AccessId>(DEFAULT_ACCESS);

  // home-only context
  const [ws, setWs] = useState("");
  const [kind, setKind] = useState<"chat" | "background">("chat");
  const [branchInfo, setBranchInfo] = useState<{ isRepo: boolean; current: string; branches: string[]; dirty: number } | null>(null);

  // goal / loop launcher panel
  const [launcher, setLauncher] = useState<null | { mode: "goal" | "loop"; task: string }>(null);

  // slash menu
  const [slashOpen, setSlashOpen] = useState(false);
  const [slashIdx, setSlashIdx] = useState(0);

  const taRef = useRef<HTMLTextAreaElement>(null);
  const imgRef = useRef<HTMLInputElement>(null);
  const anyRef = useRef<HTMLInputElement>(null);

  const voice = useVoice((t) => {
    setText((prev) => (prev ? prev + " " + t : t));
    taRef.current?.focus();
  });

  // Seed model pill from the session's remembered spec (if we made it).
  useEffect(() => {
    if (!isSession) return;
    const sp = recallSpec((props as any).sid);
    const m = sp ? modelFromSpec(sp) : null;
    if (m) {
      setProvider(m.provider);
      setModel(m.id);
    }
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
      .then((b) => alive && setBranchInfo(b))
      .catch(() => alive && setBranchInfo(null));
    return () => {
      alive = false;
    };
  }, [isSession, ws]);

  const modelLabel = modelById(provider, model)?.label || model;
  const accessLevel = isSession ? undefined : accessById(access);
  const remembered = isSession ? recallAccess((props as any).sid) : undefined;
  const sessionAccess = isSession ? (remembered ? accessById(remembered) : accessByMode((props as any).mode)) : undefined;

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

  const ensureWs = async (): Promise<string> => {
    if (ws.trim()) return ws.trim();
    const p = (await AR.makeWorkspace()).path;
    setWs(p);
    return p;
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
        const workspace = await ensureWs();
        const spec = buildSpec({ provider, model, access });
        const imgs = atts.filter((a) => a.isImage).map((a) => a.path);
        const files = atts.filter((a) => !a.isImage).map((a) => a.path);
        const r = await AR.newSession({
          spec,
          extraSpecs: [{ name: "worker.yaml", content: DEFAULT_WORKER }],
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
        const workspace = await ensureWs();
        const spec = buildSpec({ provider, model, access });
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

  // ---- model switch ----
  const chooseModel = async (p: string, id: string) => {
    setProvider(p);
    setModel(id);
    if (isSession) {
      const sid = (props as any).sid as string;
      try {
        const base = recallSpec(sid) || buildSpec({ provider: p, model: id, access: "full" });
        const spec = replaceModel(base, p, id);
        await AR.switchAgent(sid, spec, [{ name: "worker.yaml", content: DEFAULT_WORKER }]);
        rememberSpec(sid, spec);
        toast(`Model → ${modelById(p, id)?.label || id} (from your next message)`, "info");
      } catch (e: any) {
        props.onError(e.message);
      }
    }
  };

  // ---- goal / loop ----
  const startGoal = async (task: string, verifier: string, iterations: number) => {
    const workspace = isSession ? (props as any).workspace || (await ensureWs()) : await ensureWs();
    if (!workspace) return props.onError("a workspace is required to start a goal");
    setBusy(true);
    try {
      const r = await AR.startRun({
        kind: "drive",
        spec: buildGoalDriver({ task, maxIterations: iterations, verifier, provider, model }),
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

  // ---- slash execution ----
  const runSlash = async (name: string, rest: string) => {
    const sid = isSession ? ((props as any).sid as string) : "";
    const act = isSession ? ((props as any).actions as SessionActions | undefined) : undefined;
    switch (name) {
      case "goal":
        setLauncher({ mode: "goal", task: rest });
        setText("");
        return;
      case "loop":
        setLauncher({ mode: "loop", task: rest });
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
      case "compact":
        setText("");
        try {
          await AR.compact(sid);
          toast("Context compacted", "info");
        } catch (e: any) {
          props.onError(e.message);
        }
        return;
      case "clear":
        setText("");
        try {
          await AR.clear(sid);
          toast("Context cleared", "info");
        } catch (e: any) {
          props.onError(e.message);
        }
        return;
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

  const placeholder = isSession
    ? "Reply, or type / for commands…"
    : kind === "chat"
      ? "Describe a task, or ask a question…  (/ for commands)"
      : "Describe a one-shot background task…";

  const wsShort = ws ? ws.split("/").filter(Boolean).slice(-1)[0] : "auto-created";

  return (
    <div className={"cx " + (isSession ? "cx-session" : "cx-home")}>
      {launcher && (
        <GoalLoopLauncher
          mode={launcher.mode}
          initialTask={launcher.task}
          busy={busy}
          onCancel={() => setLauncher(null)}
          onStart={(task, a, b) => (launcher.mode === "goal" ? startGoal(task, a, b) : startLoop(task, a, b))}
        />
      )}

      <div className="cx-card">
        {atts.length > 0 && (
          <div className="cx-atts">
            {atts.map((a, i) => (
              <span className="cx-att" key={i} onClick={() => setAtts((p) => p.filter((_, j) => j !== i))} title="remove">
                <span className="cx-att-ico">{a.isImage ? "🖼" : "📄"}</span>
                {a.name}
                <span className="cx-att-x">✕</span>
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

        {slashOpen && filteredSlash.length > 0 && (
          <div className="cx-slash">
            <div className="cx-slash-hd">Commands</div>
            {filteredSlash.map((c, i) => (
              <button
                key={c.name}
                className={"cx-slash-item" + (i === slashIdx ? " on" : "")}
                onMouseEnter={() => setSlashIdx(i)}
                onClick={() => applySlash(c)}
              >
                <span className="cx-slash-name">
                  /{c.name}
                  {c.arg && <span className="cx-slash-arg"> {c.arg}</span>}
                </span>
                <span className="cx-slash-desc">{c.desc}</span>
              </button>
            ))}
          </div>
        )}

        {/* ---- control bar ---- */}
        <div className="cx-bar">
          {/* + Add menu */}
          <Popover
            align="left"
            trigger={(open, toggle) => (
              <button className={"cx-icon" + (open ? " active" : "")} onClick={toggle} title="Add">
                <PlusIcon />
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu">
                <PopSection label="Add">
                  <PopItem icon={<span>🖼</span>} title="Image" desc="Paste, drop, or pick an image" onClick={() => { close(); imgRef.current?.click(); }} />
                  <PopItem icon={<span>📄</span>} title="File" desc="PDF, text, or any file (≤10MB)" onClick={() => { close(); anyRef.current?.click(); }} />
                </PopSection>
                <PopSection label="Run as">
                  <PopItem icon={<GoalIcon />} title="Goal" desc="Iterate until the goal is met" onClick={() => { close(); setLauncher({ mode: "goal", task: text.trim() }); }} />
                  <PopItem icon={<LoopIcon />} title="Loop" desc="Repeat on a fixed cadence" onClick={() => { close(); setLauncher({ mode: "loop", task: text.trim() }); }} />
                  {!isSession && <PopItem icon={<PlanIcon />} title="Plan mode" desc="Read-only planning" active={access === "plan"} onClick={() => { close(); setAccess("plan"); }} />}
                </PopSection>
                <PopSection label="Advanced">
                  <PopItem icon={<span>{"{}"}</span>} title="Edit agent spec (YAML)…" onClick={() => { close(); openModal(isSession ? { kind: "agent", sid: (props as any).sid } : { kind: "new", message: text }); }} />
                </PopSection>
              </div>
            )}
          </Popover>

          {/* permission mode pill */}
          {isSession ? (
            <button className={"cx-pill cx-mode " + (sessionAccess?.risk || "low")} title="Approval mode is set when the session is created and can't change mid-session" disabled>
              {riskDot(sessionAccess?.risk || "low")}
              {sessionAccess?.label || "Ask to approve"}
            </button>
          ) : (
            <Popover
              align="left"
              trigger={(open, toggle) => (
                <button className={"cx-pill cx-mode " + (accessLevel?.risk || "low") + (open ? " active" : "")} onClick={toggle} title="How the agent's actions are approved">
                  {riskDot(accessLevel?.risk || "low")}
                  {accessLevel?.label}
                  <Caret />
                </button>
              )}
            >
              {(close) => (
                <div className="cx-menu wide">
                  <PopSection label="How should actions be approved?">
                    {ACCESS_LEVELS.map((a) => (
                      <PopItem
                        key={a.id}
                        icon={riskDot(a.risk)}
                        title={a.label}
                        desc={a.desc}
                        active={access === a.id}
                        onClick={() => { setAccess(a.id); close(); }}
                      />
                    ))}
                  </PopSection>
                </div>
              )}
            </Popover>
          )}

          <span className="cx-spacer" />

          {/* model pill */}
          <Popover
            align="right"
            trigger={(open, toggle) => (
              <button className={"cx-pill cx-model" + (open ? " active" : "")} onClick={toggle} title="Model">
                <ModelIcon />
                {modelLabel}
                <Caret />
              </button>
            )}
          >
            {(close) => (
              <div className="cx-menu wide">
                <PopSection label="Model">
                  {MODELS.map((m) => (
                    <PopItem
                      key={m.provider + m.id}
                      icon={<ModelIcon />}
                      title={m.label}
                      desc={m.sub}
                      active={provider === m.provider && model === m.id}
                      onClick={() => { chooseModel(m.provider, m.id); close(); }}
                    />
                  ))}
                </PopSection>
                <PopSection>
                  <PopItem
                    icon={<span>✎</span>}
                    title="Custom model id…"
                    onClick={() => {
                      const id = window.prompt("Model id (provider stays " + provider + "):", model);
                      if (id && id.trim()) chooseModel(provider, id.trim());
                      close();
                    }}
                  />
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

          {/* send */}
          <button className="cx-send" onClick={doSubmit} disabled={busy || !text.trim()} title="Send (Enter)">
            <ArrowUp />
          </button>
        </div>

        {/* ---- context bar (home only) ---- */}
        {!isSession && (
          <div className="cx-ctx">
            <Popover
              align="left"
              trigger={(open, toggle) => (
                <button className={"cx-ctx-pill" + (open ? " active" : "")} onClick={toggle} title="Workspace">
                  <FolderIcon />
                  {wsShort}
                  <Caret />
                </button>
              )}
            >
              {(close) => (
                <div className="cx-menu wide">
                  <PopSection label="Workspace">
                    <PopItem icon={<span>✨</span>} title="New empty workspace" desc="A fresh directory under runtime/" onClick={async () => { try { setWs((await AR.makeWorkspace()).path); } catch (e: any) { props.onError(e.message); } close(); }} />
                    <PopItem icon={<FolderIcon />} title="Enter a path…" desc="An absolute directory to work in" onClick={() => { const p = window.prompt("Absolute workspace path:", ws); if (p && p.trim()) setWs(p.trim()); close(); }} />
                  </PopSection>
                  {ws && (
                    <div className="cx-ctx-path" title={ws}>
                      {ws}
                    </div>
                  )}
                </div>
              )}
            </Popover>

            <Popover
              align="left"
              trigger={(open, toggle) => (
                <button className={"cx-ctx-pill" + (open ? " active" : "")} onClick={toggle} title="Start mode">
                  <StartIcon />
                  {kind === "chat" ? "Interactive" : "Background"}
                  <Caret />
                </button>
              )}
            >
              {(close) => (
                <div className="cx-menu wide">
                  <PopSection label="Start in">
                    <PopItem icon={<StartIcon />} title="Interactive session" desc="Chat back and forth with the agent" active={kind === "chat"} onClick={() => { setKind("chat"); close(); }} />
                    <PopItem icon={<span>⚡</span>} title="Background task" desc="One-shot: runs to completion on its own" active={kind === "background"} onClick={() => { setKind("background"); close(); }} />
                  </PopSection>
                </div>
              )}
            </Popover>

            {branchInfo?.isRepo && (
              <Popover
                align="left"
                onOpen={() => ws.trim() && AR.gitBranches(ws.trim()).then(setBranchInfo).catch(() => {})}
                trigger={(open, toggle) => (
                  <button className={"cx-ctx-pill" + (open ? " active" : "")} onClick={toggle} title="Git branch">
                    <BranchIcon />
                    {branchInfo.current || "branch"}
                    <Caret />
                  </button>
                )}
              >
                {(close) => (
                  <div className="cx-menu wide">
                    <PopSection label={`Branches${branchInfo.dirty ? ` · ${branchInfo.dirty} uncommitted` : ""}`}>
                      {branchInfo.branches.map((b) => (
                        <PopItem
                          key={b}
                          icon={<BranchIcon />}
                          title={b}
                          active={b === branchInfo.current}
                          onClick={async () => {
                            close();
                            if (b === branchInfo.current) return;
                            try {
                              await AR.gitCheckout(ws.trim(), b, false);
                              setBranchInfo({ ...branchInfo, current: b });
                              toast(`Switched to ${b}`, "info");
                            } catch (e: any) {
                              props.onError(e.message);
                            }
                          }}
                        />
                      ))}
                    </PopSection>
                    <PopSection>
                      <PopItem
                        icon={<PlusIcon />}
                        title="Create & checkout new branch…"
                        onClick={async () => {
                          const b = window.prompt("New branch name:");
                          close();
                          if (!b || !b.trim()) return;
                          try {
                            await AR.gitCheckout(ws.trim(), b.trim(), true);
                            setBranchInfo({ ...branchInfo, current: b.trim(), branches: [b.trim(), ...branchInfo.branches] });
                            toast(`Created & switched to ${b.trim()}`, "info");
                          } catch (e: any) {
                            props.onError(e.message);
                          }
                        }}
                      />
                    </PopSection>
                  </div>
                )}
              </Popover>
            )}
          </div>
        )}
      </div>

      {isSession && <div className="cx-status">{(props as any).running ? "running…" : "ready"}</div>}

      {/* hidden file inputs */}
      <input type="file" accept="image/*" ref={imgRef} style={{ display: "none" }} onChange={(e) => { const f = e.target.files?.[0]; if (f) pick(f, true); e.target.value = ""; }} />
      <input type="file" ref={anyRef} style={{ display: "none" }} onChange={(e) => { const f = e.target.files?.[0]; if (f) pick(f, false); e.target.value = ""; }} />
    </div>
  );
}

// ---- goal / loop launcher --------------------------------------------------
function GoalLoopLauncher({
  mode,
  initialTask,
  busy,
  onCancel,
  onStart,
}: {
  mode: "goal" | "loop";
  initialTask: string;
  busy: boolean;
  onCancel: () => void;
  onStart: (task: string, second: string, iterations: number) => void;
}) {
  const [task, setTask] = useState(initialTask);
  const [second, setSecond] = useState(mode === "goal" ? "" : "5m"); // verifier | interval
  const [iters, setIters] = useState(mode === "goal" ? 8 : 5);
  return (
    <div className="cx-launcher">
      <div className="cx-launcher-hd">
        {mode === "goal" ? <GoalIcon /> : <LoopIcon />}
        <b>{mode === "goal" ? "Goal" : "Loop"}</b>
        <span className="dim">
          {mode === "goal" ? "iterate until the goal is met" : "repeat on a fixed cadence"}
        </span>
        <span className="cx-spacer" />
        <button className="ghost sm" onClick={onCancel}>✕</button>
      </div>
      <textarea className="cx-launcher-task" rows={2} placeholder={mode === "goal" ? "What goal should the agent keep working toward?" : "What should each iteration do?"} value={task} onChange={(e) => setTask(e.target.value)} />
      <div className="cx-launcher-row">
        {mode === "goal" ? (
          <label className="cx-launcher-field" title="A shell command that must exit 0 for the goal to count as met (optional)">
            <span>Done when (command)</span>
            <input placeholder="e.g. go test ./…  (optional)" value={second} onChange={(e) => setSecond(e.target.value)} />
          </label>
        ) : (
          <label className="cx-launcher-field" title="How often to run (Go duration, e.g. 30s, 5m, 1h)">
            <span>Every</span>
            <input placeholder="5m" value={second} onChange={(e) => setSecond(e.target.value)} />
          </label>
        )}
        <label className="cx-launcher-field small" title="Safety cap on iterations">
          <span>Max rounds</span>
          <input type="number" min={1} value={iters} onChange={(e) => setIters(Math.max(1, Number(e.target.value) || 1))} />
        </label>
        <button className="primary cx-launcher-go" disabled={busy || !task.trim() || (mode === "loop" && !second.trim())} onClick={() => onStart(task.trim(), second.trim(), iters)}>
          Start {mode}
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
  return ACCESS_LEVELS.find((a) => a.id === "ask")!;
}

// ---- inline icons (stroke, currentColor) ----
const Caret = () => (
  <svg className="cx-caret" width="10" height="10" viewBox="0 0 10 10"><path d="M2 4l3 3 3-3" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" /></svg>
);
const PlusIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16"><path d="M8 3v10M3 8h10" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" /></svg>
);
const ArrowUp = () => (
  <svg width="16" height="16" viewBox="0 0 16 16"><path d="M8 13V3M4 7l4-4 4 4" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" /></svg>
);
const MicIcon = () => (
  <svg width="15" height="15" viewBox="0 0 16 16"><rect x="6" y="2" width="4" height="7" rx="2" fill="none" stroke="currentColor" strokeWidth="1.3" /><path d="M4 8a4 4 0 0 0 8 0M8 12v2" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" /></svg>
);
const ModelIcon = () => (
  <svg width="13" height="13" viewBox="0 0 16 16"><path d="M8 2l5 3v6l-5 3-5-3V5z" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round" /></svg>
);
const FolderIcon = () => (
  <svg width="13" height="13" viewBox="0 0 16 16"><path d="M2 4h4l1.2 1.5H14V12H2z" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round" /></svg>
);
const BranchIcon = () => (
  <svg width="13" height="13" viewBox="0 0 16 16"><circle cx="4" cy="4" r="1.6" fill="none" stroke="currentColor" strokeWidth="1.2" /><circle cx="4" cy="12" r="1.6" fill="none" stroke="currentColor" strokeWidth="1.2" /><circle cx="12" cy="5" r="1.6" fill="none" stroke="currentColor" strokeWidth="1.2" /><path d="M4 5.6v4.8M4 8h4a4 4 0 0 0 4-1.4" fill="none" stroke="currentColor" strokeWidth="1.2" /></svg>
);
const StartIcon = () => (
  <svg width="13" height="13" viewBox="0 0 16 16"><rect x="2" y="3" width="12" height="9" rx="1.5" fill="none" stroke="currentColor" strokeWidth="1.2" /><path d="M6 14h4" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" /></svg>
);
const GoalIcon = () => (
  <svg width="14" height="14" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.2" /><circle cx="8" cy="8" r="2.4" fill="none" stroke="currentColor" strokeWidth="1.2" /></svg>
);
const LoopIcon = () => (
  <svg width="14" height="14" viewBox="0 0 16 16"><path d="M3 8a5 5 0 0 1 8.5-3.5M13 8a5 5 0 0 1-8.5 3.5" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" /><path d="M11 2v3H8M5 14v-3h3" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" /></svg>
);
const PlanIcon = () => (
  <svg width="14" height="14" viewBox="0 0 16 16"><path d="M4 4h8M4 8h8M4 12h5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" /></svg>
);

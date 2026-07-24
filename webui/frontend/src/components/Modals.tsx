import { useEffect, useId, useState } from "react";
import { Folder, Globe, Terminal, X } from "@phosphor-icons/react";
import { useAppServices } from "../app/appServices";
import { useStore, type ModalKind } from "../store";
import { cadenceText, runFormDefaults, type CadenceSpec, type RunPreset, type ScheduleKind } from "../runPreset";
import { scheduleFieldError } from "../scheduleValidate";
import type { SpecFile } from "../types";
import {
  DEFAULT_DRIVER,
  DEFAULT_EFFORT,
  DEFAULT_MODEL,
  EFFORT_LEVELS,
  legacyModelFromSpec,
  MODELS,
  stripLegacyModel,
  type EffortId,
} from "../specs";
import { displayTitle } from "../title";
import { compactCount, summarizeInspect } from "../inspectPresentation";
import { Button } from "../ui/Button";
import { FocusScope } from "../ui/FocusScope";
import { IconButton } from "../ui/IconButton";
import { Input, Select, Textarea } from "../ui/Field";
import { friendlyStatus } from "./pill";
import {
  recallAccess,
  recallModel,
  recallSpec,
  rememberAccess,
  rememberModel,
  rememberSpec,
} from "./sessionSpecs";

// "Image" (input modality) and "Images" (capability flag) state the same fact
// twice, so the chip row read like a plural typo (FB-3). Dedupe on a
// singular/case-insensitive key, keeping the first label's casing.
export function dedupeCaps(labels: string[]): string[] {
  const seen = new Set<string>();
  return labels.filter((label) => {
    const key = label.toLowerCase().replace(/s$/, "");
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function Modal({
  title,
  onClose,
  children,
  footer,
  returnFocus,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  footer?: React.ReactNode;
  returnFocus?: HTMLElement;
}) {
  // Keyboard avoidance (phone): the backdrop centers the modal, so when the
  // iOS keyboard opens it covers the lower fields. Mirror the visual viewport
  // height into --app-vvh so the backdrop shrinks to the VISIBLE area and the
  // modal re-centers above the keyboard. Cleared on unmount.
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const sync = () => document.documentElement.style.setProperty("--app-vvh", vv.height + "px");
    sync();
    vv.addEventListener("resize", sync);
    vv.addEventListener("scroll", sync);
    return () => {
      vv.removeEventListener("resize", sync);
      vv.removeEventListener("scroll", sync);
      document.documentElement.style.removeProperty("--app-vvh");
    };
  }, []);
  return (
    <div
      className="backdrop bottom-auto h-[var(--app-vvh,100dvh)] overflow-hidden max-[640px]:p-2"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <FocusScope
        className="modal mx-auto flex max-h-[calc(var(--app-vvh,100dvh)-16vh)] min-h-0 flex-col max-[640px]:max-h-[calc(var(--app-vvh,100dvh)-1rem)]"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        initialFocus={[
          ".mbody input:not(:disabled), .mbody textarea:not(:disabled)",
          ".mbody select:not(:disabled), .mbody button:not(:disabled)",
        ]}
        restoreFocus={returnFocus ?? true}
        onEscape={onClose}
      >
        <div className="mhead shrink-0">
          <span className="min-w-0 truncate">{title}</span>
          <IconButton
            aria-label="Close dialog"
            className="-m-2"
            size="lg"
            variant="ghost"
            onClick={onClose}
          >
            <X size={16} />
          </IconButton>
        </div>
        <div className="mbody min-h-0 flex-1 overflow-y-auto overscroll-contain">{children}</div>
        {footer && <div className="mfoot shrink-0 max-[640px]:flex-wrap max-[640px]:justify-end">{footer}</div>}
      </FocusScope>
    </div>
  );
}

export function Modals() {
  const { modal, prompt } = useStore();
  return (
    <>
      {modal && <MainModal modal={modal} />}
      {prompt && <PromptModal {...prompt} />}
    </>
  );
}

export function MainModal({ modal }: { modal: NonNullable<ModalKind> }) {
  switch (modal.kind) {
    case "new":
      return (
        <NewSessionModal
          initialMessage={modal.message}
          initialSpec={modal.spec}
          initialWorker={modal.worker}
          initialProvider={modal.provider}
          initialModel={modal.model}
          initialEffort={modal.effort}
        />
      );
    case "run":
      return <RunModal initialPrompt={modal.prompt} preset={modal.preset} cadence={modal.cadence} returnFocus={modal.returnFocus} />;
    case "fork":
      return <ForkModal sid={modal.sid} />;
    case "agent":
      return (
        <AgentModal
          sid={modal.sid}
          provider={modal.provider}
          model={modal.model}
          effort={modal.effort}
        />
      );
    case "rename":
      return <RenameModal sid={modal.sid} />;
    case "trust":
      return <TrustModal />;
    case "confirm":
      return <ConfirmModal modal={modal} />;
    case "inspect":
      return <RunDetailsModal data={modal.data} status={modal.status} />;
    case "viewer":
      return <ViewerModal title={modal.title} body={modal.body} />;
  }
}

export function ConfirmModal({ modal }: { modal: Extract<NonNullable<ModalKind>, { kind: "confirm" }> }) {
  const { openModal, toast } = useStore();
  const [busy, setBusy] = useState(false);
  const close = () => {
    openModal(null);
    modal.onClose?.();
  };
  const confirm = async () => {
    setBusy(true);
    try {
      await modal.onConfirm();
      close();
    } catch (error: any) {
      toast(error.message);
      setBusy(false);
    }
  };
  return (
    <Modal
      title={modal.title}
      onClose={close}
      footer={
        <>
          <Button variant="outline" onClick={close}>Cancel</Button>
          <Button
            variant={modal.danger ? "outline" : "solid"}
            tone={modal.danger ? "danger" : "neutral"}
            loading={busy}
            onClick={confirm}
          >
            {modal.confirmLabel}
          </Button>
        </>
      }
    >
      <p className="confirm-copy">{modal.body}</p>
      {modal.details && modal.details.length > 0 && (
        <div className="confirm-details">
          {modal.details.map((detail) => (
            <div className="confirm-detail" key={detail.title}>
              <span className="confirm-detail-icon" aria-hidden>
                {detail.icon === "files" ? <Folder size={18} /> : detail.icon === "terminal" ? <Terminal size={18} /> : <Globe size={18} />}
              </span>
              <span className="min-w-0">
                <b>{detail.title}</b>
                <small>{detail.body}</small>
              </span>
            </div>
          ))}
        </div>
      )}
      {modal.note && <p className="confirm-note">{modal.note}</p>}
    </Modal>
  );
}

// PromptModal is the app-styled replacement for window.prompt (QA Round1
// F-C1: the native dialog synchronously freezes the renderer and looks
// nothing like the rest of the UI). It renders after (= on top of) the
// main modal, so flows inside a modal can ask for one more string.
export function PromptModal({
  title,
  label,
  initial,
  placeholder,
  submitLabel,
  onSubmit,
}: {
  title: string;
  label?: string;
  initial?: string;
  placeholder?: string;
  submitLabel?: string;
  onSubmit: (value: string) => void;
}) {
  const { openPrompt } = useStore();
  const inputId = useId();
  const [value, setValue] = useState(initial || "");
  const close = () => openPrompt(null);
  const submit = () => {
    const v = value.trim();
    if (!v) return;
    close();
    onSubmit(v);
  };
  return (
    <Modal
      title={title}
      onClose={close}
      footer={
        <>
          <Button variant="ghost" onClick={close}>Cancel</Button>
          <Button variant="solid" disabled={!value.trim()} onClick={submit}>
            {submitLabel || "OK"}
          </Button>
        </>
      }
    >
      {label && <label className="field" htmlFor={inputId}>{label}</label>}
      <Input
        id={inputId}
        type="text"
        autoFocus
        value={value}
        placeholder={placeholder}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") submit();
          if (e.key === "Escape") close();
        }}
      />
    </Modal>
  );
}

function useWorkspace() {
  const { api } = useAppServices();
  const { toast, openPrompt } = useStore();
  const [ws, setWs] = useState("");
  const mk = async () => {
    try {
      setWs((await api.makeWorkspace()).path);
    } catch (e: any) {
      toast(e.message);
    }
  };
  const ensure = async () => {
    const v = ws.trim();
    if (v) {
      // A bare name like "abc" would resolve relative to the server's cwd and
      // fail with a raw "workspace is not an existing directory: /…/abc"
      // (phone report 2026-07-12). Catch it here with actionable guidance
      // instead of leaking the resolved path. Blank = a fresh scratch dir.
      if (!v.startsWith("/") && !v.startsWith("~")) {
        throw new Error("Workspace must be a full path (starting with /), or leave it blank for a new scratch workspace — or tap “Use folder…”.");
      }
      return v;
    }
    const path = (await api.makeWorkspace()).path;
    setWs(path);
    return path;
  };
  const choose = () => {
    openPrompt({
      title: "Choose workspace",
      label: "absolute folder path",
      initial: ws.trim(),
      placeholder: "/path/to/workspace",
      onSubmit: setWs,
    });
  };
  // Codex "New worktree": create an isolated git worktree of a repo so the
  // agent's edits don't touch the user's checkout. Prompts for the repo path.
  const mkWorktree = () => {
    openPrompt({
      title: "New git worktree",
      label: "repo path to branch the worktree from",
      initial: ws.trim(),
      placeholder: "/path/to/repo",
      onSubmit: async (repo) => {
        try {
          setWs((await api.makeWorktree(repo, "")).path);
          toast("created worktree", "info");
        } catch (e: any) {
          toast(e.message);
        }
      },
    });
  };
  return { ws, setWs, mk, ensure, choose, mkWorktree };
}

function ModelFields({
  provider,
  model,
  effort,
  onModel,
  onEffort,
}: {
  provider: string;
  model: string;
  effort: EffortId;
  onModel: (provider: string, model: string) => void;
  onEffort: (effort: EffortId) => void;
}) {
  return (
    <div className="row-flex">
      <div style={{ flex: 1 }}>
        <label className="field" htmlFor="modal-model">
          Model
        </label>
        <Select
          id="modal-model"
          value={`${provider}/${model}`}
          onChange={(event) => {
            const selected = MODELS.find(
              (choice) =>
                `${choice.provider}/${choice.id}` === event.target.value,
            );
            if (selected) onModel(selected.provider, selected.id);
          }}
        >
          {MODELS.map((choice) => (
            <option
              key={`${choice.provider}/${choice.id}`}
              value={`${choice.provider}/${choice.id}`}
            >
              {choice.label}
            </option>
          ))}
          {!MODELS.some(
            (choice) =>
              choice.provider === provider && choice.id === model,
          ) && <option value={`${provider}/${model}`}>{model}</option>}
        </Select>
      </div>
      <div style={{ flex: 1 }}>
        <label className="field" htmlFor="modal-effort">
          Effort
        </label>
        <Select
          id="modal-effort"
          value={effort}
          onChange={(event) =>
            onEffort(event.target.value as EffortId)}
        >
          {EFFORT_LEVELS.map((level) => (
            <option key={level.id} value={level.id}>
              {level.label}
            </option>
          ))}
        </Select>
      </div>
    </div>
  );
}

export function NewSessionModal({
  initialMessage,
  initialSpec,
  initialWorker,
  initialProvider,
  initialModel,
  initialEffort,
}: {
  initialMessage?: string;
  initialSpec?: string;
  initialWorker?: string;
  initialProvider?: string;
  initialModel?: string;
  initialEffort?: string;
}) {
  const { api, storage } = useAppServices();
  const { openModal, select, refreshSessions, toast } = useStore();
  const { ws, setWs, ensure, choose, mkWorktree } = useWorkspace();
  const [msg, setMsg] = useState(initialMessage || "");
  const [mode, setMode] = useState("");
  const [spec, setSpec] = useState(initialSpec || "");
  const [worker, setWorker] = useState(initialWorker || "");
  const [provider, setProvider] = useState(
    initialProvider || DEFAULT_MODEL.provider,
  );
  const [model, setModel] = useState(initialModel || DEFAULT_MODEL.id);
  const [effort, setEffort] = useState<EffortId>(
    EFFORT_LEVELS.some((level) => level.id === initialEffort)
      ? (initialEffort as EffortId)
      : DEFAULT_EFFORT,
  );
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);

  useEffect(() => {
    if (spec.trim()) return;
    api
      .agents()
      .then((catalog) =>
        setSpec((current) =>
          current.trim()
            ? current
            : (
                catalog.find((entry) => entry.name === "dev") ||
                catalog[0]
              )?.yaml || "",
        ),
      )
      .catch((error: Error) => toast(error.message));
  }, []);

  const create = async () => {
    setBusy(true);
    try {
      const extraSpecs: SpecFile[] = [];
      if (worker.trim()) extraSpecs.push({ name: "worker.yaml", content: worker });
      const workspace = await ensure();
      const r = await api.newSession({
        provider,
        model,
        effort,
        spec,
        extraSpecs,
        workspace,
        message: msg.trim(),
        mode,
      });
      rememberSpec(r.sid, spec, storage.local);
      rememberModel(r.sid, { provider, model, effort }, storage.local);
      close();
      await refreshSessions();
      select(r.sid);
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title="Advanced session setup"
      onClose={close}
      footer={
        <>
          <Button variant="outline" onClick={() => openModal({ kind: "run", prompt: msg })}>Create a background run…</Button>
          <Button
            variant="solid"
            disabled={!msg.trim() || !spec.trim()}
            loading={busy}
            onClick={create}
          >
            Start session
          </Button>
        </>
      }
    >
      <label className="field" htmlFor="new-session-message">Opening message</label>
      <Textarea id="new-session-message" autoFocus rows={3} value={msg} onChange={(e) => setMsg(e.target.value)} placeholder="Describe the outcome you want" />
      <label className="field" htmlFor="new-session-workspace">Workspace</label>
      <div className="row-flex">
        <Input id="new-session-workspace" type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="Leave blank for a new scratch workspace" />
        <Button variant="outline" onClick={choose}>Use folder…</Button>
        <Button variant="outline" onClick={mkWorktree} title="Codex 'New worktree': branch a fresh, isolated git worktree of a repo so edits don't touch your checkout">
          New worktree…
        </Button>
      </div>
      <label className="field" htmlFor="new-session-approval">Approval mode</label>
      <Select id="new-session-approval" value={mode} onChange={(e) => setMode(e.target.value)} title="permission mode: default asks for approval on gated tools · plan = read-only planning · acceptEdits auto-approves file edits">
        <option value="">default</option>
        <option value="plan">plan</option>
        <option value="acceptEdits">acceptEdits</option>
      </Select>
      <ModelFields
        provider={provider}
        model={model}
        effort={effort}
        onModel={(nextProvider, nextModel) => {
          setProvider(nextProvider);
          setModel(nextModel);
        }}
        onEffort={setEffort}
      />
      <label className="field" htmlFor="new-session-spec">Agent specification (YAML)</label>
      <Textarea id="new-session-spec" code className="code" rows={11} value={spec} onChange={(e) => setSpec(e.target.value)} />
      <label className="field" htmlFor="new-session-worker">Worker specification (optional YAML)</label>
      <Textarea id="new-session-worker" code className="code" rows={6} value={worker} onChange={(e) => setWorker(e.target.value)} />
    </Modal>
  );
}

// withSchedule injects the chosen schedule into a driver.yaml: it strips any
// existing top-level schedule/interval/cron/n lines and appends the new ones,
// so the UI controls stay authoritative over what the user typed.
function withSchedule(driver: string, schedule: string, interval: string, cron: string, n: number): string {
  if (!schedule) return driver;
  const kept = driver.split("\n").filter((l) => !/^\s*(schedule|interval|cron|n)\s*:/.test(l));
  let out = kept.join("\n").replace(/\n+$/, "") + `\nschedule: ${schedule}`;
  if (schedule === "interval" && interval.trim()) out += `\ninterval: ${interval.trim()}`;
  if (schedule === "cron" && cron.trim()) out += `\ncron: ${cron.trim()}`;
  if (schedule === "parallel") out += `\nn: ${Math.max(2, n)}`;
  return out + "\n";
}

function withDriverPrompt(driver: string, prompt: string): string {
  const kept = driver.split("\n").filter((line) => !/^\s*prompt\s*:/.test(line));
  return kept.join("\n").replace(/\n+$/, "") + `\nprompt: ${JSON.stringify(prompt.trim())}\n`;
}

export function RunModal({
  initialPrompt,
  preset = "one-time",
  cadence,
  returnFocus,
}: {
  initialPrompt?: string;
  preset?: RunPreset;
  cadence?: CadenceSpec;
  returnFocus?: HTMLElement;
}) {
  const { api, clock } = useAppServices();
  const { openModal, select, selectRun, refreshRuns, refreshSessions, toast } = useStore();
  const { ws, setWs, ensure, choose } = useWorkspace();
  // SC-18 — the form OPENS on the cadence the caller already showed the user (a
  // Scheduled suggestion card's cron), not on the preset's generic `interval:
  // 5m`. Everything below stays editable: a prefilled cadence is a default, not
  // a decision. What it ends the era of is a launcher that quietly contradicted
  // the card that opened it.
  const formDefaults = runFormDefaults(preset, cadence);
  const [kind, setKind] = useState<"submit" | "drive">(formDefaults.kind);
  const [prompt, setPrompt] = useState(initialPrompt || "");
  const [mode, setMode] = useState("");
  const [idem, setIdem] = useState("");
  const [spec, setSpec] = useState("");
  const [driver, setDriver] = useState(DEFAULT_DRIVER);
  const [provider, setProvider] = useState(DEFAULT_MODEL.provider);
  const [model, setModel] = useState(DEFAULT_MODEL.id);
  const [effort, setEffort] = useState<EffortId>(DEFAULT_EFFORT);
  const [schedule, setSchedule] = useState<ScheduleKind>(formDefaults.schedule);
  const [interval, setInterval] = useState(formDefaults.interval);
  const [cron, setCron] = useState(formDefaults.cron);
  const [nAttempts, setNAttempts] = useState(formDefaults.n);
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);

  useEffect(() => {
    api
      .agents()
      .then((catalog) =>
        setSpec((current) =>
          current.trim()
            ? current
            : (
                catalog.find((entry) => entry.name === "dev") ||
                catalog[0]
              )?.yaml || "",
        ),
      )
      .catch((error: Error) => toast(error.message));
  }, []);

  const start = async () => {
    setBusy(true);
    try {
      const workspace = await ensure();
      const driverSpec = withSchedule(withDriverPrompt(driver, prompt), schedule, interval, cron, nAttempts);
      const r = await api.startRun({
        provider,
        model,
        effort,
        kind,
        spec: kind === "submit" ? spec : driverSpec,
        extraSpecs: [],
        prompt,
        workspace,
        mode,
        idem,
      });
      close();
      await refreshRuns();
      // A drive's run id (run1, run2…) belongs to this Web UI process and is
      // deliberately reused after restart. The daemon-assigned session id is
      // the durable route. Land there as soon as it appears; keep RunView only
      // as a short startup fallback while the first event is still arriving.
      if (kind === "drive") {
        for (let i = 0; i < 10; i++) {
          try {
            const sid = (await api.runs()).find((run) => run.id === r.runId)?.sessionId;
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
      }
      selectRun(r.runId);
    } catch (e: any) {
      toast(e.message, "error", e.details);
    } finally {
      setBusy(false);
    }
  };

  // Inline schedule validation (G36 余项): a mistyped cadence is caught next
  // to its field, and Start stays disabled until the cadence parses.
  const scheduleError =
    kind !== "drive"
      ? ""
      : schedule === "interval"
        ? scheduleFieldError("interval", interval) || (interval.trim() ? "" : " ")
        : schedule === "cron"
          ? scheduleFieldError("cron", cron) || (cron.trim() ? "" : " ")
          : "";
  const scheduleBlocked = scheduleError !== "";

  return (
    <Modal
      title={kind === "submit" ? "Start a run" : schedule === "immediate" ? "Set a goal" : schedule === "parallel" ? "Best of N" : "Schedule a run"}
      onClose={close}
      returnFocus={returnFocus}
      footer={
        <Button
          variant="solid"
          disabled={
            !prompt.trim() ||
            scheduleBlocked ||
            (kind === "submit" && !spec.trim())
          }
          loading={busy}
          onClick={start}
        >
          {kind === "submit" ? "Start run" : "Start schedule"}
        </Button>
      }
    >
      <label className="field">Run type</label>
      <div className="seg" role="group" aria-label="Run type">
        <button aria-pressed={kind === "submit"} className={kind === "submit" ? "on" : ""} onClick={() => setKind("submit")} title="one-shot run: a fresh session executes the prompt once">
          One-time
        </button>
        <button aria-pressed={kind === "drive"} className={kind === "drive" ? "on" : ""} onClick={() => setKind("drive")} title="scheduled work: child runs repeat on a goal / repeating / best-of-N schedule">
          Goal or repeating
        </button>
      </div>
      <label className="field" htmlFor="run-prompt">Prompt</label>
      <Textarea id="run-prompt" autoFocus rows={3} value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="Describe the outcome you want" />
      <label className="field" htmlFor="run-workspace">Workspace</label>
      <div className="row-flex">
        <Input id="run-workspace" type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="Leave blank for a new scratch workspace" />
        <Button variant="outline" onClick={choose}>Use folder…</Button>
      </div>
      <ModelFields
        provider={provider}
        model={model}
        effort={effort}
        onModel={(nextProvider, nextModel) => {
          setProvider(nextProvider);
          setModel(nextModel);
        }}
        onEffort={setEffort}
      />
      {kind === "submit" ? (
        <details className="advanced-settings">
          <summary>Advanced settings</summary>
          <div className="row-flex">
            <div style={{ flex: 1 }}>
              <label className="field" htmlFor="run-approval">Approval mode</label>
              <Select id="run-approval" value={mode} onChange={(e) => setMode(e.target.value)}>
                <option value="">From agent specification</option>
                <option value="plan">Plan (read-only)</option>
                <option value="acceptEdits">Auto-accept edits</option>
              </Select>
            </div>
            <div style={{ flex: 1 }}>
              <label className="field" htmlFor="run-idempotency">Idempotency key (optional)</label>
              <Input id="run-idempotency" type="text" value={idem} onChange={(e) => setIdem(e.target.value)} />
            </div>
          </div>
          <label className="field" htmlFor="run-agent-spec">Agent specification (YAML)</label>
          <Textarea id="run-agent-spec" code className="code" rows={9} value={spec} onChange={(e) => setSpec(e.target.value)} />
        </details>
      ) : (
        <>
          <label className="field" htmlFor="run-schedule">Schedule</label>
          <div className="row-flex">
            <Select id="run-schedule" value={schedule} onChange={(e) => setSchedule(e.target.value as ScheduleKind)} title="how iterations are paced">
              <option value="immediate">Goal — work until verified</option>
              <option value="interval">Repeat every…</option>
              <option value="cron">Cron schedule…</option>
              <option value="parallel">Best of N attempts</option>
            </Select>
            {schedule === "interval" && (
              <Input
                type="text"
                value={interval}
                onChange={(e) => setInterval(e.target.value)}
                placeholder="5m · 30s · 1h"
                title="Go duration between iterations"
                aria-label="Repeat interval"
              />
            )}
            {schedule === "cron" && (
              <Input
                type="text"
                value={cron}
                onChange={(e) => setCron(e.target.value)}
                placeholder="0 * * * * (min hr dom mon dow)"
                title="5-field cron expression"
                aria-label="Cron schedule"
              />
            )}
            {schedule === "parallel" && (
              <Input
                type="number"
                min={2}
                value={nAttempts}
                onChange={(e) => setNAttempts(Math.max(2, Number(e.target.value) || 2))}
                title="how many isolated attempts to run; the verifiers judge the best"
                aria-label="Number of attempts"
              />
            )}
          </div>
          {scheduleError.trim() !== "" && (
            <div className="mt-1 text-[12px] leading-5 text-red" role="alert">
              {scheduleError}
            </div>
          )}
          {/* SC-18 — the rhythm, in the words the Scheduled row will use for it.
              `0 8 * * 1-5` is not something anyone can proofread; the phrase
              rendered from it is. Same renderer as the suggestion cards and the
              same dialect the server reads back, so what you see here is what the
              row will say tomorrow. Only for the two paced kinds — a goal and a
              best-of-N have no cadence to misread. */}
          {(schedule === "cron" || schedule === "interval") && (
            <div className="dim" data-testid="cadence-echo" style={{ marginTop: 6 }}>
              {cadenceText({ schedule, interval, cron })}
            </div>
          )}
          <details className="advanced-settings">
            <summary>Advanced settings</summary>
            <label className="field" htmlFor="run-driver-spec">Driver specification (YAML)</label>
            <Textarea id="run-driver-spec" code className="code" rows={8} value={driver} onChange={(e) => setDriver(e.target.value)} />
          </details>
        </>
      )}
    </Modal>
  );
}

// barrierLabel turns a raw barrier id (bar-final / bar-t3 / bar-m75) into a
// readable "fork point" the way Codex names a fork spot, so the picker isn't a
// list of cryptic ids.
function barrierLabel(b: string): string {
  if (b === "bar-final") return "Latest — end of the conversation";
  const t = b.match(/^bar-t(\d+)$/);
  if (t) return `After agent step ${t[1]}`;
  const m = b.match(/^bar-m(\d+)$/);
  if (m) return `Manual checkpoint (seq ${m[1]})`;
  return b;
}

// forkRank sorts fork points latest-first: end of conversation, then turns
// descending, then the rest.
function forkRank(b: string): number {
  if (b === "bar-final") return -1;
  const t = b.match(/^bar-t(\d+)$/);
  if (t) return 1000 - Number(t[1]);
  return 2000;
}

export function ForkModal({ sid }: { sid: string }) {
  const { api, clock, storage } = useAppServices();
  const { openModal, select, refreshSessions, toast } = useStore();
  const { ws, setWs } = useWorkspace();
  const [barriers, setBarriers] = useState<string[]>([]);
  const [barrier, setBarrier] = useState("");
  const [busy, setBusy] = useState(false);
  const [showEarlier, setShowEarlier] = useState(false);
  const close = () => openModal(null);

  const loadBarriers = () => {
    api.barriers(sid)
      .then((b) => {
        const sorted = [...b].sort((x, y) => forkRank(x) - forkRank(y));
        setBarriers(sorted);
        setBarrier((cur) => (cur && sorted.includes(cur) ? cur : sorted[0] || ""));
      })
      .catch((e) => toast(e.message));
  };
  // Checkpoints keep landing while the modal is open (QA Round2 F-F1: the
  // one-shot fetch went stale and forced a full page reload) — poll gently.
  useEffect(() => {
    loadBarriers();
    const t = clock.setInterval(loadBarriers, 3000);
    return () => clock.clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sid]);

  const createCheckpoint = async () => {
    setBusy(true);
    try {
      await api.barrier(sid);
      toast("checkpoint created", "info");
      loadBarriers();
    } catch (error: any) {
      toast(error.message);
    } finally {
      setBusy(false);
    }
  };

  const doFork = async () => {
    if (!barrier) return;
    setBusy(true);
    try {
      const r = await api.fork(sid, barrier, ws.trim());
      close();
      await refreshSessions();
      if (r.sid) {
        // The fork runs under the SOURCE session's spec: carry the
        // remembered approval posture over so its composer pill reports
        // the truth (QA Round1 F-C3).
        const acc = recallAccess(sid, storage.local);
        if (acc) rememberAccess(r.sid, acc, storage.local);
        const spec = recallSpec(sid, storage.local);
        if (spec) rememberSpec(r.sid, spec, storage.local);
        select(r.sid);
      }
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title="Continue in new session"
      onClose={close}
      footer={
        <Button variant="solid" disabled={!barrier} loading={busy} onClick={doFork}>
          Continue
        </Button>
      }
    >
      <div className="dim" style={{ marginBottom: 10 }}>
        Starts a new session from a checkpoint of this one, in its own git worktree.
        This session stays unchanged.
      </div>
      <label className="field" htmlFor="fork-checkpoint">Continue from</label>
      {barriers.length === 0 ? (
        <div className="fork-empty">
          <span>No checkpoints yet. Create one now, then continue from this exact point.</span>
          <Button variant="outline" loading={busy} onClick={createCheckpoint}>Create checkpoint</Button>
        </div>
      ) : (
        <>
          {!showEarlier && (
            <div className="fork-latest">
              <strong>{barrierLabel(barrier)}</strong>
              {barriers.length > 1 && (
                <Button variant="outline" onClick={() => setShowEarlier(true)}>
                  Choose an earlier checkpoint
                </Button>
              )}
            </div>
          )}
          {showEarlier && (
            <>
              <Select id="fork-checkpoint" value={barrier} onChange={(e) => setBarrier(e.target.value)} title="the checkpoint to branch the new session from">
                {barriers.map((b) => (
                  <option key={b} value={b}>
                    {barrierLabel(b)}
                  </option>
                ))}
              </Select>
              <div className="dim mt-2">
                Agent steps are internal model/tool checkpoints, not conversation turns.
              </div>
            </>
          )}
        </>
      )}
      <details className="advanced-settings">
        <summary>Advanced settings</summary>
        <label className="field" htmlFor="fork-worktree">Worktree directory (optional)</label>
        <Input id="fork-worktree" type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="Automatically create a worktree" />
      </details>
    </Modal>
  );
}

export function AgentModal({
  sid,
  provider: initialProvider,
  model: initialModel,
  effort: initialEffort,
}: {
  sid: string;
  provider?: string;
  model?: string;
  effort?: string;
}) {
  const { api, storage } = useAppServices();
  const { openModal, toast } = useStore();
  const rawRememberedSpec = recallSpec(sid, storage.local) || "";
  const remembered =
    recallModel(sid, storage.local) ||
    legacyModelFromSpec(rawRememberedSpec);
  const [spec, setSpec] = useState(() =>
    stripLegacyModel(rawRememberedSpec));
  const [worker, setWorker] = useState("");
  const [provider, setProvider] = useState(
    initialProvider || remembered?.provider || DEFAULT_MODEL.provider,
  );
  const [model, setModel] = useState(
    initialModel || remembered?.model || DEFAULT_MODEL.id,
  );
  const [effort, setEffort] = useState<EffortId>(
    EFFORT_LEVELS.some(
      (level) => level.id === (initialEffort || remembered?.effort),
    )
      ? ((initialEffort || remembered?.effort) as EffortId)
      : DEFAULT_EFFORT,
  );
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);

  useEffect(() => {
    if (spec.trim()) return;
    api
      .agents()
      .then((catalog) =>
        setSpec((current) =>
          current.trim()
            ? current
            : (
                catalog.find((entry) => entry.name === "dev") ||
                catalog[0]
              )?.yaml || "",
        ),
      )
      .catch((error: Error) => toast(error.message));
  }, []);

  const swap = async () => {
    setBusy(true);
    try {
      const extraSpecs: SpecFile[] = [];
      if (worker.trim()) extraSpecs.push({ name: "worker.yaml", content: worker });
      await api.switchAgent(sid, spec, extraSpecs, {
        provider,
        model,
        effort,
      });
      rememberSpec(sid, spec, storage.local);
      rememberModel(sid, { provider, model, effort }, storage.local);
      close();
      toast("agent spec switched", "info");
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title={`Switch agent · ${sid}`}
      onClose={close}
      footer={
        <Button
          variant="solid"
          disabled={!spec.trim()}
          loading={busy}
          onClick={swap}
        >
          Switch
        </Button>
      }
    >
      <div className="dim">Same session, context carries over; takes effect at the next safe boundary (decision #32) and lands in the journal as spec_changed. The new spec is written to runtime/specs.</div>
      <ModelFields
        provider={provider}
        model={model}
        effort={effort}
        onModel={(nextProvider, nextModel) => {
          setProvider(nextProvider);
          setModel(nextModel);
        }}
        onEffort={setEffort}
      />
      <label className="field" htmlFor="agent-base-spec">base.yaml (new main agent spec)</label>
      <Textarea id="agent-base-spec" code className="code" rows={12} value={spec} onChange={(e) => setSpec(e.target.value)} />
      <label className="field" htmlFor="agent-worker-spec">worker.yaml (sibling sub-agent spec; leave empty to skip)</label>
      <Textarea id="agent-worker-spec" code className="code" rows={6} value={worker} onChange={(e) => setWorker(e.target.value)} />
    </Modal>
  );
}

export function TrustModal() {
  const { api } = useAppServices();
  const { openModal, toast } = useStore();
  const [dir, setDir] = useState("");
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);
  const go = async () => {
    setBusy(true);
    try {
      await api.trust(dir.trim());
      close();
      toast("trusted: " + dir, "info");
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      title="Trust workspace directory"
      onClose={close}
      footer={
        <Button
          variant="solid"
          disabled={!dir.trim()}
          loading={busy}
          onClick={go}
          title="enable project hooks/settings for sessions in this directory (ar trust)"
        >
          Trust directory
        </Button>
      }
    >
      <label className="field" htmlFor="trust-directory">directory (absolute path)</label>
      <Input id="trust-directory" type="text" value={dir} onChange={(e) => setDir(e.target.value)} placeholder="/path/to/workspace" />
    </Modal>
  );
}

export function RenameModal({ sid }: { sid: string }) {
  const { openModal, sessions, renames, setRename, toast } = useStore();
  const raw = sessions.find((s) => s.id === sid)?.title;
  const [name, setName] = useState(() => displayTitle(renames, sid, raw));
  const close = () => openModal(null);
  const save = () => {
    if (!name.trim()) {
      close();
      return; // titles are journal facts now — blank input is a no-op
    }
    setRename(sid, name);
    close();
    toast("renamed", "info");
  };
  return (
    <Modal
      title="Rename session"
      onClose={close}
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button variant="solid" onClick={save}>
            Save
          </Button>
        </>
      }
    >
      <label className="field" htmlFor="rename-session">Keep it short and recognizable</label>
      <Input
        id="rename-session"
        type="text"
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        onFocus={(e) => e.target.select()}
        onKeyDown={(e) => {
          if (e.key === "Enter") save();
          else if (e.key === "Escape") close();
        }}
        placeholder="Session name"
      />
    </Modal>
  );
}

export function ViewerModal({ title, body }: { title: string; body: string }) {
  const { openModal } = useStore();
  return (
    <Modal title={title} onClose={() => openModal(null)}>
      <pre style={{ fontFamily: "var(--mono)", fontSize: 12, whiteSpace: "pre-wrap", wordBreak: "break-all", margin: 0 }}>
        {body}
      </pre>
    </Modal>
  );
}

export function RunDetailsModal({ data, status }: { data: unknown; status?: string }) {
  const { openModal } = useStore();
  const summary = summarizeInspect(data, status);
  const displayStatus = friendlyStatus(summary.status.text);
  return (
    <Modal title="Run details" onClose={() => openModal(null)}>
      <div className="run-details">
        <div className="rd-hero">
          <div>
            <span className="rd-kicker">Current run</span>
            <strong>{summary.spec}</strong>
          </div>
          <span className={`pill ${summary.status.cls || displayStatus.cls}`}>{summary.status.text}</span>
        </div>

        {summary.waiting && (
          <section className="rd-attention" aria-label="Waiting for you">
            <b>{summary.waiting.title}</b>
            <span>{summary.waiting.subject}</span>
          </section>
        )}

        <section className="rd-section">
          <h3>Overview</h3>
          <dl className="rd-grid">
            <div><dt>Model</dt><dd>{summary.model}</dd></div>
            <div><dt>Access</dt><dd>{summary.mode}</dd></div>
            <div><dt>Turns</dt><dd>{summary.turns}</dd></div>
            <div><dt>Steps</dt><dd>{summary.steps}</dd></div>
            <div><dt>Subagents</dt><dd>{summary.agents}</dd></div>
            <div><dt>Provider</dt><dd>{summary.provider || "Not reported"}</dd></div>
          </dl>
        </section>

        <section className="rd-section">
          <h3>Usage</h3>
          <div className="rd-metrics">
            <div><strong>{compactCount(summary.usage.billed)}</strong><span>Billed tokens</span></div>
            <div><strong>{compactCount(summary.usage.input)}</strong><span>Input</span></div>
            <div><strong>{compactCount(summary.usage.output)}</strong><span>Output</span></div>
          </div>
        </section>

        <section className="rd-section">
          <h3>Activity</h3>
          <p className="rd-summary">{summary.activity.modelCalls} model calls · {summary.activity.toolCalls} tool calls{summary.activity.blocked ? ` · ${summary.activity.blocked} blocked` : ""}</p>
          {summary.activity.recentTools.length > 0 && (
            <div className="rd-tools">
              {summary.activity.recentTools.map((tool, index) => (
                <div className={tool.blocked ? "blocked" : ""} key={`${tool.name}-${index}`}>
                  <span>{tool.name}</span>
                  <code>{tool.detail || (tool.blocked ? "Blocked by policy" : "Completed")}</code>
                </div>
              ))}
            </div>
          )}
        </section>

        {(summary.modalities.length > 0 || summary.capabilities.length > 0) && (
          <section className="rd-section">
            <h3>Provider capabilities</h3>
            <div className="rd-tags">
              {dedupeCaps([...summary.modalities, ...summary.capabilities]).map((label) => <span key={label}>{label}</span>)}
            </div>
          </section>
        )}

        <details className="rd-raw">
          <summary>Raw run data</summary>
          <pre tabIndex={0} aria-label="Raw run data contents">
            {JSON.stringify(data, null, 2)}
          </pre>
        </details>
      </div>
    </Modal>
  );
}

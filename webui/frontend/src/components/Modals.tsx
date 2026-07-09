import { useEffect, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import type { SpecFile } from "../types";
import { DEFAULT_DRIVER, DEFAULT_DRIVER_AGENT, DEFAULT_SPEC, DEFAULT_WORKER } from "../specs";

function Modal({
  title,
  onClose,
  children,
  footer,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  footer?: React.ReactNode;
}) {
  return (
    <div className="backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <div className="mhead">
          <span>{title}</span>
          <button className="ghost" onClick={onClose}>
            ✕
          </button>
        </div>
        <div className="mbody">{children}</div>
        {footer && <div className="mfoot">{footer}</div>}
      </div>
    </div>
  );
}

export function Modals() {
  const { modal } = useStore();
  if (!modal) return null;
  switch (modal.kind) {
    case "new":
      return <NewSessionModal initialMessage={modal.message} />;
    case "run":
      return <RunModal initialTask={modal.task} />;
    case "fork":
      return <ForkModal sid={modal.sid} />;
    case "agent":
      return <AgentModal sid={modal.sid} />;
    case "trust":
      return <TrustModal />;
    case "viewer":
      return <ViewerModal title={modal.title} body={modal.body} />;
  }
}

function useWorkspace() {
  const { toast } = useStore();
  const [ws, setWs] = useState("");
  const mk = async () => {
    try {
      setWs((await AR.makeWorkspace()).path);
    } catch (e: any) {
      toast(e.message);
    }
  };
  return { ws, setWs, mk };
}

function NewSessionModal({ initialMessage }: { initialMessage?: string }) {
  const { openModal, select, refreshSessions, toast } = useStore();
  const { ws, setWs, mk } = useWorkspace();
  const [msg, setMsg] = useState(initialMessage || "Hello — in one sentence, introduce your tool capabilities.");
  const [mode, setMode] = useState("");
  const [spec, setSpec] = useState(DEFAULT_SPEC);
  const [worker, setWorker] = useState(DEFAULT_WORKER);
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);

  const create = async () => {
    setBusy(true);
    try {
      const extraSpecs: SpecFile[] = [];
      if (worker.trim()) extraSpecs.push({ name: "worker.yaml", content: worker });
      const r = await AR.newSession({ spec, extraSpecs, workspace: ws.trim(), message: msg.trim(), mode });
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
      title="New session (chat)"
      onClose={close}
      footer={
        <>
          <button onClick={() => openModal({ kind: "run" })}>Run in background instead (submit/drive)…</button>
          <button className="primary" disabled={busy} onClick={create}>
            Create
          </button>
        </>
      }
    >
      <label className="field">workspace directory (absolute path)</label>
      <div className="row-flex">
        <input type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="/path/to/workspace" />
        <button style={{ whiteSpace: "nowrap" }} onClick={mk} title="create a fresh empty directory under runtime/ and fill it in here">
          make empty workspace
        </button>
      </div>
      <label className="field">opening message</label>
      <textarea rows={2} value={msg} onChange={(e) => setMsg(e.target.value)} />
      <label className="field">mode</label>
      <select value={mode} onChange={(e) => setMode(e.target.value)} title="permission mode: default asks for approval on gated tools · plan = read-only planning · acceptEdits auto-approves file edits">
        <option value="">default</option>
        <option value="plan">plan</option>
        <option value="acceptEdits">acceptEdits</option>
      </select>
      <label className="field">base.yaml (main agent spec)</label>
      <textarea className="code" rows={11} value={spec} onChange={(e) => setSpec(e.target.value)} />
      <label className="field">worker.yaml (sibling sub-agent spec; leave empty to skip)</label>
      <textarea className="code" rows={6} value={worker} onChange={(e) => setWorker(e.target.value)} />
    </Modal>
  );
}

function RunModal({ initialTask }: { initialTask?: string }) {
  const { openModal, selectRun, refreshRuns, toast } = useStore();
  const { ws, setWs, mk } = useWorkspace();
  const [kind, setKind] = useState<"submit" | "drive">("submit");
  const [task, setTask] = useState(initialTask || "Create hello.txt containing hi in the workspace, then confirm.");
  const [mode, setMode] = useState("");
  const [idem, setIdem] = useState("");
  const [spec, setSpec] = useState(DEFAULT_SPEC);
  const [driver, setDriver] = useState(DEFAULT_DRIVER);
  const [driverAgent, setDriverAgent] = useState(DEFAULT_DRIVER_AGENT);
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);

  const start = async () => {
    setBusy(true);
    try {
      const r = await AR.startRun({
        kind,
        spec: kind === "submit" ? spec : driver,
        // drive needs the child agent spec as an agent.yaml sibling (driver's
        // agent_spec field points at it); submit needs no sibling.
        extraSpecs: kind === "drive" ? [{ name: "agent.yaml", content: driverAgent }] : [],
        task,
        workspace: ws.trim(),
        mode,
        idem,
      });
      close();
      await refreshRuns();
      selectRun(r.runId);
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title="Background run"
      onClose={close}
      footer={
        <button className="primary" disabled={busy} onClick={start}>
          Start {kind}
        </button>
      }
    >
      <label className="field">kind</label>
      <div className="seg">
        <button className={kind === "submit" ? "on" : ""} onClick={() => setKind("submit")} title="one-shot task: a fresh session runs the task once and completes">
          submit (one-shot task)
        </button>
        <button className={kind === "drive" ? "on" : ""} onClick={() => setKind("drive")} title="iterative driver: child runs repeat per driver.yaml (goal / loop / best-of-N)">
          drive (iterative driver)
        </button>
      </div>
      <label className="field">workspace directory (absolute path)</label>
      <div className="row-flex">
        <input type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="/path/to/workspace" />
        <button style={{ whiteSpace: "nowrap" }} onClick={mk} title="create a fresh empty directory under runtime/ and fill it in here">
          make empty workspace
        </button>
      </div>
      {kind === "submit" ? (
        <>
          <label className="field">task</label>
          <textarea rows={2} value={task} onChange={(e) => setTask(e.target.value)} />
          <div className="row-flex">
            <div style={{ flex: 1 }}>
              <label className="field">mode</label>
              <select value={mode} onChange={(e) => setMode(e.target.value)} title="permission mode: default asks for approval on gated tools · plan = read-only planning · acceptEdits auto-approves file edits">
                <option value="">default</option>
                <option value="plan">plan</option>
                <option value="acceptEdits">acceptEdits</option>
              </select>
            </div>
            <div style={{ flex: 1 }}>
              <label className="field">idem key (optional)</label>
              <input type="text" value={idem} onChange={(e) => setIdem(e.target.value)} title="idempotency key: resubmitting with the same key reuses the run instead of starting a new one" />
            </div>
          </div>
          <label className="field">spec.yaml</label>
          <textarea className="code" rows={10} value={spec} onChange={(e) => setSpec(e.target.value)} />
        </>
      ) : (
        <>
          <label className="field">driver.yaml (iterative driver spec: agent_spec / task / verifiers)</label>
          <textarea className="code" rows={8} value={driver} onChange={(e) => setDriver(e.target.value)} />
          <label className="field">agent.yaml (the sub-agent each iteration runs; referenced by the driver's agent_spec)</label>
          <textarea
            className="code"
            rows={7}
            value={driverAgent}
            onChange={(e) => setDriverAgent(e.target.value)}
          />
        </>
      )}
    </Modal>
  );
}

function ForkModal({ sid }: { sid: string }) {
  const { openModal, select, refreshSessions, toast } = useStore();
  const { ws, setWs, mk } = useWorkspace();
  const [barriers, setBarriers] = useState<string[]>([]);
  const [barrier, setBarrier] = useState("");
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);

  useEffect(() => {
    AR.barriers(sid)
      .then((b) => {
        setBarriers(b);
        if (b.length) setBarrier(b[0]);
      })
      .catch((e) => toast(e.message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sid]);

  const doFork = async () => {
    if (!barrier) return;
    setBusy(true);
    try {
      const r = await AR.fork(sid, barrier, ws.trim());
      close();
      await refreshSessions();
      if (r.sid) select(r.sid);
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title={`Fork session · ${sid}`}
      onClose={close}
      footer={
        <button className="primary" disabled={busy || !barrier} onClick={doFork}>
          fork
        </button>
      }
    >
      <label className="field">barrier (fork point)</label>
      {barriers.length === 0 ? (
        <div className="dim">No barriers in this session yet (have the agent drop one with the barrier tool, then fork).</div>
      ) : (
        <select value={barrier} onChange={(e) => setBarrier(e.target.value)} title="the checkpoint to branch the new session from">
          {barriers.map((b) => (
            <option key={b} value={b}>
              {b}
            </option>
          ))}
        </select>
      )}
      <label className="field">fork workspace (optional; leave empty for auto)</label>
      <div className="row-flex">
        <input type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="empty = auto <ws>-fork-<id>" />
        <button style={{ whiteSpace: "nowrap" }} onClick={mk} title="create a fresh empty directory under runtime/ and fill it in here">
          make empty workspace
        </button>
      </div>
    </Modal>
  );
}

function AgentModal({ sid }: { sid: string }) {
  const { openModal, toast } = useStore();
  const [spec, setSpec] = useState(DEFAULT_SPEC);
  const [worker, setWorker] = useState(DEFAULT_WORKER);
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);

  const swap = async () => {
    setBusy(true);
    try {
      const extraSpecs: SpecFile[] = [];
      if (worker.trim()) extraSpecs.push({ name: "worker.yaml", content: worker });
      await AR.switchAgent(sid, spec, extraSpecs);
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
        <button className="primary" disabled={busy} onClick={swap}>
          Switch
        </button>
      }
    >
      <div className="dim">Same session, context carries over; takes effect at the next safe boundary (decision #32) and lands in the journal as spec_changed. The new spec is written to runtime/specs.</div>
      <label className="field">base.yaml (new main agent spec)</label>
      <textarea className="code" rows={12} value={spec} onChange={(e) => setSpec(e.target.value)} />
      <label className="field">worker.yaml (sibling sub-agent spec; leave empty to skip)</label>
      <textarea className="code" rows={6} value={worker} onChange={(e) => setWorker(e.target.value)} />
    </Modal>
  );
}

function TrustModal() {
  const { openModal, toast } = useStore();
  const [dir, setDir] = useState("");
  const [busy, setBusy] = useState(false);
  const close = () => openModal(null);
  const go = async () => {
    setBusy(true);
    try {
      await AR.trust(dir.trim());
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
        <button className="primary" disabled={busy || !dir.trim()} onClick={go} title="enable project hooks/settings for sessions in this directory (ar trust)">
          trust
        </button>
      }
    >
      <label className="field">directory (absolute path)</label>
      <input type="text" value={dir} onChange={(e) => setDir(e.target.value)} placeholder="/path/to/workspace" />
    </Modal>
  );
}

function ViewerModal({ title, body }: { title: string; body: string }) {
  const { openModal } = useStore();
  return (
    <Modal title={title} onClose={() => openModal(null)}>
      <pre style={{ fontFamily: "var(--mono)", fontSize: 12, whiteSpace: "pre-wrap", wordBreak: "break-all", margin: 0 }}>
        {body}
      </pre>
    </Modal>
  );
}

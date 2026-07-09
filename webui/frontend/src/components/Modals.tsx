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
  const [msg, setMsg] = useState(initialMessage || "你好,请先用一句话自我介绍你的工具能力。");
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
      title="新会话 (chat)"
      onClose={close}
      footer={
        <>
          <button onClick={() => openModal({ kind: "run" })}>改为后台运行 (submit/drive)…</button>
          <button className="primary" disabled={busy} onClick={create}>
            创建
          </button>
        </>
      }
    >
      <label className="field">workspace 目录(绝对路径)</label>
      <div className="row-flex">
        <input type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="/path/to/workspace" />
        <button style={{ whiteSpace: "nowrap" }} onClick={mk}>
          造空 workspace
        </button>
      </div>
      <label className="field">开场消息</label>
      <textarea rows={2} value={msg} onChange={(e) => setMsg(e.target.value)} />
      <label className="field">mode</label>
      <select value={mode} onChange={(e) => setMode(e.target.value)}>
        <option value="">default</option>
        <option value="plan">plan</option>
        <option value="acceptEdits">acceptEdits</option>
      </select>
      <label className="field">base.yaml(主 agent spec)</label>
      <textarea className="code" rows={11} value={spec} onChange={(e) => setSpec(e.target.value)} />
      <label className="field">worker.yaml(子 agent spec,留空则不写)</label>
      <textarea className="code" rows={6} value={worker} onChange={(e) => setWorker(e.target.value)} />
    </Modal>
  );
}

function RunModal({ initialTask }: { initialTask?: string }) {
  const { openModal, selectRun, refreshRuns, toast } = useStore();
  const { ws, setWs, mk } = useWorkspace();
  const [kind, setKind] = useState<"submit" | "drive">("submit");
  const [task, setTask] = useState(initialTask || "在 workspace 里创建 hello.txt 写入 hi,然后确认。");
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
      title="后台运行"
      onClose={close}
      footer={
        <button className="primary" disabled={busy} onClick={start}>
          启动 {kind}
        </button>
      }
    >
      <label className="field">类型</label>
      <div className="seg">
        <button className={kind === "submit" ? "on" : ""} onClick={() => setKind("submit")}>
          submit (一次性任务)
        </button>
        <button className={kind === "drive" ? "on" : ""} onClick={() => setKind("drive")}>
          drive (迭代驱动)
        </button>
      </div>
      <label className="field">workspace 目录(绝对路径)</label>
      <div className="row-flex">
        <input type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="/path/to/workspace" />
        <button style={{ whiteSpace: "nowrap" }} onClick={mk}>
          造空 workspace
        </button>
      </div>
      {kind === "submit" ? (
        <>
          <label className="field">任务描述</label>
          <textarea rows={2} value={task} onChange={(e) => setTask(e.target.value)} />
          <div className="row-flex">
            <div style={{ flex: 1 }}>
              <label className="field">mode</label>
              <select value={mode} onChange={(e) => setMode(e.target.value)}>
                <option value="">default</option>
                <option value="plan">plan</option>
                <option value="acceptEdits">acceptEdits</option>
              </select>
            </div>
            <div style={{ flex: 1 }}>
              <label className="field">idem key(可选)</label>
              <input type="text" value={idem} onChange={(e) => setIdem(e.target.value)} />
            </div>
          </div>
          <label className="field">spec.yaml</label>
          <textarea className="code" rows={10} value={spec} onChange={(e) => setSpec(e.target.value)} />
        </>
      ) : (
        <>
          <label className="field">driver.yaml(迭代驱动 spec:agent_spec / task / verifiers)</label>
          <textarea className="code" rows={8} value={driver} onChange={(e) => setDriver(e.target.value)} />
          <label className="field">agent.yaml(每轮迭代跑的子 agent,被 driver 的 agent_spec 引用)</label>
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
      title={`fork 会话 · ${sid}`}
      onClose={close}
      footer={
        <button className="primary" disabled={busy || !barrier} onClick={doFork}>
          fork
        </button>
      }
    >
      <label className="field">barrier(分叉点)</label>
      {barriers.length === 0 ? (
        <div className="dim">该会话还没有 barrier(用 barrier 工具落一个后再来 fork)。</div>
      ) : (
        <select value={barrier} onChange={(e) => setBarrier(e.target.value)}>
          {barriers.map((b) => (
            <option key={b} value={b}>
              {b}
            </option>
          ))}
        </select>
      )}
      <label className="field">fork workspace(可选,留空自动)</label>
      <div className="row-flex">
        <input type="text" value={ws} onChange={(e) => setWs(e.target.value)} placeholder="留空 = 自动 <ws>-fork-<id>" />
        <button style={{ whiteSpace: "nowrap" }} onClick={mk}>
          造空 workspace
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
      toast("已切换 agent spec", "info");
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title={`切换 agent · ${sid}`}
      onClose={close}
      footer={
        <button className="primary" disabled={busy} onClick={swap}>
          切换
        </button>
      }
    >
      <div className="dim">在下一个安全边界生效(决策 #32)。新 spec 会写入 runtime/specs。</div>
      <label className="field">base.yaml(新主 agent spec)</label>
      <textarea className="code" rows={12} value={spec} onChange={(e) => setSpec(e.target.value)} />
      <label className="field">worker.yaml(子 agent spec,留空则不写)</label>
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
      toast("已信任: " + dir, "info");
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      title="信任 workspace 目录"
      onClose={close}
      footer={
        <button className="primary" disabled={busy || !dir.trim()} onClick={go}>
          trust
        </button>
      }
    >
      <label className="field">目录(绝对路径)</label>
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

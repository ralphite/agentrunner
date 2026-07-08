import { useState } from "react";
import type { ApprovalRef } from "../timeline";

function pretty(raw: any): string {
  if (raw == null) return "";
  try {
    return JSON.stringify(typeof raw === "string" ? JSON.parse(raw) : raw, null, 2);
  } catch {
    return String(raw);
  }
}

export function ApprovalCard({
  approval,
  readonly,
  onDecide,
  onError,
}: {
  approval: ApprovalRef & { agent?: string; viaSSE?: boolean };
  readonly: boolean;
  onDecide: (id: string, decision: "approve" | "deny", reason: string) => Promise<void>;
  onError: (msg: string) => void;
}) {
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);

  const decide = async (d: "approve" | "deny") => {
    setBusy(true);
    try {
      await onDecide(approval.id, d, reason);
    } catch (e: any) {
      onError(e.message);
      setBusy(false);
    }
  };

  return (
    <div className="approval-card">
      <div className="head">
        <span>⚖ 审批 · {approval.tool}</span>
        {approval.agent && <span className="badge">请求方: {approval.agent}</span>}
        {approval.gates.map((g, i) => (
          <span className="badge" key={i}>
            {g.gate}:{g.decision}
            {g.reason ? " " + g.reason : ""}
          </span>
        ))}
      </div>
      {approval.args !== undefined && approval.args !== null && <pre>{pretty(approval.args)}</pre>}
      <div className="act">
        {readonly ? (
          <span className="badge">请在父会话中审批</span>
        ) : (
          <>
            <button className="primary sm" disabled={busy} onClick={() => decide("approve")}>
              批准
            </button>
            <button className="danger sm" disabled={busy} onClick={() => decide("deny")}>
              拒绝
            </button>
            <input placeholder="理由(可选)" value={reason} onChange={(e) => setReason(e.target.value)} />
          </>
        )}
      </div>
    </div>
  );
}

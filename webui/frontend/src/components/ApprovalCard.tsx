import { useMemo, useState } from "react";
import { CaretRight, ShieldCheck, TerminalWindow, WarningCircle } from "@phosphor-icons/react";
import type { ApprovalRef } from "../timeline";
import { describeApproval } from "../approvalPresentation";

function pretty(raw: unknown): string {
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
  workspace,
  onDecide,
  onError,
}: {
  approval: ApprovalRef & { agent?: string; viaSSE?: boolean };
  readonly: boolean;
  // The session's workspace path — shown so you know WHERE the command will
  // run before approving it (W25).
  workspace?: string;
  onDecide: (id: string, decision: "approve" | "deny", reason: string, always?: boolean) => Promise<void>;
  onError: (msg: string) => void;
}) {
  const [reason, setReason] = useState("");
  const [denying, setDenying] = useState(false);
  const [busy, setBusy] = useState(false);
  const presentation = useMemo(() => describeApproval(approval.tool, approval.args), [approval.tool, approval.args]);

  const decide = async (decision: "approve" | "deny", always = false) => {
    setBusy(true);
    try {
      await onDecide(approval.id, decision, reason.trim(), always);
    } catch (error: any) {
      onError(error.message);
      setBusy(false);
    }
  };

  return (
    <section className="approval-card" aria-label="Approval required">
      <div className="approval-heading">
        <span className="approval-icon"><ShieldCheck size={18} weight="fill" /></span>
        <div>
          <span className="approval-kicker">Approval required</span>
          <h3>{presentation.title}</h3>
        </div>
        {approval.agent && <span className="approval-agent">Requested by {approval.agent}</span>}
      </div>

      <p className="approval-description">{presentation.description}</p>
      <div className="approval-subject">
        <TerminalWindow size={15} />
        <code>{presentation.subject}</code>
      </div>
      <div className="approval-scope" title={workspace || undefined}>
        <WarningCircle size={14} /> {presentation.scope}
        {workspace && presentation.scope === "Current workspace" && <code className="approval-ws">{workspace}</code>}
      </div>

      <details className="approval-details">
        <summary><CaretRight size={12} /> Details</summary>
        <pre>{pretty(approval.args)}</pre>
        {approval.gates.length > 0 && (
          <div className="approval-gates">
            {approval.gates.map((gate, index) => (
              <span key={index}>{gate.gate}: {gate.decision}{gate.reason ? ` · ${gate.reason}` : ""}</span>
            ))}
          </div>
        )}
      </details>

      {readonly ? (
        <div className="approval-readonly">Review this request in the parent task.</div>
      ) : (
        <div className="approval-actions">
          {denying ? (
            <div className="deny-reason">
              <input autoFocus value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Reason (optional)" />
              <button disabled={busy} onClick={() => setDenying(false)}>Cancel</button>
              <button className="danger" disabled={busy} onClick={() => decide("deny")}>Deny</button>
            </div>
          ) : (
            <>
              <span className="approval-shortcut">⌘↵ approve · ⌘⌫ deny</span>
              <button disabled={busy} onClick={() => setDenying(true)}>Deny</button>
              <button
                disabled={busy}
                title="Approve AND save an exact allow rule to your user config, so this same call never asks again (any session)"
                onClick={() => decide("approve", true)}
              >
                Always allow
              </button>
              <button className="primary" disabled={busy} onClick={() => decide("approve")}>Approve once</button>
            </>
          )}
        </div>
      )}
    </section>
  );
}

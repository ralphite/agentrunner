import { useMemo, useState } from "react";
import { CaretRight, ShieldCheck, TerminalWindow, WarningCircle } from "@phosphor-icons/react";
import type { ApprovalRef } from "../timeline";
import { compactWorkspaceName, describeApproval } from "../approvalPresentation";
import { modLabel } from "../shortcuts";

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
  // The session's workspace path — represented compactly so you know WHERE
  // the command will run before approving it (W25).
  workspace?: string;
  onDecide: (id: string, decision: "approve" | "deny", reason: string, always?: boolean) => Promise<void>;
  onError: (msg: string) => void;
}) {
  const [reason, setReason] = useState("");
  const [denying, setDenying] = useState(false);
  const [busy, setBusy] = useState(false);
  const presentation = useMemo(() => describeApproval(approval.tool, approval.args), [approval.tool, approval.args]);
  const workspaceName = useMemo(() => compactWorkspaceName(workspace), [workspace]);

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
    <section className="approval-card min-w-0 overflow-hidden rounded-[8px] shadow-none" aria-label="Approval required">
      <div className="approval-heading min-w-0 flex-wrap gap-2">
        <span className="approval-icon mt-0.5"><ShieldCheck size={15} weight="duotone" /></span>
        <div className="min-w-0 flex-1">
          <span className="approval-kicker block">Approval required</span>
          <h3 className="m-0 mt-0.5 text-[13px] font-medium leading-[1.4]">{presentation.title}</h3>
        </div>
        {approval.agent && (
          <span className="approval-agent max-w-[45%] shrink-0 text-right [overflow-wrap:anywhere]">
            Requested by {approval.agent}
          </span>
        )}
      </div>

      <p className="approval-description my-2 leading-[1.4]">{presentation.description}</p>
      <div className="approval-subject flex min-w-0 items-start gap-2 overflow-hidden">
        <TerminalWindow className="mt-px shrink-0" size={15} />
        <code className="min-w-0 flex-1 whitespace-pre-wrap leading-[1.4] [overflow-wrap:anywhere]">
          {presentation.subject}
        </code>
      </div>
      <div
        className="approval-scope mt-2 flex min-w-0 flex-wrap items-start gap-x-1.5 gap-y-1 leading-[1.4]"
        title={workspace || undefined}
      >
        <WarningCircle className="mt-px shrink-0" size={14} />
        <span>{presentation.scope}</span>
        {workspaceName && presentation.scope === "Current workspace" && (
          <code className="approval-ws m-0 min-w-0 max-w-full basis-full whitespace-normal p-1.5 leading-[1.35] [overflow-wrap:anywhere] sm:basis-auto sm:flex-1">
            {workspaceName}
          </code>
        )}
      </div>

      <details className="approval-details mt-2 min-w-0 border-t border-line pt-2">
        <summary className="inline-flex items-center gap-1 text-[12px] text-ink-2"><CaretRight size={12} /> Details</summary>
        <pre className="m-0 mt-2 max-h-48 max-w-full overflow-auto whitespace-pre-wrap rounded-[8px] bg-panel-2 p-2 text-[11px] [overflow-wrap:anywhere]">
          {pretty(approval.args)}
        </pre>
        {approval.gates.length > 0 && (
          <div className="approval-gates">
            {approval.gates.map((gate, index) => (
              <span key={index}>{gate.gate}: {gate.decision}{gate.reason ? ` · ${gate.reason}` : ""}</span>
            ))}
          </div>
        )}
      </details>

      {readonly ? (
        <div className="approval-readonly">Review this request in the parent session.</div>
      ) : (
        <div className="approval-actions min-w-0">
          {denying ? (
            <div className="deny-reason w-full min-w-0 flex-col sm:flex-row">
              <input
                className="w-full min-w-0"
                autoFocus
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                placeholder="Reason (optional)"
              />
              <div className="flex shrink-0 justify-end gap-2">
                <button className="min-h-9 flex-1 sm:flex-none" disabled={busy} onClick={() => setDenying(false)}>Cancel</button>
                <button className="danger min-h-9 flex-1 sm:flex-none" disabled={busy} onClick={() => decide("deny")}>Deny</button>
              </div>
            </div>
          ) : (
            <>
              <button
                className="primary min-h-9 flex-[1_1_100%] sm:flex-initial"
                disabled={busy}
                onClick={() => decide("approve")}
              >
                Approve once
              </button>
              <button
                className="subtle min-h-9 flex-1 sm:flex-initial"
                disabled={busy}
                title="Approve AND save an exact allow rule to your user config, so this same call never asks again (any session)"
                onClick={() => decide("approve", true)}
              >
                Always allow
              </button>
              <button className="min-h-9 flex-1 sm:flex-initial" disabled={busy} onClick={() => setDenying(true)}>Deny</button>
              {/* 平台感知:mac 显示 ⌘,其余显示 Ctrl(QA-0718 实测 Linux 上显示 ⌘,与
                  sidebar 的 CtrlAltN 提示不一致)。 */}
              <span className="approval-shortcut ml-auto max-[680px]:hidden">{modLabel}↵ approve · {modLabel}⌫ deny</span>
            </>
          )}
        </div>
      )}
    </section>
  );
}

import { useEffect, useState } from "react";
import { FileCode, Files } from "@phosphor-icons/react";
import { AR } from "../api";
import { summarizeChanges, type ChangesSummary } from "../diffSummary";

export function ChangesOutcome({ sid, refreshKey, onReview }: { sid: string; refreshKey: number; onReview: () => void }) {
  const [summary, setSummary] = useState<ChangesSummary | null>(null);

  useEffect(() => {
    let alive = true;
    AR.diff(sid)
      .then((data) => {
        if (!alive) return;
        if (!data.known || !data.isRepo || data.nested) setSummary(null);
        else setSummary(summarizeChanges(data));
      })
      .catch(() => alive && setSummary(null));
    return () => { alive = false; };
  }, [sid, refreshKey]);

  if (!summary?.files.length) return null;
  const shown = summary.files.slice(0, 6);
  return (
    <section className="changes-outcome" aria-label="Workspace changes">
      <header>
        <span className="changes-outcome-icon"><Files size={18} /></span>
        <div className="changes-outcome-title">
          <b>Edited {summary.files.length} file{summary.files.length === 1 ? "" : "s"}</b>
          <span>
            {summary.totalAdd > 0 && <em className="add">+{summary.totalAdd}</em>}
            {summary.totalDel > 0 && <em className="del">−{summary.totalDel}</em>}
          </span>
        </div>
        <button type="button" onClick={onReview}>Review</button>
      </header>
      <div className="changes-outcome-files">
        {shown.map((file) => (
          <div key={file.path}>
            <FileCode size={14} />
            <span title={file.path}>{file.path}</span>
            {file.countsKnown && (
              <small>
                {file.add > 0 && <em className="add">+{file.add}</em>}
                {file.del > 0 && <em className="del">−{file.del}</em>}
              </small>
            )}
            {!file.countsKnown && <small className="dim">new</small>}
          </div>
        ))}
        {summary.files.length > shown.length && <button type="button" onClick={onReview}>Show {summary.files.length - shown.length} more files</button>}
      </div>
    </section>
  );
}

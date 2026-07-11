import { useEffect, useState } from "react";
import { DownloadSimple, FileCode, FilePdf, FileText, Files } from "@phosphor-icons/react";
import { AR } from "../api";
import { splitPath, summarizeChanges, type ChangesSummary, type FileDiffSummary } from "../diffSummary";

// J2 · Document artifacts. A completed turn's produced documents get a
// Codex-style file-card row above the Edited-files summary. Only prose/document
// files qualify — code files stay in the diff card so the chip row doesn't turn
// into noise. The extension drives both the icon and the "Type · EXT" subtitle.
const DOC_KIND: Record<string, string> = {
  md: "Document",
  markdown: "Document",
  txt: "Document",
  text: "Document",
  rst: "Document",
  adoc: "Document",
  org: "Document",
  rtf: "Document",
  doc: "Document",
  docx: "Document",
  pdf: "PDF",
};

function docKind(path: string): { ext: string; label: string } | null {
  const m = /\.([A-Za-z0-9]+)$/.exec(path);
  if (!m) return null;
  const ext = m[1].toLowerCase();
  const label = DOC_KIND[ext];
  return label ? { ext, label } : null;
}

function ArtifactChips({ files }: { files: FileDiffSummary[] }) {
  const docs: { file: FileDiffSummary; ext: string; label: string }[] = [];
  for (const file of files) {
    const kind = docKind(file.path);
    if (kind) docs.push({ file, ext: kind.ext, label: kind.label });
  }
  if (!docs.length) return null;
  return (
    <div className="cx-artifacts" aria-label="Documents produced this turn">
      {docs.map(({ file, ext, label }) => {
        const { base } = splitPath(file.path);
        return (
          <div className="cx-artifact" key={file.path}>
            <span className="cx-artifact-ic">{ext === "pdf" ? <FilePdf size={18} /> : <FileText size={18} />}</span>
            <div className="cx-artifact-meta">
              <span className="cx-artifact-name" title={file.path}>{base}</span>
              <span className="cx-artifact-sub">{label} · {ext.toUpperCase()}</span>
            </div>
            {/* No workspace file-read endpoint exists yet, so Download stays
                dimmed with an explanatory tooltip rather than faking a link. */}
            <span
              className="cx-artifact-dl"
              role="button"
              aria-disabled="true"
              title="Download isn't available yet — the workspace file API isn't wired up. Use Review to see the change."
            >
              <DownloadSimple size={16} />
            </span>
          </div>
        );
      })}
    </div>
  );
}

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
    <>
      <ArtifactChips files={summary.files} />
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
    </>
  );
}

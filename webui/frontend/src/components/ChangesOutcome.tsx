import { useEffect, useState } from "react";
import { ArrowCounterClockwise, ArrowSquareOut, CaretDown, CaretUp, DownloadSimple, FilePdf, FileText, GitDiff, ImageBroken } from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";
import { splitPath, summarizeChanges, type ChangesSummary, type FileDiffSummary } from "../diffSummary";
import { Lightbox } from "./Lightbox";

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

// Image artifacts (INC-41 RT-1). A turn that produces a screenshot/chart used to
// leave it as one grey line inside "Edited N files" — the picture the turn was
// about was never actually shown. Image extensions get preview cards instead,
// rendered from the same session-file endpoint the document rows use, and open
// full-size in the shared Lightbox.
const IMG_EXT = new Set(["png", "jpg", "jpeg", "gif", "webp", "svg", "avif", "bmp", "ico"]);

// Artifact cards past this count fold behind a "Show N more" toggle so a
// document-heavy turn doesn't flood the thread (Codex caps the same way).
const ARTIFACT_CAP = 4;
// Thumbnails are far smaller than a document row, so the grid holds more before
// folding.
const IMAGE_CAP = 6;

function fileExt(path: string): string {
  const m = /\.([A-Za-z0-9]+)$/.exec(path);
  return m ? m[1].toLowerCase() : "";
}

function docKind(path: string): { ext: string; label: string } | null {
  const ext = fileExt(path);
  if (!ext) return null;
  const label = DOC_KIND[ext];
  return label ? { ext, label } : null;
}

// One preview card: the image itself over its filename. A file that can't be
// decoded (a corrupt write, or an .svg the browser refuses) degrades to a
// broken-image placeholder card that still names + links the file.
function ImageCard({ sid, path, onOpen }: { sid: string; path: string; onOpen: () => void }) {
  const [failed, setFailed] = useState(false);
  const { base } = splitPath(path);
  const url = AR.fileURL(sid, path);
  return (
    <button
      type="button"
      className="flex flex-col w-[168px] shrink-0 rounded-[10px] border border-line bg-panel overflow-hidden text-left hover:border-ink-2"
      onClick={onOpen}
      title={path}
      aria-label={`Open ${base}`}
    >
      {failed ? (
        <span className="grid place-items-center w-full h-[104px] bg-panel-2 text-dim">
          <ImageBroken size={20} />
        </span>
      ) : (
        <img
          className="w-full h-[104px] object-cover bg-panel-2"
          src={url}
          alt={base}
          loading="lazy"
          onError={() => setFailed(true)}
        />
      )}
      <span className="px-[9px] py-[7px] text-[12px] text-ink overflow-hidden text-ellipsis whitespace-nowrap border-t border-line">{base}</span>
    </button>
  );
}

function ImageArtifacts({ sid, files }: { sid: string; files: FileDiffSummary[] }) {
  const [expanded, setExpanded] = useState(false);
  const [lightbox, setLightbox] = useState<number | null>(null);
  const imgs = files.filter((f) => IMG_EXT.has(fileExt(f.path))).map((f) => f.path);
  if (!imgs.length) return null;
  const shown = expanded ? imgs : imgs.slice(0, IMAGE_CAP);
  const hidden = imgs.length - shown.length;
  return (
    <div className="flex flex-col gap-[8px] mt-[12px] mb-[8px]" aria-label="Images produced this turn">
      <div className="flex flex-wrap gap-[8px]">
        {shown.map((path, i) => (
          <ImageCard key={path} sid={sid} path={path} onOpen={() => setLightbox(i)} />
        ))}
      </div>
      {(hidden > 0 || expanded) && imgs.length > IMAGE_CAP && (
        <button
          type="button"
          className="self-start inline-flex items-center gap-[5px] text-[12px] text-dim hover:text-ink"
          onClick={() => setExpanded((e) => !e)}
        >
          {expanded ? "Show less" : `Show ${hidden} more`}
          {expanded ? <CaretUp size={13} /> : <CaretDown size={13} />}
        </button>
      )}
      {lightbox !== null && (
        <Lightbox
          images={shown}
          index={lightbox}
          resolve={(p) => AR.fileURL(sid, p)}
          onIndex={setLightbox}
          onClose={() => setLightbox(null)}
        />
      )}
    </div>
  );
}

// One artifact row inside the shared bordered container. The two former
// controls (Open in a new tab + Download) fold behind a single "Open in ⌄"
// caret menu (Codex parity), so the row keeps a single trailing control.
function ArtifactRow({ sid, file, ext, label, divider }: { sid: string; file: FileDiffSummary; ext: string; label: string; divider: boolean }) {
  const [open, setOpen] = useState(false);
  const { base } = splitPath(file.path);
  const url = AR.fileURL(sid, file.path);
  return (
    <div className={"flex items-center gap-[10px] px-[10px] py-[8px]" + (divider ? " border-t border-line" : "")}>
      <span className="grid place-items-center w-[32px] h-[32px] shrink-0 rounded-[8px] bg-panel-2 text-ink-2">{ext === "pdf" ? <FilePdf size={18} /> : <FileText size={18} />}</span>
      <div className="flex flex-col gap-[1px] flex-1 min-w-0">
        <span className="text-[13px] font-[550] text-ink overflow-hidden text-ellipsis whitespace-nowrap" title={file.path}>{base}</span>
        <span className="text-[11px] text-dim">{label} · {ext.toUpperCase()}</span>
      </div>
      <div className="relative shrink-0">
        <button
          type="button"
          className="inline-flex items-center gap-[6px] px-[11px] h-[30px] rounded-[8px] border border-line text-[13px] text-ink hover:bg-panel-2"
          onClick={() => setOpen((o) => !o)}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label={`Open ${base}`}
        >
          Open in <CaretDown size={13} />
        </button>
        {open && (
          <>
            <button type="button" className="fixed inset-0 z-[5] cursor-default" aria-hidden="true" tabIndex={-1} onClick={() => setOpen(false)} />
            <div className="absolute right-0 top-[34px] z-10 flex flex-col min-w-[160px] py-[4px] rounded-[8px] border border-line bg-panel shadow-lg" role="menu">
              <a
                className="flex items-center gap-[8px] px-[10px] py-[6px] text-[13px] text-ink hover:bg-panel-2"
                href={url}
                target="_blank"
                rel="noreferrer"
                role="menuitem"
                onClick={() => setOpen(false)}
              >
                <ArrowSquareOut size={14} /> New tab
              </a>
              <a
                className="flex items-center gap-[8px] px-[10px] py-[6px] text-[13px] text-ink hover:bg-panel-2"
                href={url}
                download={base}
                role="menuitem"
                onClick={() => setOpen(false)}
              >
                <DownloadSimple size={14} /> Download
              </a>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function ArtifactChips({ sid, files }: { sid: string; files: FileDiffSummary[] }) {
  const [expanded, setExpanded] = useState(false);
  const docs: { file: FileDiffSummary; ext: string; label: string }[] = [];
  for (const file of files) {
    const kind = docKind(file.path);
    if (kind) docs.push({ file, ext: kind.ext, label: kind.label });
  }
  if (!docs.length) return null;
  const shown = expanded ? docs : docs.slice(0, ARTIFACT_CAP);
  const hidden = docs.length - shown.length;
  return (
    <div className="flex flex-col mt-[12px] mb-[8px] border border-line rounded-[8px] bg-panel overflow-hidden" aria-label="Documents produced this turn">
      {shown.map(({ file, ext, label }, i) => (
        <ArtifactRow key={file.path} sid={sid} file={file} ext={ext} label={label} divider={i > 0} />
      ))}
      {hidden > 0 && (
        <button
          type="button"
          className="inline-flex items-center justify-center gap-[5px] py-[8px] border-t border-line text-[12px] text-dim hover:text-ink"
          onClick={() => setExpanded(true)}
        >
          Show {hidden} more<CaretDown size={13} />
        </button>
      )}
      {expanded && docs.length > ARTIFACT_CAP && (
        <button
          type="button"
          className="inline-flex items-center justify-center gap-[5px] py-[8px] border-t border-line text-[12px] text-dim hover:text-ink"
          onClick={() => setExpanded(false)}
        >
          Show less<CaretUp size={13} />
        </button>
      )}
    </div>
  );
}

export function ChangesOutcome({ sid, refreshKey, onReview }: { sid: string; refreshKey: number; onReview: () => void }) {
  const openModal = useStore((s) => s.openModal);
  const toast = useStore((s) => s.toast);
  const [summary, setSummary] = useState<ChangesSummary | null>(null);
  const [expanded, setExpanded] = useState(false);
  // Local bump re-fetches the summary after an Undo without needing the parent
  // to change refreshKey (a reverted card should collapse to nothing).
  const [bump, setBump] = useState(0);

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
  }, [sid, refreshKey, bump]);

  // Undo ↺ — discard the whole change set back to HEAD (destructive; confirmed).
  const undo = () => {
    const n = summary?.files.length ?? 0;
    openModal({
      kind: "confirm",
      title: "Undo all changes?",
      body: `Discards all ${n} changed file${n === 1 ? "" : "s"} in the workspace back to the last commit and deletes any new files the agent created. This can't be undone.`,
      confirmLabel: "Undo changes",
      danger: true,
      onConfirm: async () => {
        try {
          await AR.revert(sid);
          toast("changes reverted", "info");
          setBump((b) => b + 1);
        } catch (e: any) {
          toast(e.message);
        }
      },
    });
  };

  if (!summary?.files.length) return null;
  // "Show N more files" reveals the remaining rows inline (Codex behaviour);
  // the header "Review" button stays the separate jump-to-full-diff path.
  const shown = expanded ? summary.files : summary.files.slice(0, 6);
  const hidden = summary.files.length - shown.length;
  return (
    <>
      <ImageArtifacts sid={sid} files={summary.files} />
      <ArtifactChips sid={sid} files={summary.files} />
      <section className="changes-outcome" aria-label="Workspace changes">
        <header>
          <span className="changes-outcome-icon"><GitDiff size={18} /></span>
          <div className="changes-outcome-title">
            <b>Edited {summary.files.length} file{summary.files.length === 1 ? "" : "s"}</b>
            <span>
              <em className="add">+{summary.totalAdd}</em>
              <em className="del">−{summary.totalDel}</em>
            </span>
          </div>
          <button
            type="button"
            className="inline-flex items-center gap-[5px] bg-transparent border-0 text-ink-2 hover:text-ink"
            onClick={undo}
            title="Discard all these changes (git checkout . + remove new files)"
          >
            Undo <ArrowCounterClockwise size={13} />
          </button>
          <button type="button" onClick={onReview}>Review</button>
        </header>
        <div className="changes-outcome-files">
          {shown.map((file) => {
            const { dir, base } = splitPath(file.path);
            return (
              <div key={file.path}>
                <span title={file.path}>
                  {dir && <span style={{ color: "var(--dim)" }}>{dir}</span>}
                  <b style={{ fontWeight: 600, color: "var(--ink)" }}>{base}</b>
                </span>
                {file.countsKnown && (
                  <small>
                    <em className="add">+{file.add}</em>
                    <em className="del">−{file.del}</em>
                  </small>
                )}
                {!file.countsKnown && <small className="dim">new</small>}
              </div>
            );
          })}
          {summary.files.length > 6 && (
            <button
              type="button"
              onClick={() => setExpanded((e) => !e)}
              style={{ display: "inline-flex", alignItems: "center", gap: 5 }}
            >
              {expanded ? "Show less" : `Show ${hidden} more files`}
              {expanded ? <CaretUp size={13} /> : <CaretDown size={13} />}
            </button>
          )}
        </div>
      </section>
    </>
  );
}

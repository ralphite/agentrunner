import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { ArrowClockwise, ArrowCounterClockwise, ArrowSquareOut, CaretDown, CaretUp, DownloadSimple, FilePdf, FileText, ImageBroken, WarningCircle } from "@phosphor-icons/react";
import { useStore } from "../store";
import { useAppServices } from "../app/appServices";
import { dropGeneratedFiles, splitPath, summarizeChanges, type ChangesSummary, type FileDiffSummary } from "../diffSummary";
import { Lightbox } from "./Lightbox";
import { inlinedImagePaths, inlinedImagesVersion, subscribeInlinedImages } from "./Markdown";

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
  const { api } = useAppServices();
  const [failed, setFailed] = useState(false);
  const { base } = splitPath(path);
  const url = api.fileURL(sid, path);
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

// INC-41 TH-9. The thumbnail row is a fallback, not a duplicate: an image the
// answer already painted inline (Markdown's `img.md-img`) is NOT re-shown here.
// It used to be unconditional, so a two-screenshot turn rendered the same pair
// twice — two 326×205 inline images plus two 144×104 cards — and the pair ate
// ~600px of a 723px thread viewport before the reader reached the summary. Only
// an image the prose never mentioned still earns a card; when every produced
// image is already inline the whole row renders nothing (no empty shell).
function ImageArtifacts({ sid, files }: { sid: string; files: FileDiffSummary[] }) {
  const { api } = useAppServices();
  const [expanded, setExpanded] = useState(false);
  const [lightbox, setLightbox] = useState<number | null>(null);
  useSyncExternalStore(subscribeInlinedImages, inlinedImagesVersion, inlinedImagesVersion);
  const inlined = inlinedImagePaths(sid);
  const imgs = files
    .filter((f) => IMG_EXT.has(fileExt(f.path)) && !inlined.has(f.path))
    .map((f) => f.path);
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
          resolve={(p) => api.fileURL(sid, p)}
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
  const { api } = useAppServices();
  const [open, setOpen] = useState(false);
  // INC-41 ART-SCRIM — the menu is anchored to the VIEWPORT, not to the row.
  // As a plain `absolute` child it was clipped to a 9px sliver of its own 74px
  // box: the artifact container two levels up is `overflow-hidden` (it has to be,
  // to keep the rows inside its rounded corners), and it ends ~9px below the
  // trigger. Worse, the clip took the menu out of hit-testing too, so a click
  // aimed at "New tab" / "Download" fell straight through to the scrim behind and
  // merely closed the menu — the control was 100% inoperable, not just ugly.
  // `position: fixed` escapes any ancestor overflow (no transformed ancestor is
  // in play — the scrim proves it, it already covers the viewport from the same
  // subtree), so the menu can hang outside the card the way a menu must.
  const btnRef = useRef<HTMLButtonElement>(null);
  const [pos, setPos] = useState<{ top: number; right: number } | null>(null);
  const { base } = splitPath(file.path);
  const url = api.fileURL(sid, file.path);

  // A viewport-anchored menu would drift away from its trigger if the thread
  // scrolled underneath it, so scrolling (the timeline is its own scroll box,
  // hence the capture listener) or resizing just dismisses it.
  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
  }, [open]);

  const toggle = () => {
    if (open) {
      setOpen(false);
      return;
    }
    const r = btnRef.current?.getBoundingClientRect();
    if (r) setPos({ top: r.bottom + 4, right: Math.max(8, window.innerWidth - r.right) });
    setOpen(true);
  };

  return (
    <div className={"flex items-center gap-[10px] px-[12px] py-[10px]" + (divider ? " border-t border-line" : "")}>
      <span className="grid place-items-center w-[38px] h-[38px] shrink-0 rounded-[10px] bg-panel-2 text-ink-2">{ext === "pdf" ? <FilePdf size={20} /> : <FileText size={20} />}</span>
      <div className="flex flex-col gap-[1px] flex-1 min-w-0">
        <span className="text-[15px] font-[550] text-ink overflow-hidden text-ellipsis whitespace-nowrap" title={file.path}>{base}</span>
        <span className="text-[13px] text-dim">{label} · {ext.toUpperCase()}</span>
      </div>
      <div className="relative shrink-0">
        <button
          ref={btnRef}
          type="button"
          className="inline-flex items-center gap-[6px] px-[11px] h-[30px] rounded-[8px] border border-line text-[13px] text-ink hover:bg-panel-2"
          onClick={toggle}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label={`Open ${base}`}
        >
          Open in <CaretDown size={13} />
        </button>
        {open && pos && (
          <>
            {/* INC-41 ART-SCRIM — the click-outside scrim. It MUST paint nothing:
                the app's base reset (tw.css @layer base) gives every <button> an
                opaque `background: var(--panel)` and a `:hover` of `var(--panel-2)`,
                so this bare full-bleed button used to cover the entire viewport in
                flat white — grey (#f4f4f4) once the cursor landed on it after the
                click. The whole app read as "crashed". `bg-transparent border-0` is
                load-bearing; tw.css carries a matching net for future scrims. */}
            <button type="button" className="fixed inset-0 z-[5] cursor-default bg-transparent border-0" aria-hidden="true" tabIndex={-1} onClick={() => setOpen(false)} />
            <div
              className="fixed z-10 flex flex-col min-w-[160px] py-[4px] rounded-[8px] border border-line bg-panel shadow-lg"
              style={{ top: pos.top, right: pos.right }}
              role="menu"
            >
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
    <div className="flex flex-col mt-[12px] mb-[8px] border border-line rounded-[14px] bg-panel overflow-hidden" aria-label="Documents produced this turn">
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

// INC-41 TH-8: the card is a SUMMARY, not a file list — Codex previews exactly
// three rows and folds the rest behind "Show N more files".
const PREVIEW_CAP = 3;

// INC-41 TH-6 — the badge glyph. It used to be Phosphor's `GitDiff` (the
// two-branch fork-and-merge arrows), which says "this turn branched/merged" —
// nothing of the sort happened; the card is about lines ADDED and REMOVED.
// Codex draws a boxed ± (qa/codex-reference/codex-crop-change-card.jpg), and
// Phosphor ships no boxed ± (its `PlusMinus` is the slashed math sign), so the
// glyph is drawn here: 24-grid, currentColor, stroke weight matched to the
// Phosphor icons it sits beside.
function PlusMinusSquare({ size = 18 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.7}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <rect x="3.25" y="3.25" width="17.5" height="17.5" rx="4.75" />
      {/* + over − */}
      <path d="M12 7.4v4.3M9.85 9.55h4.3M9.4 15.95h5.2" />
    </svg>
  );
}

// The card shell: the ± badge plus whatever the current phase puts next to it.
// Loading, error and loaded all render through it, so the header keeps the same
// 64px box in every phase and the thread never reflows underneath (INC-41 TH-7).
function ChangesShell({ children }: { children: ReactNode }) {
  return (
    <section className="changes-outcome overflow-hidden" aria-label="Workspace changes">
      <header className="flex min-w-0 items-center gap-[11px]">
        <span className="changes-outcome-icon grid h-[38px] w-[38px] shrink-0 place-items-center rounded-[10px] bg-panel-2 text-ink-2"><PlusMinusSquare /></span>
        {children}
      </header>
    </section>
  );
}

type Phase = "loading" | "ready" | "error";

// onReview carries the card's current scope so the diff panel opens on the
// scope this card is claiming — a "Changes in workspace" card must not link
// into a panel that answers "No changes this turn" (QA-0719: the workspace
// fallback broke RVW-4's card-scope/panel-scope pairing).
export function ChangesOutcome({ sid, refreshKey, onReview }: { sid: string; refreshKey: number; onReview: (scope: "turn" | "workspace") => void }) {
  const { api } = useAppServices();
  const openModal = useStore((s) => s.openModal);
  const toast = useStore((s) => s.toast);
  const focusDiffFile = useStore((s) => s.focusDiffFile);
  const bumpWorkspaceEpoch = useStore((s) => s.bumpWorkspaceEpoch);
  const [summary, setSummary] = useState<ChangesSummary | null>(null);
  // A merge conflict is a workspace-wide blocking state, not merely one more
  // changed file. Keep it on the timeline card so the main screen does not look
  // healthy until the user happens to open Review.
  const [conflicts, setConflicts] = useState<string[]>([]);
  // INC-41 TH-7. The fetch used to have two outcomes — a summary, or null — so a
  // failed request was indistinguishable from "this turn changed nothing": one
  // flaky /diff and the whole "Edited N files" card vanished without a word, and
  // the user read that as "the agent touched no files". The phase is now
  // explicit: an in-flight fetch paints a skeleton, a failed one keeps the card
  // shell and offers Retry, and only a backend that actually says "no changed
  // files" renders nothing.
  const [phase, setPhase] = useState<Phase>("loading");
  // "turn" = last-turn 快照有内容(标题 Edited N files);"workspace" =
  // 回退到 working-tree(标题 Changes in workspace,不谎称本 turn 编辑)。
  const [scope, setScope] = useState<"turn" | "workspace">("turn");
  const [expanded, setExpanded] = useState(false);
  // Local bump re-fetches the summary after an Undo (or a Retry) without needing
  // the parent to change refreshKey (a reverted card should collapse to nothing).
  const [bump, setBump] = useState(0);
  // refreshKey ticks on every streamed event, so a live turn re-fetches the diff
  // constantly. Those refreshes must NOT drop back to the skeleton — the card
  // would strobe. Only a new session (or an explicit Retry) re-arms it.
  const shownSid = useRef<string | null>(null);

  useEffect(() => {
    let alive = true;
    if (shownSid.current !== sid) {
      shownSid.current = sid;
      setPhase("loading");
      setSummary(null);
      setConflicts([]);
    }
    // QA-0718 用户实机(两张截图,两个方向的错):
    // 1. 无参 diff = working-tree 全量——新会话接手带历史未提交改动的
    //    workspace 时,旧脏货被谎称成"这个 turn 编辑的文件"。→ 先查
    //    last-turn scope(基线 = 最近一次人类 turn 开始时的 shadow
    //    snapshot),标题 "Edited N files" 只在它有内容时使用。
    // 2. 但 last-turn 基线并不总在:daemon 重启后丢失、子 agent 写盘
    //    可能不入父 turn 快照——文件明明写了,卡却整个消失(第二张
    //    截图)。→ last-turn 空时回退 working-tree,有变更则以
    //    "Changes in workspace" 呈现:不谎称本 turn 编辑,但工作区
    //    现状(可 Review/commit)不失踪。
    // Both backend scopes project the workspace-wide conflict set. Trust that
    // contract here instead of doubling /diff traffic on every streamed event.
    api.diff(sid, "last-turn")
      .then(async (data) => {
        if (!alive) return;
        setConflicts(data.conflicts || []);
        const turnSummary = !data.known || !data.isRepo || data.nested ? null : dropGeneratedFiles(summarizeChanges(data));
        if (turnSummary?.files.length) {
          setScope("turn");
          setSummary(turnSummary);
          setPhase("ready");
          return;
        }
        const wt = await api.diff(sid, "working-tree");
        if (!alive) return;
        setConflicts(wt.conflicts || data.conflicts || []);
        const wtSummary = !wt.known || !wt.isRepo || wt.nested ? null : dropGeneratedFiles(summarizeChanges(wt));
        setScope("workspace");
        setSummary(wtSummary?.files.length ? wtSummary : null);
        setPhase("ready");
      })
      .catch(() => {
        if (!alive) return;
        setPhase("error");
      });
    return () => { alive = false; };
  }, [sid, refreshKey, bump]);

  const retry = () => {
    setPhase("loading");
    setBump((b) => b + 1);
  };

  // Undo ↺ — discard the whole change set back to HEAD (destructive; confirmed).
  // 卡显示的是 last-turn,而 revert 吞的是整个 working tree——两者可能不同
  // (workspace 带着本 turn 之前的未提交改动)。确认弹窗必须报 revert 的
  // 真实范围,所以先取 working-tree 计数;超出卡集合时明说(QA-0718)。
  const undo = async () => {
    const cardN = summary?.files.length ?? 0;
    let n = cardN;
    try {
      const wt = await api.diff(sid, "working-tree");
      if (wt.known && wt.isRepo && !wt.nested) n = summarizeChanges(wt).files.length;
    } catch {
      /* fall back to the card count */
    }
    const beyond = n > cardN ? ` — including ${n - cardN} file${n - cardN === 1 ? "" : "s"} changed before this turn` : "";
    openModal({
      kind: "confirm",
      title: "Undo all changes?",
      body: `Discards all ${n} uncommitted file${n === 1 ? "" : "s"} in the workspace back to the last commit and deletes any new files${beyond}. This can't be undone.`,
      confirmLabel: "Undo changes",
      danger: true,
      onConfirm: async () => {
        try {
          await api.revert(sid);
          toast("changes reverted", "info");
          setBump((b) => b + 1);
          bumpWorkspaceEpoch(); // the rail's git rows state the same facts
        } catch (e: any) {
          toast(e.message);
        }
      },
    });
  };

  if (phase === "loading") {
    return (
      <ChangesShell>
        <div className="changes-outcome-skel" aria-label="Loading changes" role="status">
          <div className="text-[12px] text-dim">Loading changes…</div>
          <span />
          <span />
        </div>
      </ChangesShell>
    );
  }

  if (phase === "error") {
    return (
      <ChangesShell>
        <div className="changes-outcome-title">
          <b>Couldn't load changes</b>
        </div>
        <button type="button" onClick={retry} className="inline-flex items-center gap-[5px]">
          Retry <ArrowClockwise size={13} />
        </button>
      </ChangesShell>
    );
  }

  if (!summary?.files.length) return null;
  // "Show N more files" reveals the remaining rows inline (Codex behaviour);
  // the header "Review" button stays the separate jump-to-full-diff path.
  const shown = expanded ? summary.files : summary.files.slice(0, PREVIEW_CAP);
  const hidden = summary.files.length - shown.length;
  // INC-41 TH-13 — never print a ± count we don't have. A new/untracked (or
  // binary) file carries countsKnown=false and add/del=0, so a turn that only
  // CREATED files summed to a confident, green-and-red `+0 −0` under "Edited 2
  // files" — read literally: "two files changed, not one line touched". False,
  // and it contradicted the Supervision panel one screen over, which said
  // `Changes · 2 new` off the same data. The ± pair now covers only the files
  // whose counts git actually gave us, and the uncounted ones are reported the
  // panel's way: `N new`. All-unknown → just `N new`, no zeros at all.
  const countedFiles = summary.files.filter((f) => f.countsKnown).length;
  const newFiles = summary.files.length - countedFiles;
  return (
    <>
      <ImageArtifacts sid={sid} files={summary.files} />
      <ArtifactChips sid={sid} files={summary.files} />
      <section className="changes-outcome overflow-hidden" aria-label="Workspace changes">
        <header className="flex min-w-0 items-center gap-[11px]">
          <span className="changes-outcome-icon grid h-[38px] w-[38px] shrink-0 place-items-center rounded-[10px] bg-panel-2 text-ink-2"><PlusMinusSquare /></span>
          <div className="changes-outcome-title grid min-w-0 flex-1 gap-[2px] text-[15px] leading-tight">
            <b className="overflow-hidden text-ellipsis whitespace-nowrap">{scope === "turn" ? `Edited ${summary.files.length} file${summary.files.length === 1 ? "" : "s"}` : "Changes in workspace"}</b>
            <span className="flex items-center gap-[7px] overflow-hidden whitespace-nowrap text-[13px]">
              {countedFiles > 0 && (
                <>
                  <em className="add not-italic text-green">+{summary.totalAdd}</em>
                  <em className="del not-italic text-red">-{summary.totalDel}</em>
                </>
              )}
              {newFiles > 0 && (
                <em className="dim not-italic text-dim">{countedFiles > 0 ? "· " : ""}{newFiles} new</em>
              )}
              {conflicts.length > 0 && (
                <em
                  className="inline-flex items-center gap-[4px] not-italic text-red"
                  title={conflicts.join("\n")}
                >
                  <WarningCircle size={13} weight="fill" />
                  {conflicts.length} merge {conflicts.length === 1 ? "conflict" : "conflicts"}
                </em>
              )}
            </span>
          </div>
          <div className="changes-outcome-actions ml-auto flex shrink-0 items-center gap-1">
            <button
              type="button"
              className="inline-flex shrink-0 items-center gap-[5px] border-0 bg-transparent px-2 text-ink hover:text-ink"
              onClick={undo}
              title="Discard all these changes (git checkout . + remove new files)"
            >
              Undo <ArrowCounterClockwise size={13} />
            </button>
            {/* CHANGE-CARD-REVIEW-BTN (R68): Codex gold renders Review as an outlined pill, not a borderless slab — match the sibling "Open in" pill (:189). */}
            <button type="button" className="inline-flex items-center shrink-0 px-[11px] h-[30px] rounded-[8px] border border-line text-[13px] text-ink hover:bg-panel-2" onClick={() => onReview(scope)}>Review</button>
          </div>
        </header>
        <div className="changes-outcome-files -mx-3 -mb-3 mt-3 grid gap-0 overflow-hidden border-t border-line-2">
          {shown.map((file) => {
            const { dir, base } = splitPath(file.path);
            // INC-41 TH-5 — the file row is NAVIGATION, not a label. Codex's
            // change card sends you to the file's diff when you click its name;
            // ours rendered the same three columns and then swallowed the click,
            // so the one obvious question a summary raises ("what changed in
            // THAT file?") had no answer but "find it yourself in the panel".
            // It's a `div[role=button]` rather than a `<button>` because the row's
            // whole layout — 38px beat, the path's flex:1 ellipsis, the right-set
            // ± column — hangs off `.changes-outcome-files > div` in tw.css,
            // and `> button` is already spoken for by the "Show N more" row.
            const open = () => {
              focusDiffFile(file.path); // DiffView expands + scrolls to it
              onReview(scope); // …in the panel the Review button already opens
            };
            return (
              <div
                key={file.path}
                role="button"
                tabIndex={0}
                className="flex min-h-[38px] min-w-0 cursor-pointer items-center gap-2 px-[14px] py-[7px] text-[13px] text-ink-2 hover:bg-panel-2"
                aria-label={`Review changes to ${base}`}
                onClick={open}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    open();
                  }
                }}
              >
                <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap" title={file.path}>
                  {dir && <span style={{ color: "var(--dim)" }}>{dir}</span>}
                  <b style={{ fontWeight: 600, color: "var(--ink)" }}>{base}</b>
                </span>
                {file.countsKnown && (
                  <small className="flex shrink-0 items-center gap-[7px] text-[13px]">
                    <em className="add not-italic">+{file.add}</em>
                    <em className="del not-italic">-{file.del}</em>
                  </small>
                )}
                {!file.countsKnown && <small className="dim shrink-0 text-[13px] text-dim">new</small>}
              </div>
            );
          })}
          {summary.files.length > PREVIEW_CAP && (
            // INC-41 TR-4 — this toggle is one more ROW of the list (tw.css
            // gives it the file row's height and left inset, and the full row as
            // hit target), so it carries no inline layout of its own. And it
            // counts: 4 files behind a cap of 3 used to read "Show 1 more files".
            <button
              type="button"
              className="flex min-h-[38px] w-full items-center gap-[5px] rounded-none border-0 bg-transparent px-[14px] py-[7px] text-left text-[13px] text-dim hover:bg-panel-2 hover:text-ink-2"
              onClick={() => setExpanded((e) => !e)}
            >
              {expanded ? "Show less" : `Show ${hidden} more file${hidden === 1 ? "" : "s"}`}
              {expanded ? <CaretUp size={13} /> : <CaretDown size={13} />}
            </button>
          )}
        </div>
      </section>
    </>
  );
}

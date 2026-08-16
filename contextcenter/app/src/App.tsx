import { useCallback, useEffect, useRef, useState } from "react";
import { MoreHorizontal, PanelLeft, PanelRight, Sparkles } from "lucide-react";
import { CRUMBS, STUBS, INITIAL_STATUSES } from "./app-data";
import { AppCtx, type Statuses } from "./ctx";
import { Sidebar } from "./tree";
import { Page } from "./pages";
import { MetadataPanel, ChatPanel, type ChatMsg } from "./rail";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip";

const DEFAULT_DOC = "session-recovery";

function readHash(): string {
  const id = window.location.hash.slice(1).split("@")[0];
  return id && (CRUMBS[id] || STUBS[id]) ? id : DEFAULT_DOC;
}

export default function App() {
  const [doc, setDoc] = useState(readHash);
  const [sideClosed, setSideClosed] = useState(false);
  const [railHidden, setRailHidden] = useState(false);
  const [statuses, setStatuses] = useState<Statuses>(INITIAL_STATUSES);
  const [agent, setAgent] = useState("Codex");
  const [draft, setDraft] = useState("");
  const [extraMsgs, setExtraMsgs] = useState<Record<string, ChatMsg[]>>({});
  const [pill, setPill] = useState<{ x: number; y: number } | null>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const openDoc = useCallback((id: string) => {
    setDoc(id);
    setDraft("");
    setPill(null);
    window.location.hash = id;
    canvasRef.current?.scrollTo({ top: 0 });
  }, []);

  useEffect(() => {
    const onHash = () => setDoc(readHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => {
    const onSend = (e: Event) => {
      const { doc: d, msg } = (e as CustomEvent).detail as { doc: string; msg: ChatMsg };
      setExtraMsgs((prev) => ({ ...prev, [d]: [...(prev[d] ?? []), msg] }));
    };
    window.addEventListener("cc-send", onSend);
    return () => window.removeEventListener("cc-send", onSend);
  }, []);

  // 选区 → Quote in chat 浮标
  useEffect(() => {
    let t: number;
    const onUp = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest?.(".askpill")) return;
      window.clearTimeout(t);
      t = window.setTimeout(() => {
        const sel = window.getSelection();
        const txt = sel?.toString().trim() ?? "";
        const inCanvas = (e.target as HTMLElement).closest?.(".canvas");
        if (txt.length > 2 && sel?.rangeCount && inCanvas) {
          const r = sel.getRangeAt(0).getBoundingClientRect();
          setPill({
            x: Math.max(12, Math.min(window.innerWidth - 190, r.left + r.width / 2 - 80)),
            y: Math.max(10, r.top - 44),
          });
        } else setPill(null);
      }, 10);
    };
    const onScroll = () => setPill(null);
    document.addEventListener("mouseup", onUp);
    document.addEventListener("scroll", onScroll, true);
    return () => {
      document.removeEventListener("mouseup", onUp);
      document.removeEventListener("scroll", onScroll, true);
    };
  }, []);

  const quoteSelection = () => {
    const sel = window.getSelection();
    const txt = sel?.toString().trim() ?? "";
    if (txt) {
      const quoted = txt.split("\n").map((l) => "> " + l.trim()).filter((l) => l !== "> ").join("\n");
      setDraft((d) => quoted + "\n" + d);
      requestAnimationFrame(() => {
        const ta = taRef.current;
        if (ta) {
          ta.style.height = "auto";
          ta.style.height = Math.min(130, ta.scrollHeight) + "px";
          ta.focus();
          ta.setSelectionRange(ta.value.length, ta.value.length);
        }
      });
    }
    sel?.removeAllRanges();
    setPill(null);
  };

  const crumbs = CRUMBS[doc] ?? STUBS[doc]?.crumbs ?? [];

  return (
    <AppCtx.Provider value={{
      doc, openDoc, statuses, agent, setAgent,
      setStatus: (key, value) => setStatuses((s) => ({ ...s, [key]: value } as Statuses)),
    }}>
      <div className={`app ${sideClosed ? "side-closed" : ""} ${railHidden ? "rail-hidden" : ""}`}>
        <Sidebar onClose={() => setSideClosed(true)} />

        <div className="main">
          <div className="topbar">
            {sideClosed && (
              <button className="iconbtn" onClick={() => setSideClosed(false)}>
                <PanelLeft size={16} strokeWidth={1.5} />
              </button>
            )}
            <div className="crumbs">
              {crumbs.map((c, idx) => (
                <span key={idx} style={{ display: "contents" }}>
                  {idx > 0 && <span className="sep">/</span>}
                  <span
                    className={`seg ${idx === crumbs.length - 1 ? "here" : ""} ${c.doc ? "link" : ""}`}
                    onClick={c.doc ? () => openDoc(c.doc!) : undefined}
                  >
                    {c.projIcon && (
                      <span style={{ color: "var(--faint)", display: "grid", placeItems: "center" }}>
                        {c.projIcon === "layers"
                          ? <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round"><path d="M12 2 22 7l-10 5L2 7z" /><path d="M2 12l10 5 10-5" /><path d="M2 17l10 5 10-5" /></svg>
                          : <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round"><path d="M12 2 21 7v10l-9 5-9-5V7z" /><path d="M3 7l9 5 9-5M12 12v10" /></svg>}
                      </span>
                    )}
                    {c.label}
                  </span>
                </span>
              ))}
            </div>
            <span style={{ flex: 1 }} />
            <Tooltip>
              <TooltipTrigger asChild>
                <button className="iconbtn" onClick={() => setRailHidden(!railHidden)}>
                  <PanelRight size={16} strokeWidth={1.5} />
                </button>
              </TooltipTrigger>
              <TooltipContent>Toggle metadata &amp; chat</TooltipContent>
            </Tooltip>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button className="iconbtn"><MoreHorizontal size={16} strokeWidth={1.5} /></button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem>Copy link</DropdownMenuItem>
                <DropdownMenuItem>Export as Markdown</DropdownMenuItem>
                <DropdownMenuItem>View frontmatter</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          <div className="canvas" ref={canvasRef}>
            <Page doc={doc} />
          </div>
        </div>

        <aside className="rail">
          <div className="rail-head">Metadata</div>
          <div className="rail-info"><MetadataPanel doc={doc} /></div>
          <ChatPanel doc={doc} extra={extraMsgs[doc] ?? []} draft={draft} setDraft={setDraft} taRef={taRef} />
        </aside>
      </div>

      {pill && (
        <button className="askpill" style={{ left: pill.x, top: pill.y }} onClick={quoteSelection}>
          <Sparkles size={13} strokeWidth={1.75} />Quote in chat
        </button>
      )}
    </AppCtx.Provider>
  );
}

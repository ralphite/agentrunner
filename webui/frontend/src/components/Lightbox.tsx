import { type CSSProperties, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { CaretLeft, CaretRight, DownloadSimple, MagnifyingGlassMinus, MagnifyingGlassPlus, X } from "@phosphor-icons/react";
import { uploadURL } from "../api";
import { FocusScope } from "../ui/FocusScope";
import { IconButton } from "../ui/IconButton";
import { IconLink } from "../ui/IconLink";

// Lightbox is the full-screen image viewer (W9): a dark overlay with the image
// centered, a bottom zoom bar (− 100% +, 25% steps clamped 50–300%), a top-right
// download + close, Esc/background-click to dismiss, and arrow-key navigation
// across the images in the same thumbnail group. Focus enters the overlay on
// open and is restored to the trigger on close.
const ZOOM_MIN = 50;
const ZOOM_MAX = 300;
const ZOOM_STEP = 25;

const basename = (path: string) => path.split("/").pop() || "image";

// `images` are opaque source keys, not URLs: `resolve` turns one into a fetchable
// URL. It defaults to uploadURL (composer attachments, the original caller), and
// the thread's inline images pass a workspace-file resolver instead (INC-41
// RT-1). Keeping the keys unresolved means the download filename still comes
// from the real path rather than from a query-string-laden endpoint URL.
export function Lightbox({
  images,
  index,
  onIndex,
  onClose,
  resolve = uploadURL,
}: {
  images: string[];
  index: number;
  onIndex: (i: number) => void;
  onClose: () => void;
  resolve?: (path: string) => string;
}) {
  const [zoom, setZoom] = useState(100);
  const [failed, setFailed] = useState(false);
  const overlayRef = useRef<HTMLDivElement>(null);
  const multi = images.length > 1;
  const src = images[index] ? resolve(images[index]) : "";
  const name = images[index] ? basename(images[index]) : "image";
  const imageStyle = {
    "--lb-mobile-width": `${zoom}%`,
    "--lb-scale": zoom / 100,
  } as CSSProperties;

  const zoomBy = (delta: number) => setZoom((z) => Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z + delta)));
  const go = (delta: number) => {
    if (!multi) return;
    onIndex((index + delta + images.length) % images.length);
  };

  // Reset zoom whenever the shown image changes — a 300% zoom on one photo
  // shouldn't carry over to the next.
  useEffect(() => {
    setZoom(100);
    setFailed(false);
  }, [index, src]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      switch (e.key) {
        case "ArrowLeft":
          if (multi) { e.preventDefault(); go(-1); }
          break;
        case "ArrowRight":
          if (multi) { e.preventDefault(); go(1); }
          break;
        case "+":
        case "=":
          e.preventDefault();
          zoomBy(ZOOM_STEP);
          break;
        case "-":
        case "_":
          e.preventDefault();
          zoomBy(-ZOOM_STEP);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [index, multi]);

  return createPortal(
    <FocusScope
      className="lightbox"
      ref={overlayRef}
      role="dialog"
      aria-modal="true"
      aria-label="Image viewer"
      initialFocus={overlayRef}
      onEscape={onClose}
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="lb-top">
        {multi && <span className="lb-count">{index + 1} / {images.length}</span>}
        <span className="lb-spacer" />
        <IconLink
          className="lb-btn"
          href={src}
          download={name}
          tone="inverse"
          variant="outline"
          aria-label="Download image"
          onClick={(e) => e.stopPropagation()}
        >
          <DownloadSimple size={18} />
        </IconLink>
        <IconButton
          className="lb-btn"
          onClick={onClose}
          title="Close (Esc)"
          tone="inverse"
          variant="outline"
          aria-label="Close"
        >
          <X size={18} />
        </IconButton>
      </div>

      <div
        className="lb-stage"
        role="region"
        aria-label="Zoomed image"
        tabIndex={0}
        onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
      >
        {failed ? (
          <div className="lb-error" role="alert">
            <span>Image unavailable</span>
            <small>{name}</small>
          </div>
        ) : (
          <img
            className={`lb-img ${zoom > 100 ? "is-zoomed" : ""}`}
            src={src}
            alt={name}
            style={imageStyle}
            onError={() => setFailed(true)}
          />
        )}
      </div>

      <div className="lb-controls" onClick={(e) => e.stopPropagation()}>
        {multi ? (
          <IconButton
            className="lb-nav prev !rounded-full"
            size="lg"
            tone="inverse"
            variant="outline"
            onClick={() => go(-1)}
            title="Previous (←)"
            aria-label="Previous image"
          >
            <CaretLeft size={26} />
          </IconButton>
        ) : <span />}
        <div className="lb-zoom-center">
          <IconButton
            className="lb-btn"
            tone="inverse"
            variant="outline"
            onClick={() => zoomBy(-ZOOM_STEP)}
            disabled={failed || zoom <= ZOOM_MIN}
            aria-label="Zoom out"
          >
            <MagnifyingGlassMinus size={17} />
          </IconButton>
          <span className="lb-pct" aria-live="polite">{zoom}%</span>
          <IconButton
            className="lb-btn"
            tone="inverse"
            variant="outline"
            onClick={() => zoomBy(ZOOM_STEP)}
            disabled={failed || zoom >= ZOOM_MAX}
            aria-label="Zoom in"
          >
            <MagnifyingGlassPlus size={17} />
          </IconButton>
        </div>
        {multi ? (
          <IconButton
            className="lb-nav next !rounded-full"
            size="lg"
            tone="inverse"
            variant="outline"
            onClick={() => go(1)}
            title="Next (→)"
            aria-label="Next image"
          >
            <CaretRight size={26} />
          </IconButton>
        ) : <span />}
      </div>
    </FocusScope>,
    document.body,
  );
}

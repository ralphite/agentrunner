import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
  type UIEventHandler,
} from "react";
import { useAppServices } from "../../app/appServices";

const NEAR_BOTTOM_PX = 80;
const STORAGE_PREFIX = "arwebui.timelineScroll.";

export interface TimelineScrollController {
  viewportRef: RefObject<HTMLDivElement>;
  showJump: boolean;
  unseen: number;
  onScroll: UIEventHandler<HTMLDivElement>;
  jumpToBottom: () => void;
}

export interface TimelineScrollControllerOptions {
  sessionKey?: string;
  activityCount: number;
  pendingCount: number;
  loading: boolean;
}

/**
 * Owns the timeline viewport lifecycle.
 *
 * The view only consumes the returned ref/state/actions; sessionStorage,
 * restoration, stick-to-bottom, unseen counting, and imperative scrolling stay
 * behind this controller boundary.
 */
export function useTimelineScrollController({
  sessionKey,
  activityCount,
  pendingCount,
  loading,
}: TimelineScrollControllerOptions): TimelineScrollController {
  const { storage } = useAppServices();
  const viewportRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const [showJump, setShowJump] = useState(false);
  const [unseen, setUnseen] = useState(0);
  const restored = useRef(!sessionKey);
  const restoredSessionKey = useRef<string>();
  const activityReady = useRef(false);
  const prevActivityCount = useRef(activityCount);
  const prevPendingCount = useRef(pendingCount);
  const scrollStorageKey = sessionKey ? `${STORAGE_PREFIX}${sessionKey}` : "";

  const clearSavedPosition = () => {
    if (!scrollStorageKey) return;
    try {
      storage.session.removeItem(scrollStorageKey);
    } catch {
      // Storage can be unavailable in hardened/private browser contexts.
    }
  };

  useLayoutEffect(() => {
    const el = viewportRef.current;
    if (!el) return;
    if (restoredSessionKey.current !== sessionKey) {
      restoredSessionKey.current = sessionKey;
      restored.current = !sessionKey;
      stick.current = true;
      activityReady.current = false;
      prevActivityCount.current = activityCount;
      prevPendingCount.current = pendingCount;
      setShowJump(false);
      setUnseen(0);
    }
    if (!restored.current) {
      if (loading) return;
      restored.current = true;
      let savedTop: number | null = null;
      try {
        const raw = storage.session.getItem(scrollStorageKey);
        if (raw !== null) {
          const parsed = Number(raw);
          if (Number.isFinite(parsed) && parsed >= 0) savedTop = parsed;
        }
      } catch {
        // Fall through to the normal latest position.
      }
      const maxTop = Math.max(0, el.scrollHeight - el.clientHeight);
      if (savedTop !== null && maxTop - savedTop >= NEAR_BOTTOM_PX) {
        stick.current = false;
        el.scrollTop = Math.min(savedTop, maxTop);
        setShowJump(true);
      } else {
        clearSavedPosition();
        el.scrollTop = el.scrollHeight;
      }
      prevActivityCount.current = activityCount;
      activityReady.current = true;
      return;
    }
    if (stick.current) el.scrollTop = el.scrollHeight;
  });

  // Sending re-sticks the feed so the user's own message cannot land below the
  // viewport while they are reading older history.
  useEffect(() => {
    if (pendingCount > prevPendingCount.current) {
      stick.current = true;
      setShowJump(false);
      setUnseen(0);
      clearSavedPosition();
      const el = viewportRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    }
    prevPendingCount.current = pendingCount;
  }, [pendingCount]);

  useEffect(() => {
    if (!restored.current || loading) {
      prevActivityCount.current = activityCount;
      return;
    }
    if (!activityReady.current) {
      prevActivityCount.current = activityCount;
      activityReady.current = true;
      return;
    }
    const added = activityCount - prevActivityCount.current;
    if (added > 0 && !stick.current) {
      setUnseen((count) => count + added);
    }
    prevActivityCount.current = activityCount;
  }, [activityCount, loading]);

  const onScroll: UIEventHandler<HTMLDivElement> = () => {
    const el = viewportRef.current;
    if (!el || !restored.current) return;
    const nearBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX;
    stick.current = nearBottom;
    setShowJump(!nearBottom);
    if (nearBottom) {
      setUnseen(0);
      clearSavedPosition();
    } else if (scrollStorageKey) {
      try {
        storage.session.setItem(scrollStorageKey, String(el.scrollTop));
      } catch {
        // The in-memory interaction remains correct when storage is unavailable.
      }
    }
  };

  const jumpToBottom = () => {
    const el = viewportRef.current;
    if (!el) return;
    stick.current = true;
    setShowJump(false);
    setUnseen(0);
    clearSavedPosition();
    el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  };

  return {
    viewportRef,
    showJump,
    unseen,
    onScroll,
    jumpToBottom,
  };
}

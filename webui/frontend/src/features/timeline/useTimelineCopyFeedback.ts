import { useEffect, useRef, useState } from "react";
import { useAppServices } from "../../app/appServices";
import { copyText } from "../../clipboard";

export interface TimelineCopyFeedbackController {
  copied: boolean;
  copy: () => Promise<void>;
}

/** Owns clipboard and timer side effects outside timeline render leaves. */
export function useTimelineCopyFeedback(
  content: string,
): TimelineCopyFeedbackController {
  const { clock } = useAppServices();
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current !== null) clock.clearTimeout(resetTimer.current);
    },
    [clock],
  );

  return {
    copied,
    copy: async () => {
      await copyText(content);
      setCopied(true);
      if (resetTimer.current !== null) clock.clearTimeout(resetTimer.current);
      resetTimer.current = clock.setTimeout(() => {
        resetTimer.current = null;
        setCopied(false);
      }, 1200);
    },
  };
}

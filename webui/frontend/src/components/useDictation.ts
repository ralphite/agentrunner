import { useCallback, useEffect, useRef, useState } from "react";
import { AR } from "../api";

// useDictation is server-side dictation (INC-56, HANDA-PARITY #18): record with
// MediaRecorder, upload the clip through the existing /api/upload, then hand its
// path to `ar dictate`, which owns the provider call. The browser never talks to
// a provider — the transcript comes back as a composer text convenience. When
// MediaRecorder/getUserMedia is unavailable the caller falls back to the browser
// SpeechRecognition path (useVoice).
export function useDictation(
  onText: (text: string) => void,
  context: () => string,
  onError: (msg: string) => void,
) {
  const supported =
    typeof navigator !== "undefined" &&
    !!navigator.mediaDevices?.getUserMedia &&
    typeof (window as any).MediaRecorder !== "undefined";

  const [recording, setRecording] = useState(false);
  const [busy, setBusy] = useState(false); // uploading + transcribing after stop

  const recRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const streamRef = useRef<MediaStream | null>(null);
  const onTextRef = useRef(onText);
  const contextRef = useRef(context);
  const onErrorRef = useRef(onError);
  onTextRef.current = onText;
  contextRef.current = context;
  onErrorRef.current = onError;

  const releaseMic = () => {
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = null;
  };

  useEffect(() => () => releaseMic(), []);

  const transcribe = useCallback(async (blob: Blob, mimeType: string) => {
    setBusy(true);
    try {
      const file = new File([blob], "dictation." + extForMime(mimeType), { type: blob.type || mimeType });
      const up = await AR.upload(file);
      const { text } = await AR.dictate(up.path, contextRef.current());
      const t = (text || "").trim();
      if (t) onTextRef.current(t);
    } catch (e: any) {
      onErrorRef.current(e?.message || "dictation failed");
    } finally {
      setBusy(false);
    }
  }, []);

  const start = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      const mimeType = pickRecorderMime();
      const rec = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
      chunksRef.current = [];
      rec.ondataavailable = (e) => {
        if (e.data && e.data.size) chunksRef.current.push(e.data);
      };
      rec.onstop = () => {
        releaseMic();
        const type = rec.mimeType || mimeType || "audio/webm";
        const blob = new Blob(chunksRef.current, { type });
        chunksRef.current = [];
        recRef.current = null;
        if (blob.size) void transcribe(blob, type);
      };
      rec.onerror = () => {
        releaseMic();
        setRecording(false);
        onErrorRef.current("recording failed");
      };
      rec.start();
      recRef.current = rec;
      setRecording(true);
    } catch (e: any) {
      releaseMic();
      setRecording(false);
      onErrorRef.current(e?.name === "NotAllowedError" ? "Microphone permission denied" : "Could not start recording");
    }
  }, [transcribe]);

  const toggle = useCallback(() => {
    if (busy) return;
    const rec = recRef.current;
    if (recording && rec) {
      try {
        rec.stop();
      } catch {
        releaseMic();
      }
      setRecording(false);
    } else if (!recording) {
      void start();
    }
  }, [busy, recording, start]);

  return { supported, recording, busy, toggle };
}

// pickRecorderMime prefers a container Gemini accepts natively (Ogg) when the
// browser can record it (Firefox), else falls back to WebM (Chrome). Empty =
// let the browser choose its default.
function pickRecorderMime(): string {
  const MR: any = (window as any).MediaRecorder;
  const isSupported = (t: string) => typeof MR?.isTypeSupported === "function" && MR.isTypeSupported(t);
  for (const t of ["audio/ogg;codecs=opus", "audio/ogg", "audio/webm;codecs=opus", "audio/webm", "audio/mp4"]) {
    if (isSupported(t)) return t;
  }
  return "";
}

// extForMime maps a recorder MIME onto a file extension so `ar dictate` can
// infer the audio type from the uploaded filename.
function extForMime(mime: string): string {
  const base = mime.split(";")[0].trim();
  switch (base) {
    case "audio/ogg":
      return "ogg";
    case "audio/mp4":
      return "m4a";
    case "audio/wav":
    case "audio/wave":
      return "wav";
    case "audio/mpeg":
    case "audio/mp3":
      return "mp3";
    case "audio/webm":
    default:
      return "webm";
  }
}

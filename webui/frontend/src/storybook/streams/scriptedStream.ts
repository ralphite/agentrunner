import type { AppEventStream } from "../../app/appServices";
import { cloneFixture } from "../fixtures";

export type StreamStep =
  | {
      type: "message";
      data: unknown;
    }
  | {
      type: "end";
      data?: unknown;
    }
  | {
      type: "error";
      error?: string;
    };

export type StreamScripts = Record<string, readonly StreamStep[]>;

function messageData(data: unknown): string {
  return typeof data === "string" ? data : JSON.stringify(data);
}

export class ScriptedEventStream implements AppEventStream {
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly path: string;
  closed = false;

  private readonly listeners = new Map<string, Set<EventListener>>();

  constructor(path: string) {
    this.path = path;
  }

  addEventListener(type: string, listener: EventListener): void {
    if (this.closed) return;
    const listeners = this.listeners.get(type) ?? new Set<EventListener>();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  emitMessage(data: unknown): boolean {
    if (this.closed) return false;
    const event = new MessageEvent<string>("message", {
      data: messageData(data),
    });
    const listeners = [...(this.listeners.get("message") ?? [])];
    this.onmessage?.(event);
    this.dispatch(listeners, event);
    return true;
  }

  emitEnd(data?: unknown): boolean {
    if (this.closed) return false;
    const event = new MessageEvent<string>("end", {
      data: data === undefined ? "" : messageData(data),
    });
    this.dispatch([...(this.listeners.get("end") ?? [])], event);
    return true;
  }

  emitError(message = "Scripted stream error"): boolean {
    if (this.closed) return false;
    const event = typeof ErrorEvent === "undefined"
      ? new Event("error")
      : new ErrorEvent("error", { message });
    const listeners = [...(this.listeners.get("error") ?? [])];
    this.onerror?.(event);
    this.dispatch(listeners, event);
    return true;
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.onmessage = null;
    this.onerror = null;
    this.listeners.clear();
  }

  private dispatch(listeners: EventListener[], event: Event): void {
    for (const listener of listeners) listener(event);
  }
}

interface ControlledStream {
  stream: ScriptedEventStream;
  script: StreamStep[];
  cursor: number;
}

export interface StreamProgress {
  path: string;
  cursor: number;
  total: number;
  closed: boolean;
}

/**
 * Owns every stream opened by one story. `next` is synchronous and advances
 * exactly one scripted event, making the same script usable by a click-to-play
 * Demo or by an auto-player that supplies its own injected clock.
 */
export class ScriptedStreamController {
  private readonly scripts = new Map<string, StreamStep[]>();
  private readonly opened: ControlledStream[] = [];

  constructor(scripts: StreamScripts = {}) {
    for (const [path, script] of Object.entries(scripts)) {
      this.scripts.set(path, cloneFixture([...script]));
    }
  }

  open = (path: string): ScriptedEventStream => {
    const stream = new ScriptedEventStream(path);
    this.opened.push({
      stream,
      script: cloneFixture(this.scripts.get(path) ?? []),
      cursor: 0,
    });
    return stream;
  };

  setScript(path: string, script: readonly StreamStep[]): void {
    this.scripts.set(path, cloneFixture([...script]));
  }

  enqueue(target: string | ScriptedEventStream, ...steps: StreamStep[]): boolean {
    const controlled = this.find(target);
    if (!controlled || controlled.stream.closed) return false;
    controlled.script.push(...cloneFixture(steps));
    return true;
  }

  next(target?: string | ScriptedEventStream): StreamStep | null {
    const controlled = this.find(target);
    if (
      !controlled ||
      controlled.stream.closed ||
      controlled.cursor >= controlled.script.length
    ) {
      return null;
    }

    const step = controlled.script[controlled.cursor];
    const emitted = this.emit(controlled.stream, step);
    if (!emitted) return null;
    controlled.cursor += 1;
    return cloneFixture(step);
  }

  playAll(target?: string | ScriptedEventStream): StreamStep[] {
    const played: StreamStep[] = [];
    for (;;) {
      const step = this.next(target);
      if (!step) return played;
      played.push(step);
    }
  }

  progress(target?: string | ScriptedEventStream): StreamProgress | null {
    const controlled = this.find(target);
    return controlled
      ? {
          path: controlled.stream.path,
          cursor: controlled.cursor,
          total: controlled.script.length,
          closed: controlled.stream.closed,
        }
      : null;
  }

  streams(path?: string): ScriptedEventStream[] {
    return this.opened
      .filter((controlled) => path === undefined || controlled.stream.path === path)
      .map((controlled) => controlled.stream);
  }

  reset(): void {
    for (const controlled of this.opened) controlled.stream.close();
    this.opened.length = 0;
  }

  private find(
    target?: string | ScriptedEventStream,
  ): ControlledStream | undefined {
    if (target instanceof ScriptedEventStream) {
      return this.opened.find((controlled) => controlled.stream === target);
    }
    for (let index = this.opened.length - 1; index >= 0; index -= 1) {
      const controlled = this.opened[index];
      if (target === undefined || controlled.stream.path === target) {
        return controlled;
      }
    }
    return undefined;
  }

  private emit(stream: ScriptedEventStream, step: StreamStep): boolean {
    switch (step.type) {
      case "message":
        return stream.emitMessage(step.data);
      case "end":
        return stream.emitEnd(step.data);
      case "error":
        return stream.emitError(step.error);
    }
  }
}

export function createScriptedStreamController(
  scripts: StreamScripts = {},
): ScriptedStreamController {
  return new ScriptedStreamController(scripts);
}

import { AR } from "../api";
import type { AppEventStream, AppServices } from "../app/appServices";
import type { Theme } from "../theme";

export class MemoryStorage implements Storage {
  private readonly values = new Map<string, string>();

  constructor(seed: Record<string, string> = {}) {
    for (const [key, value] of Object.entries(seed)) this.values.set(key, value);
  }

  get length() {
    return this.values.size;
  }

  clear() {
    this.values.clear();
  }

  getItem(key: string) {
    return this.values.get(key) ?? null;
  }

  key(index: number) {
    return [...this.values.keys()][index] ?? null;
  }

  removeItem(key: string) {
    this.values.delete(key);
  }

  setItem(key: string, value: string) {
    this.values.set(key, String(value));
  }
}

export class StoryEventStream implements AppEventStream {
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  private readonly listeners = new Map<string, Set<EventListener>>();
  closed = false;

  addEventListener(type: string, listener: EventListener) {
    const listeners = this.listeners.get(type) ?? new Set<EventListener>();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  emit(type: string, data: unknown) {
    if (this.closed) return;
    const event = new MessageEvent(type, { data: JSON.stringify(data) });
    if (type === "message") this.onmessage?.(event);
    for (const listener of this.listeners.get(type) ?? []) listener(event);
  }

  fail() {
    if (this.closed) return;
    const event = new Event("error");
    this.onerror?.(event);
    for (const listener of this.listeners.get("error") ?? []) listener(event);
  }

  close() {
    this.closed = true;
    this.listeners.clear();
  }
}

export interface StoryAppServicesHarness {
  services: AppServices;
  local: MemoryStorage;
  session: MemoryStorage;
  streams: StoryEventStream[];
  navigation: {
    hash(): string;
    setHash(hash: string): void;
  };
}

export interface StoryAppServicesOptions {
  api?: AppServices["api"];
  local?: Record<string, string>;
  session?: Record<string, string>;
  hash?: string;
  theme?: Theme;
}

export function createStoryAppServices(
  options: StoryAppServicesOptions = {},
): StoryAppServicesHarness {
  const local = new MemoryStorage(options.local);
  const session = new MemoryStorage(options.session);
  const streams: StoryEventStream[] = [];
  const listeners = new Set<() => void>();
  const sequences = new Map<string, number>();
  let hash = options.hash ?? "";
  let theme = options.theme ?? "system";

  const navigation = {
    hash: () => hash,
    setHash: (next: string) => {
      if (hash === next) return;
      hash = next;
      for (const listener of listeners) listener();
    },
  };

  const services: AppServices = {
    api: options.api ?? AR,
    streams: {
      open: () => {
        const stream = new StoryEventStream();
        streams.push(stream);
        return stream;
      },
    },
    storage: { local, session },
    navigation: {
      hash: navigation.hash,
      setHash: navigation.setHash,
      replaceHash: navigation.setHash,
      listen: (listener) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
    },
    clock: {
      now: () => Date.now(),
      setTimeout: (callback, delay) => globalThis.setTimeout(callback, delay),
      clearTimeout: (handle) => globalThis.clearTimeout(handle),
      setInterval: (callback, delay) => globalThis.setInterval(callback, delay),
      clearInterval: (handle) => globalThis.clearInterval(handle),
    },
    ids: {
      next: (namespace) => {
        const next = (sequences.get(namespace) ?? 0) + 1;
        sequences.set(namespace, next);
        return next;
      },
      uuid: (namespace) => {
        const next = (sequences.get(namespace) ?? 0) + 1;
        sequences.set(namespace, next);
        return `${namespace}_${next}`;
      },
    },
    theme: {
      load: () => theme,
      save: (next) => {
        theme = next;
      },
    },
    notifications: {
      sessions: () => {},
      runs: () => {},
      requestPermission: () => {},
    },
  };

  return { services, local, session, streams, navigation };
}

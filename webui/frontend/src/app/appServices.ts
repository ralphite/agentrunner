import { createContext, createElement, useContext, type ReactNode } from "react";
import { AR } from "../api";
import { notifyRunChanges, notifySessionChanges, requestNotifyPermission } from "../notify";
import { loadTheme, saveTheme, type Theme } from "../theme";
import type { Run, Session } from "../types";

export interface AppEventStream {
  onmessage: ((event: MessageEvent<string>) => void) | null;
  onerror: ((event: Event) => void) | null;
  addEventListener(type: string, listener: EventListener): void;
  close(): void;
}

export interface AppServices {
  api: typeof AR;
  streams: {
    open(path: string): AppEventStream;
  };
  storage: {
    local: Storage;
    session: Storage;
  };
  navigation: {
    hash(): string;
    setHash(hash: string): void;
    replaceHash(hash: string): void;
    listen(listener: () => void): () => void;
  };
  clock: {
    now(): number;
    setTimeout(callback: () => void, delay: number): ReturnType<typeof setTimeout>;
    clearTimeout(handle: ReturnType<typeof setTimeout>): void;
    setInterval(callback: () => void, delay: number): ReturnType<typeof setInterval>;
    clearInterval(handle: ReturnType<typeof setInterval>): void;
  };
  ids: {
    next(namespace: string): number;
    uuid(namespace: string): string;
  };
  theme: {
    load(): Theme;
    save(theme: Theme): void;
  };
  notifications: {
    sessions(previous: Session[], next: Session[], currentSid: string | null): void;
    runs(previous: Run[], next: Run[]): void;
    requestPermission(): void;
  };
}

function fallbackStorage(): Storage {
  const values = new Map<string, string>();
  return {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => {
      values.delete(key);
    },
    setItem: (key, value) => {
      values.set(key, String(value));
    },
  };
}

export function createProductionAppServices(): AppServices {
  const sequences = new Map<string, number>();
  // Node-based model tests import the production store without a DOM. Use an
  // ephemeral adapter only in that non-browser environment; a real browser
  // still captures its exact local/sessionStorage objects and keys.
  const local = typeof globalThis.localStorage === "undefined"
    ? fallbackStorage()
    : globalThis.localStorage;
  const session = typeof globalThis.sessionStorage === "undefined"
    ? fallbackStorage()
    : globalThis.sessionStorage;
  return {
    api: AR,
    streams: {
      open: (path) => new EventSource(path),
    },
    storage: {
      local,
      session,
    },
    navigation: {
      hash: () => location.hash.replace(/^#/, ""),
      setHash: (hash) => {
        location.hash = hash;
      },
      replaceHash: (hash) => {
        const suffix = hash ? `#${hash}` : "";
        if (typeof history !== "undefined" && typeof history.replaceState === "function") {
          history.replaceState(null, "", `${location.pathname}${location.search}${suffix}`);
        } else {
          location.hash = hash;
        }
      },
      listen: (listener) => {
        window.addEventListener("hashchange", listener);
        return () => window.removeEventListener("hashchange", listener);
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
        if (typeof globalThis.crypto?.randomUUID === "function") {
          return `${namespace}_${globalThis.crypto.randomUUID().replaceAll("-", "_")}`;
        }
        const next = (sequences.get(namespace) ?? 0) + 1;
        sequences.set(namespace, next);
        return `${namespace}_${next}`;
      },
    },
    theme: {
      load: () => loadTheme(local),
      save: (theme) => saveTheme(theme, local),
    },
    notifications: {
      sessions: notifySessionChanges,
      runs: notifyRunChanges,
      requestPermission: requestNotifyPermission,
    },
  };
}

export const productionAppServices = createProductionAppServices();

const AppServicesContext = createContext<AppServices>(productionAppServices);

export interface AppServicesProviderProps {
  services: AppServices;
  children: ReactNode;
}

export function AppServicesProvider({ services, children }: AppServicesProviderProps) {
  return createElement(AppServicesContext.Provider, { value: services }, children);
}

export function useAppServices(): AppServices {
  return useContext(AppServicesContext);
}

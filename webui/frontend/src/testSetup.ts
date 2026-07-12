// Node 25 exposes an experimental global localStorage that can shadow jsdom's
// Storage object inside Vitest workers. Component tests expect browser storage;
// when the environment gives us an incomplete storage object, install a small
// in-memory Storage-compatible replacement.
function memoryStorage(): Storage {
  const data = new Map<string, string>();
  return {
    get length() {
      return data.size;
    },
    clear() {
      data.clear();
    },
    getItem(key: string) {
      return data.has(key) ? data.get(key)! : null;
    },
    key(index: number) {
      return [...data.keys()][index] ?? null;
    },
    removeItem(key: string) {
      data.delete(key);
    },
    setItem(key: string, value: string) {
      data.set(key, String(value));
    },
  };
}

function usableStorage(candidate: Storage | undefined): Storage {
  return candidate && typeof candidate.clear === "function" ? candidate : memoryStorage();
}

if (typeof window !== "undefined") {
  const local = usableStorage(window.localStorage);
  const session = usableStorage(window.sessionStorage);
  Object.defineProperty(window, "localStorage", { configurable: true, value: local });
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: local });
  Object.defineProperty(window, "sessionStorage", { configurable: true, value: session });
  Object.defineProperty(globalThis, "sessionStorage", { configurable: true, value: session });
}

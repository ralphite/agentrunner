import type { AppServices } from "../../app/appServices";

type AppClock = AppServices["clock"];
type TimerHandle = ReturnType<typeof setTimeout>;

interface Timer {
  id: number;
  due: number;
  interval: number | null;
  callback: () => void;
}

export class ScenarioClock implements AppClock {
  private time: number;
  private nextId = 1;
  private readonly timers = new Map<number, Timer>();

  constructor(start = Date.parse("2026-01-15T12:00:00Z")) {
    this.time = start;
  }

  now = () => this.time;

  setTimeout = (callback: () => void, delay: number): TimerHandle =>
    this.schedule(callback, delay, null);

  clearTimeout = (handle: TimerHandle) => {
    this.timers.delete(Number(handle));
  };

  setInterval = (callback: () => void, delay: number): TimerHandle =>
    this.schedule(callback, delay, Math.max(1, delay));

  clearInterval = (handle: TimerHandle) => {
    this.timers.delete(Number(handle));
  };

  async advanceBy(delay: number): Promise<void> {
    if (!Number.isFinite(delay) || delay < 0) {
      throw new Error(`ScenarioClock delay must be finite and non-negative: ${delay}`);
    }
    const target = this.time + delay;
    for (;;) {
      const timer = [...this.timers.values()]
        .filter((candidate) => candidate.due <= target)
        .sort((left, right) => left.due - right.due || left.id - right.id)[0];
      if (!timer) break;

      this.time = timer.due;
      if (timer.interval === null) {
        this.timers.delete(timer.id);
      } else {
        timer.due += timer.interval;
      }
      timer.callback();
      await Promise.resolve();
    }
    this.time = target;
    await Promise.resolve();
  }

  reset(start = Date.parse("2026-01-15T12:00:00Z")) {
    this.timers.clear();
    this.time = start;
    this.nextId = 1;
  }

  private schedule(
    callback: () => void,
    delay: number,
    interval: number | null,
  ): TimerHandle {
    const id = this.nextId++;
    this.timers.set(id, {
      id,
      due: this.time + Math.max(0, Number.isFinite(delay) ? delay : 0),
      interval,
      callback,
    });
    return id as unknown as TimerHandle;
  }
}

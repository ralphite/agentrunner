import { describe, expect, it, vi } from "vitest";
import {
  createDemoScenarioTiming,
  instantScenarioTiming,
  ScenarioExecutionError,
  ScenarioOwnerError,
  ScenarioRunner,
  ScenarioStateError,
  type DemoStep,
  type ScenarioScheduler,
  type ScenarioSnapshot,
} from "./ScenarioRunner";

interface Deferred<Value> {
  promise: Promise<Value>;
  resolve(value: Value): void;
  reject(error: unknown): void;
}

function deferred<Value = void>(): Deferred<Value> {
  let resolve!: (value: Value) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<Value>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function noValue(deferredValue: Deferred<void>): void {
  deferredValue.resolve();
}

class ControlledScheduler implements ScenarioScheduler {
  readonly waits: Array<{
    delayMs: number;
    signal: AbortSignal;
    deferred: Deferred<void>;
  }> = [];

  sleep(delayMs: number, signal: AbortSignal): Promise<void> {
    const wait = deferred();
    const abort = () =>
      wait.reject(new DOMException("controlled wait aborted", "AbortError"));
    signal.addEventListener("abort", abort, { once: true });
    this.waits.push({ delayMs, signal, deferred: wait });
    return wait.promise.finally(() =>
      signal.removeEventListener("abort", abort),
    );
  }
}

describe("ScenarioRunner", () => {
  it("publishes the complete state-machine journey and current step", async () => {
    const calls: string[] = [];
    const snapshots: ScenarioSnapshot[] = [];
    const runner = new ScenarioRunner({
      context: { prefix: "ctx" },
      steps: [
        {
          id: "open",
          title: "Open the page",
          run: async (context) => {
            await Promise.resolve();
            calls.push(`${context.prefix}:open`);
          },
        },
        {
          id: "submit",
          run: (context) => {
            calls.push(`${context.prefix}:submit`);
          },
        },
      ],
    });
    runner.subscribe((snapshot) => snapshots.push(snapshot));

    expect(runner.getSnapshot()).toMatchObject({
      status: "idle",
      stepIndex: 0,
      currentStep: { id: "open", title: "Open the page", index: 0 },
      completedSteps: 0,
    });

    await runner.play("autoplay");

    expect(calls).toEqual(["ctx:open", "ctx:submit"]);
    expect(runner.getSnapshot()).toMatchObject({
      status: "completed",
      stepIndex: 2,
      currentStep: null,
      completedSteps: 2,
      owner: null,
    });
    expect(snapshots.map(({ status }) => status)).toEqual([
      "running",
      "running",
      "completed",
    ]);
    expect(snapshots[1].currentStep?.id).toBe("submit");
  });

  it("pauses an abortable wait and resumes with the selected speed", async () => {
    const scheduler = new ControlledScheduler();
    const run = vi.fn();
    const runner = new ScenarioRunner({
      context: undefined,
      steps: [{ id: "one", run }],
      timing: createDemoScenarioTiming(1_000),
      scheduler,
    });

    const firstPlay = runner.play();
    expect(scheduler.waits).toHaveLength(1);
    expect(scheduler.waits[0].delayMs).toBe(1_000);

    expect(runner.pause()).toBe(true);
    await firstPlay;
    expect(scheduler.waits[0].signal.aborted).toBe(true);
    expect(runner.getSnapshot()).toMatchObject({
      status: "paused",
      stepIndex: 0,
      owner: null,
    });
    expect(run).not.toHaveBeenCalled();

    runner.setSpeed(2);
    const resumed = runner.play();
    expect(scheduler.waits[1].delayMs).toBe(500);
    noValue(scheduler.waits[1].deferred);
    await resumed;

    expect(run).toHaveBeenCalledOnce();
    expect(runner.getSnapshot()).toMatchObject({
      status: "completed",
      speed: 2,
    });
  });

  it("honors Pause at the boundary of an asynchronous atomic step", async () => {
    const firstStep = deferred();
    const calls: string[] = [];
    const runner = new ScenarioRunner({
      context: undefined,
      steps: [
        {
          id: "async-action",
          run: async () => {
            calls.push("async:start");
            await firstStep.promise;
            calls.push("async:end");
          },
        },
        {
          id: "next-action",
          run: () => {
            calls.push("next");
          },
        },
      ],
    });

    const playing = runner.play();
    expect(calls).toEqual(["async:start"]);
    expect(runner.pause()).toBe(true);
    expect(runner.getSnapshot().status).toBe("running");

    noValue(firstStep);
    await playing;
    expect(runner.getSnapshot()).toMatchObject({
      status: "paused",
      stepIndex: 1,
      currentStep: { id: "next-action" },
    });
    expect(calls).toEqual(["async:start", "async:end"]);

    await runner.play();
    expect(calls).toEqual(["async:start", "async:end", "next"]);
    expect(runner.getSnapshot().status).toBe("completed");
  });

  it("advances exactly one step without presentation timing", async () => {
    const scheduler = new ControlledScheduler();
    const calls: number[] = [];
    const steps: DemoStep<void>[] = [0, 1, 2].map((index) => ({
      id: `step-${index}`,
      run: () => {
        calls.push(index);
      },
    }));
    const runner = new ScenarioRunner({
      context: undefined,
      steps,
      timing: createDemoScenarioTiming(2_000),
      scheduler,
    });

    await runner.next();
    expect(calls).toEqual([0]);
    expect(runner.getSnapshot()).toMatchObject({
      status: "paused",
      stepIndex: 1,
    });
    expect(scheduler.waits).toHaveLength(0);

    await runner.next();
    expect(calls).toEqual([0, 1]);
    expect(runner.getSnapshot().status).toBe("paused");

    await runner.next();
    expect(calls).toEqual([0, 1, 2]);
    expect(runner.getSnapshot().status).toBe("completed");
  });

  it("Reset and Replay dispose and recreate isolated contexts", async () => {
    const disposed: Array<[number, string]> = [];
    const seen: number[] = [];
    let nextContextId = 1;
    const runner = new ScenarioRunner({
      context: { id: nextContextId },
      steps: [
        {
          id: "read-context",
          run: (context) => {
            seen.push(context.id);
          },
        },
      ],
      recreateContext: () => ({ id: ++nextContextId }),
      disposeContext: (context, reason) => {
        disposed.push([context.id, reason]);
      },
    });

    await runner.next();
    expect(seen).toEqual([1]);

    await runner.reset();
    expect(disposed).toEqual([[1, "reset"]]);
    expect(runner.getContext().id).toBe(2);
    expect(runner.getSnapshot()).toMatchObject({
      status: "idle",
      stepIndex: 0,
      epoch: 1,
      error: null,
    });

    await runner.replay("interactions");
    expect(disposed).toEqual([
      [1, "reset"],
      [2, "reset"],
    ]);
    expect(runner.getContext().id).toBe(3);
    expect(seen).toEqual([1, 3]);
    expect(runner.getSnapshot()).toMatchObject({
      status: "completed",
      epoch: 2,
    });
  });

  it("ignores completion and failure from an aborted old epoch", async () => {
    const oldAction = deferred();
    const signals: AbortSignal[] = [];
    const seen: number[] = [];
    let contextId = 1;
    const runner = new ScenarioRunner({
      context: { id: contextId },
      steps: [
        {
          id: "late-action",
          run: async (context, signal) => {
            signals.push(signal);
            if (context.id === 1) await oldAction.promise;
            seen.push(context.id);
          },
        },
      ],
      recreateContext: () => ({ id: ++contextId }),
    });

    const oldPlay = runner.play();
    expect(signals).toHaveLength(1);
    await runner.reset();

    expect(signals[0].aborted).toBe(true);
    expect(runner.getSnapshot()).toMatchObject({
      status: "idle",
      stepIndex: 0,
      epoch: 1,
    });

    noValue(oldAction);
    await oldPlay;
    expect(seen).toEqual([1]);
    expect(runner.getSnapshot()).toMatchObject({
      status: "idle",
      stepIndex: 0,
      error: null,
    });

    await runner.play();
    expect(seen).toEqual([1, 2]);
    expect(runner.getSnapshot().status).toBe("completed");
  });

  it("surfaces a failed step in place and recovers only through Replay", async () => {
    const failure = new Error("mock stream failed");
    let attempt = 0;
    const runner = new ScenarioRunner({
      context: undefined,
      steps: [
        {
          id: "stream",
          run: () => {
            attempt += 1;
            if (attempt === 1) throw failure;
          },
        },
      ],
    });

    await runner.play();
    const failed = runner.getSnapshot();
    expect(failed).toMatchObject({
      status: "failed",
      stepIndex: 0,
      currentStep: { id: "stream" },
      owner: null,
    });
    expect(failed.error).toBeInstanceOf(ScenarioExecutionError);
    expect(failed.error).toMatchObject({
      phase: "step",
      step: { id: "stream", index: 0 },
      cause: failure,
    });
    expect(() => runner.play()).toThrow(ScenarioStateError);

    await runner.replay();
    expect(attempt).toBe(2);
    expect(runner.getSnapshot()).toMatchObject({
      status: "completed",
      error: null,
      epoch: 1,
    });
  });

  it("does not hide an atomic step failure behind a pending Pause", async () => {
    const action = deferred();
    const failure = new Error("click failed");
    const runner = new ScenarioRunner({
      context: undefined,
      steps: [{ id: "click", run: () => action.promise }],
    });

    const playing = runner.play();
    runner.pause();
    action.reject(failure);
    await playing;

    expect(runner.getSnapshot()).toMatchObject({
      status: "failed",
      stepIndex: 0,
      error: { phase: "step", cause: failure },
    });
  });

  it("reports reset errors and permits a clean reset retry", async () => {
    const failure = new Error("remount failed");
    let attempts = 0;
    const runner = new ScenarioRunner({
      context: { id: 1 },
      steps: [],
      recreateContext: () => {
        attempts += 1;
        if (attempts === 1) throw failure;
        return { id: 2 };
      },
    });

    await runner.reset();
    expect(runner.getSnapshot()).toMatchObject({
      status: "failed",
      stepIndex: 0,
      error: { phase: "reset", step: null, cause: failure },
    });

    await runner.reset();
    expect(runner.getContext()).toEqual({ id: 2 });
    expect(runner.getSnapshot()).toMatchObject({
      status: "idle",
      error: null,
      epoch: 2,
    });
  });

  it("cleans a context created after its reset epoch became stale", async () => {
    const replacement = deferred<{ id: number }>();
    const disposed: Array<[number, string]> = [];
    const runner = new ScenarioRunner({
      context: { id: 1 },
      steps: [],
      recreateContext: () => replacement.promise,
      disposeContext: (context, reason) => {
        disposed.push([context.id, reason]);
      },
    });

    const resetting = runner.reset();
    await Promise.resolve();
    await runner.dispose();
    replacement.resolve({ id: 2 });
    await resetting;

    expect(disposed).toEqual([
      [1, "reset"],
      [2, "stale"],
    ]);
    expect(runner.getSnapshot().status).toBe("disposed");
  });

  it("prevents two owners from running the same Canvas", async () => {
    const action = deferred();
    const runner = new ScenarioRunner({
      context: undefined,
      steps: [{ id: "owned", run: () => action.promise }],
    });

    const first = runner.play("autoplay");
    expect(runner.play("autoplay")).toBe(first);
    expect(() => runner.play("interactions")).toThrow(ScenarioOwnerError);
    expect(() => runner.next()).toThrow(ScenarioStateError);
    expect(() => runner.pause("manual")).toThrow(ScenarioOwnerError);

    expect(runner.pause("autoplay")).toBe(true);
    noValue(action);
    await first;
    expect(runner.getSnapshot().status).toBe("completed");
  });

  it("Dispose aborts active work, cleans the context once, and silences listeners", async () => {
    const action = deferred();
    const signals: AbortSignal[] = [];
    const disposed = vi.fn();
    const listener = vi.fn();
    const runner = new ScenarioRunner({
      context: { id: 1 },
      steps: [
        {
          id: "pending",
          run: async (_context, signal) => {
            signals.push(signal);
            await action.promise;
          },
        },
      ],
      disposeContext: disposed,
    });
    runner.subscribe(listener);

    const playing = runner.play();
    await runner.dispose();
    expect(signals[0].aborted).toBe(true);
    expect(runner.getSnapshot()).toMatchObject({
      status: "disposed",
      owner: null,
      error: null,
    });
    expect(disposed).toHaveBeenCalledOnce();
    expect(disposed).toHaveBeenCalledWith({ id: 1 }, "dispose");
    expect(
      listener.mock.calls[listener.mock.calls.length - 1]?.[0].status,
    ).toBe("disposed");

    noValue(action);
    await playing;
    expect(listener).toHaveBeenCalledTimes(2);
    expect(() => runner.play()).toThrow(ScenarioStateError);
    expect(() => runner.subscribe(() => {})).toThrow(ScenarioStateError);
    await runner.dispose();
    expect(disposed).toHaveBeenCalledOnce();
  });

  it("keeps fast CUJ timing separate and validates timing and speed inputs", () => {
    expect(instantScenarioTiming.beforeStepMs).toBe(0);
    expect(createDemoScenarioTiming().beforeStepMs).toBe(650);
    expect(
      typeof createDemoScenarioTiming(({ step }) =>
        step.id === "review" ? 3000 : 1200,
      ).beforeStepMs,
    ).toBe("function");
    expect(
      new ScenarioRunner({
        context: { kind: "typed-context" },
        steps: [],
        timing: instantScenarioTiming,
      }).getSnapshot().speed,
    ).toBe(1);
    expect(() => createDemoScenarioTiming(-1)).toThrow(RangeError);
    expect(
      () =>
        new ScenarioRunner({
          context: undefined,
          steps: [],
          initialSpeed: 0,
        }),
    ).toThrow(RangeError);

    const runner = new ScenarioRunner({ context: undefined, steps: [] });
    expect(() => runner.setSpeed(Number.POSITIVE_INFINITY)).toThrow(RangeError);
  });

  it("validates stable, unique step identifiers", () => {
    expect(
      () =>
        new ScenarioRunner({
          context: undefined,
          steps: [{ id: " ", run: () => {} }],
        }),
    ).toThrow("Scenario step id must not be empty");
    expect(
      () =>
        new ScenarioRunner({
          context: undefined,
          steps: [
            { id: "same", run: () => {} },
            { id: "same", run: () => {} },
          ],
        }),
    ).toThrow("Duplicate scenario step id: same");
  });
});

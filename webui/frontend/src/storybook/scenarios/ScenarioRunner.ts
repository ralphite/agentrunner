export type Awaitable<T> = T | PromiseLike<T>;

export type ScenarioOwner = "manual" | "autoplay" | "interactions";

export type ScenarioStatus =
  | "idle"
  | "running"
  | "paused"
  | "completed"
  | "failed"
  | "resetting"
  | "disposed";

export interface DemoStep<Context> {
  id: string;
  title?: string;
  run(context: Context, signal: AbortSignal): Awaitable<void>;
}

export interface ScenarioStepSnapshot {
  id: string;
  title?: string;
  index: number;
}

export interface ScenarioSnapshot {
  status: ScenarioStatus;
  stepIndex: number;
  stepCount: number;
  completedSteps: number;
  currentStep: ScenarioStepSnapshot | null;
  owner: ScenarioOwner | null;
  speed: number;
  epoch: number;
  error: ScenarioExecutionError | null;
}

export interface ScenarioTimingContext<Context> {
  context: Context;
  step: DemoStep<Context>;
  stepIndex: number;
}

export interface ScenarioTiming<Context> {
  beforeStepMs:
    | number
    | ((input: ScenarioTimingContext<Context>) => number);
}

// CI/CUJ playback opts into this timing explicitly. Scenario actions never own
// presentation delays, so the same scenario remains fast and deterministic.
export const instantScenarioTiming: ScenarioTiming<unknown> = Object.freeze({
  beforeStepMs: 0,
});

// Canvas demos can use human-visible pacing without slowing interaction tests.
export function createDemoScenarioTiming<Context>(
  beforeStepMs = 650,
): ScenarioTiming<Context> {
  assertDelay(beforeStepMs);
  return Object.freeze({ beforeStepMs });
}

export interface ScenarioScheduler {
  sleep(delayMs: number, signal: AbortSignal): Promise<void>;
}

export const systemScenarioScheduler: ScenarioScheduler = {
  sleep(delayMs, signal) {
    if (signal.aborted) return Promise.reject(createAbortError());
    if (delayMs === 0) return Promise.resolve();

    return new Promise<void>((resolve, reject) => {
      const timer = globalThis.setTimeout(() => {
        signal.removeEventListener("abort", onAbort);
        resolve();
      }, delayMs);
      const onAbort = () => {
        globalThis.clearTimeout(timer);
        signal.removeEventListener("abort", onAbort);
        reject(createAbortError());
      };
      signal.addEventListener("abort", onAbort, { once: true });
    });
  },
};

export type ScenarioDisposeReason = "reset" | "dispose" | "stale";

export interface ScenarioContextFactoryInput<Context> {
  previous: Context;
  signal: AbortSignal;
  epoch: number;
}

export interface ScenarioRunnerOptions<Context> {
  steps: readonly DemoStep<Context>[];
  context: Context;
  timing?: ScenarioTiming<Context>;
  scheduler?: ScenarioScheduler;
  initialSpeed?: number;
  recreateContext?: (
    input: ScenarioContextFactoryInput<Context>,
  ) => Awaitable<Context>;
  disposeContext?: (
    context: Context,
    reason: ScenarioDisposeReason,
  ) => Awaitable<void>;
}

export class ScenarioExecutionError extends Error {
  readonly phase: "step" | "reset";
  readonly step: ScenarioStepSnapshot | null;
  readonly cause: unknown;

  constructor(
    phase: "step" | "reset",
    cause: unknown,
    step: ScenarioStepSnapshot | null,
  ) {
    const detail = cause instanceof Error ? cause.message : String(cause);
    super(
      phase === "step"
        ? `Scenario step "${step?.id ?? "unknown"}" failed: ${detail}`
        : `Scenario reset failed: ${detail}`,
    );
    this.name = "ScenarioExecutionError";
    this.phase = phase;
    this.step = step;
    this.cause = cause;
  }
}

export class ScenarioStateError extends Error {
  constructor(action: string, status: ScenarioStatus) {
    super(`Cannot ${action} while scenario is ${status}`);
    this.name = "ScenarioStateError";
  }
}

export class ScenarioOwnerError extends Error {
  constructor(requested: ScenarioOwner, active: ScenarioOwner) {
    super(
      `Scenario is owned by "${active}"; "${requested}" cannot start a second run`,
    );
    this.name = "ScenarioOwnerError";
  }
}

type ScenarioListener = (snapshot: ScenarioSnapshot) => void;

interface ActiveExecution {
  epoch: number;
  owner: ScenarioOwner;
  inStep: boolean;
  pauseRequested: boolean;
  waitController: AbortController | null;
  promise: Promise<void>;
  resolve: () => void;
  reject: (error: unknown) => void;
}

export class ScenarioRunner<Context> {
  private readonly steps: readonly DemoStep<Context>[];
  private readonly timing: ScenarioTiming<Context>;
  private readonly scheduler: ScenarioScheduler;
  private readonly recreateContext?: ScenarioRunnerOptions<Context>["recreateContext"];
  private readonly disposeContext?: ScenarioRunnerOptions<Context>["disposeContext"];
  private readonly listeners = new Set<ScenarioListener>();

  private context: Context;
  private contextLive = true;
  private status: ScenarioStatus = "idle";
  private stepIndex = 0;
  private owner: ScenarioOwner | null = null;
  private speed: number;
  private epoch = 0;
  private error: ScenarioExecutionError | null = null;
  private epochController = new AbortController();
  private active: ActiveExecution | null = null;
  private resetPromise: Promise<void> | null = null;
  private disposePromise: Promise<void> | null = null;
  private snapshot: ScenarioSnapshot;

  constructor(options: ScenarioRunnerOptions<Context>) {
    validateSteps(options.steps);
    assertSpeed(options.initialSpeed ?? 1);

    this.steps = [...options.steps];
    this.context = options.context;
    this.timing = options.timing ?? ({ beforeStepMs: 0 } as ScenarioTiming<Context>);
    this.scheduler = options.scheduler ?? systemScenarioScheduler;
    this.speed = options.initialSpeed ?? 1;
    this.recreateContext = options.recreateContext;
    this.disposeContext = options.disposeContext;
    this.snapshot = this.createSnapshot();
  }

  getSnapshot = (): ScenarioSnapshot => this.snapshot;

  getContext(): Context {
    this.assertNotDisposed("read context");
    return this.context;
  }

  subscribe(listener: ScenarioListener): () => void {
    this.assertNotDisposed("subscribe");
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  setSpeed(speed: number): void {
    this.assertNotDisposed("change speed");
    assertSpeed(speed);
    if (speed === this.speed) return;
    this.speed = speed;
    this.publish();
  }

  play(owner: ScenarioOwner = "manual"): Promise<void> {
    this.assertNotDisposed("play");

    if (this.status === "running") {
      const active = this.requireActive();
      if (active.owner !== owner) {
        throw new ScenarioOwnerError(owner, active.owner);
      }
      return active.promise;
    }
    if (this.status !== "idle" && this.status !== "paused") {
      throw new ScenarioStateError("play", this.status);
    }

    const execution = this.claim(owner);
    void this.runContinuously(execution).then(
      execution.resolve,
      execution.reject,
    );
    return execution.promise;
  }

  pause(owner?: ScenarioOwner): boolean {
    this.assertNotDisposed("pause");
    if (this.status !== "running") return false;

    const active = this.requireActive();
    if (owner !== undefined && active.owner !== owner) {
      throw new ScenarioOwnerError(owner, active.owner);
    }

    active.pauseRequested = true;
    active.waitController?.abort();

    // Waiting is outside an atomic action, so it is safe to release ownership
    // immediately. A step already in flight keeps ownership until its boundary.
    if (!active.inStep) {
      this.active = null;
      this.owner = null;
      this.status = "paused";
      this.publish();
    }
    return true;
  }

  next(owner: ScenarioOwner = "manual"): Promise<void> {
    this.assertNotDisposed("advance");
    if (this.status !== "idle" && this.status !== "paused") {
      throw new ScenarioStateError("advance", this.status);
    }

    const execution = this.claim(owner);
    void this.runSingle(execution).then(execution.resolve, execution.reject);
    return execution.promise;
  }

  reset(): Promise<void> {
    this.assertNotDisposed("reset");
    if (this.resetPromise) return this.resetPromise;

    let resolveReset!: () => void;
    let rejectReset!: (error: unknown) => void;
    const resetWork = new Promise<void>((resolve, reject) => {
      resolveReset = resolve;
      rejectReset = reject;
    });
    this.resetPromise = resetWork;

    void this.performReset().then(
      () => {
        if (this.resetPromise === resetWork) this.resetPromise = null;
        resolveReset();
      },
      (error: unknown) => {
        if (this.resetPromise === resetWork) this.resetPromise = null;
        rejectReset(error);
      },
    );
    return resetWork;
  }

  async replay(owner: ScenarioOwner = "manual"): Promise<void> {
    this.assertNotDisposed("replay");
    await this.reset();
    if (this.status !== "idle") return;
    await this.play(owner);
  }

  dispose(): Promise<void> {
    if (this.disposePromise) return this.disposePromise;

    let resolveDispose!: () => void;
    let rejectDispose!: (error: unknown) => void;
    const disposeWork = new Promise<void>((resolve, reject) => {
      resolveDispose = resolve;
      rejectDispose = reject;
    });
    this.disposePromise = disposeWork;
    void this.performDispose().then(resolveDispose, rejectDispose);
    return disposeWork;
  }

  private claim(owner: ScenarioOwner): ActiveExecution {
    if (this.active) {
      if (this.active.owner !== owner) {
        throw new ScenarioOwnerError(owner, this.active.owner);
      }
      throw new ScenarioStateError("start another run", this.status);
    }

    let resolveExecution!: () => void;
    let rejectExecution!: (error: unknown) => void;
    const promise = new Promise<void>((resolve, reject) => {
      resolveExecution = resolve;
      rejectExecution = reject;
    });
    const execution: ActiveExecution = {
      epoch: this.epoch,
      owner,
      inStep: false,
      pauseRequested: false,
      waitController: null,
      promise,
      resolve: resolveExecution,
      reject: rejectExecution,
    };
    this.active = execution;
    this.owner = owner;
    this.status = "running";
    this.error = null;
    this.publish();
    return execution;
  }

  private async runContinuously(execution: ActiveExecution): Promise<void> {
    try {
      while (this.isCurrent(execution)) {
        if (this.stepIndex >= this.steps.length) {
          this.complete(execution);
          return;
        }

        const delayMs = this.resolveDelay();
        if (delayMs > 0) {
          const waitController = new AbortController();
          execution.waitController = waitController;
          try {
            await this.scheduler.sleep(delayMs, waitController.signal);
          } finally {
            if (execution.waitController === waitController) {
              execution.waitController = null;
            }
          }
        }

        if (!this.isCurrent(execution)) return;
        if (execution.pauseRequested) {
          this.finishPaused(execution);
          return;
        }

        const succeeded = await this.executeCurrentStep(execution);
        if (!succeeded || !this.isCurrent(execution)) return;
        if (this.stepIndex >= this.steps.length) {
          this.complete(execution);
          return;
        }
        if (execution.pauseRequested) {
          this.finishPaused(execution);
          return;
        }
        this.publish();
      }
    } catch (cause) {
      if (!this.isCurrent(execution)) return;
      this.failStep(execution, cause);
    }
  }

  private async runSingle(execution: ActiveExecution): Promise<void> {
    if (this.stepIndex >= this.steps.length) {
      this.complete(execution);
      return;
    }

    try {
      const succeeded = await this.executeCurrentStep(execution);
      if (!succeeded || !this.isCurrent(execution)) return;
      if (this.stepIndex >= this.steps.length) {
        this.complete(execution);
      } else {
        this.finishPaused(execution);
      }
    } catch (cause) {
      if (this.isCurrent(execution)) this.failStep(execution, cause);
    }
  }

  private async executeCurrentStep(
    execution: ActiveExecution,
  ): Promise<boolean> {
    const step = this.steps[this.stepIndex];
    if (!step) return false;

    execution.inStep = true;
    try {
      await step.run(this.context, this.epochController.signal);
    } finally {
      execution.inStep = false;
    }
    if (!this.isCurrent(execution)) return false;

    this.stepIndex += 1;
    return true;
  }

  private complete(execution: ActiveExecution): void {
    if (!this.isCurrent(execution)) return;
    this.active = null;
    this.owner = null;
    this.status = "completed";
    this.publish();
  }

  private finishPaused(execution: ActiveExecution): void {
    if (!this.isCurrent(execution)) return;
    this.active = null;
    this.owner = null;
    this.status = "paused";
    this.publish();
  }

  private failStep(execution: ActiveExecution, cause: unknown): void {
    if (!this.isCurrent(execution)) return;
    this.error = new ScenarioExecutionError(
      "step",
      cause,
      this.currentStepSnapshot(),
    );
    this.active = null;
    this.owner = null;
    this.status = "failed";
    this.publish();
  }

  private async performReset(): Promise<void> {
    const previous = this.context;
    const previousWasLive = this.contextLive;

    this.invalidateEpoch();
    const resetEpoch = this.epoch;
    const resetSignal = this.epochController.signal;
    this.stepIndex = 0;
    this.owner = null;
    this.error = null;
    this.status = "resetting";
    this.publish();

    try {
      if (this.recreateContext) {
        if (previousWasLive && this.disposeContext) {
          this.contextLive = false;
          await this.disposeContext(previous, "reset");
          if (!this.isEpochCurrent(resetEpoch)) return;
        }

        const next = await this.recreateContext({
          previous,
          signal: resetSignal,
          epoch: resetEpoch,
        });
        if (!this.isEpochCurrent(resetEpoch)) {
          await this.disposeContext?.(next, "stale");
          return;
        }
        this.context = next;
        this.contextLive = true;
      }

      if (!this.isEpochCurrent(resetEpoch)) return;
      this.status = "idle";
      this.publish();
    } catch (cause) {
      if (!this.isEpochCurrent(resetEpoch)) return;
      this.error = new ScenarioExecutionError("reset", cause, null);
      this.owner = null;
      this.status = "failed";
      this.publish();
    }
  }

  private async performDispose(): Promise<void> {
    if (this.status === "disposed") return;

    this.invalidateEpoch();
    const context = this.context;
    const shouldDisposeContext = this.contextLive;
    this.contextLive = false;
    this.owner = null;
    this.error = null;
    this.status = "disposed";
    this.publish();
    this.listeners.clear();

    if (shouldDisposeContext) {
      await this.disposeContext?.(context, "dispose");
    }
  }

  private invalidateEpoch(): void {
    this.epochController.abort();
    this.active?.waitController?.abort();
    this.active = null;
    this.epoch += 1;
    this.epochController = new AbortController();
  }

  private resolveDelay(): number {
    const step = this.steps[this.stepIndex];
    if (!step) return 0;

    const configured =
      typeof this.timing.beforeStepMs === "function"
        ? this.timing.beforeStepMs({
            context: this.context,
            step,
            stepIndex: this.stepIndex,
          })
        : this.timing.beforeStepMs;
    assertDelay(configured);
    return configured / this.speed;
  }

  private isCurrent(execution: ActiveExecution): boolean {
    return (
      this.status !== "disposed" &&
      this.active === execution &&
      execution.epoch === this.epoch &&
      !this.epochController.signal.aborted
    );
  }

  private isEpochCurrent(epoch: number): boolean {
    return (
      this.status !== "disposed" &&
      epoch === this.epoch &&
      !this.epochController.signal.aborted
    );
  }

  private requireActive(): ActiveExecution {
    if (!this.active) {
      throw new ScenarioStateError("access the active run", this.status);
    }
    return this.active;
  }

  private assertNotDisposed(action: string): void {
    if (this.status === "disposed") {
      throw new ScenarioStateError(action, "disposed");
    }
  }

  private publish(): void {
    this.snapshot = this.createSnapshot();
    for (const listener of this.listeners) listener(this.snapshot);
  }

  private createSnapshot(): ScenarioSnapshot {
    return Object.freeze({
      status: this.status,
      stepIndex: this.stepIndex,
      stepCount: this.steps.length,
      completedSteps: this.stepIndex,
      currentStep: this.currentStepSnapshot(),
      owner: this.owner,
      speed: this.speed,
      epoch: this.epoch,
      error: this.error,
    });
  }

  private currentStepSnapshot(): ScenarioStepSnapshot | null {
    const step = this.steps[this.stepIndex];
    if (!step) return null;
    return Object.freeze({
      id: step.id,
      title: step.title,
      index: this.stepIndex,
    });
  }
}

function validateSteps<Context>(steps: readonly DemoStep<Context>[]): void {
  const ids = new Set<string>();
  for (const step of steps) {
    if (step.id.trim() === "") {
      throw new TypeError("Scenario step id must not be empty");
    }
    if (ids.has(step.id)) {
      throw new TypeError(`Duplicate scenario step id: ${step.id}`);
    }
    ids.add(step.id);
  }
}

function assertSpeed(speed: number): void {
  if (!Number.isFinite(speed) || speed <= 0) {
    throw new RangeError("Scenario speed must be a finite number greater than 0");
  }
}

function assertDelay(delayMs: number): void {
  if (!Number.isFinite(delayMs) || delayMs < 0) {
    throw new RangeError(
      "Scenario delay must be a finite number greater than or equal to 0",
    );
  }
}

function createAbortError(): DOMException {
  return new DOMException("Scenario wait was aborted", "AbortError");
}

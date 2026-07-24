import { useSyncExternalStore } from "react";
import type { ScenarioRunner, ScenarioSnapshot } from "./ScenarioRunner";
import "./ScenarioControls.css";

export interface ScenarioControlsProps<Context> {
  runner: ScenarioRunner<Context>;
  label?: string;
  autoPlay?: boolean;
  onAutoPlayChange?: (enabled: boolean) => void;
}

function run(action: () => Promise<void>) {
  void action().catch(() => {
    // ScenarioRunner publishes the durable failed snapshot; the controls keep
    // the failure visible in-canvas instead of creating an unhandled rejection.
  });
}

function canPlay(snapshot: ScenarioSnapshot) {
  return snapshot.status === "idle" || snapshot.status === "paused";
}

function canReset(snapshot: ScenarioSnapshot) {
  return snapshot.status !== "resetting" && snapshot.status !== "disposed";
}

export function ScenarioControls<Context>({
  runner,
  label = "Demo playback",
  autoPlay = false,
  onAutoPlayChange,
}: ScenarioControlsProps<Context>) {
  const snapshot = useSyncExternalStore(
    runner.subscribe.bind(runner),
    runner.getSnapshot,
    runner.getSnapshot,
  );

  return (
    <section className="scenario-controls" aria-label={label}>
      <div className="scenario-controls-actions">
        <button
          type="button"
          onClick={() => {
            onAutoPlayChange?.(false);
            run(() => runner.play("manual"));
          }}
          disabled={!canPlay(snapshot)}
        >
          Play
        </button>
        <button
          type="button"
          // A person always outranks the playback owner. Passing no owner lets
          // the same Pause control stop either a manual run or an autoplay run.
          onClick={() => {
            runner.pause();
            onAutoPlayChange?.(false);
          }}
          disabled={snapshot.status !== "running"}
        >
          Pause
        </button>
        <button
          type="button"
          onClick={() => {
            onAutoPlayChange?.(false);
            run(() => runner.next("manual"));
          }}
          disabled={!canPlay(snapshot)}
        >
          Next
        </button>
        <button
          type="button"
          onClick={() => {
            onAutoPlayChange?.(false);
            run(() => runner.reset());
          }}
          disabled={!canReset(snapshot)}
        >
          Reset
        </button>
        <button
          type="button"
          onClick={() => {
            onAutoPlayChange?.(false);
            run(() => runner.replay("manual"));
          }}
          disabled={
            snapshot.status === "running" ||
            snapshot.status === "resetting" ||
            snapshot.status === "disposed"
          }
        >
          Replay
        </button>
        {onAutoPlayChange && (
          <label>
            <input
              type="checkbox"
              aria-label="Autoplay"
              checked={autoPlay}
              disabled={snapshot.status === "running"}
              onChange={(event) => onAutoPlayChange(event.target.checked)}
            />
            Autoplay
          </label>
        )}
        <label>
          Speed
          <select
            aria-label="Playback speed"
            value={snapshot.speed}
            onChange={(event) => runner.setSpeed(Number(event.target.value))}
          >
            <option value={0.5}>0.5×</option>
            <option value={1}>1×</option>
            <option value={2}>2×</option>
          </select>
        </label>
      </div>
      <div className="scenario-controls-status" role="status" aria-live="polite">
        <b>{snapshot.status}</b>
        <span>
          Step {Math.min(snapshot.stepIndex + 1, snapshot.stepCount)} /{" "}
          {snapshot.stepCount}
        </span>
        {snapshot.currentStep?.title && <span>{snapshot.currentStep.title}</span>}
        {snapshot.error && <span role="alert">{snapshot.error.message}</span>}
      </div>
    </section>
  );
}

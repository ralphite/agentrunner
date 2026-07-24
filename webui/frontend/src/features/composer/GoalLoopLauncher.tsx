import {
  ArrowClockwise,
  ChartBar,
  Target,
  X,
} from "@phosphor-icons/react";
import { useState } from "react";
import { scheduleFieldError } from "../../scheduleValidate";
import { Button } from "../../ui/Button";
import { Input, Textarea } from "../../ui/Field";
import { IconButton } from "../../ui/IconButton";

export interface GoalLoopLauncherProps {
  mode: "goal" | "loop" | "best";
  initialPrompt: string;
  busy: boolean;
  onCancel: () => void;
  onStart: (prompt: string, second: string, iterations: number) => void;
}

/**
 * Owns the complete launcher form interaction for Goal, Loop and Best-of-N.
 *
 * The parent only orchestrates the resulting command. Draft fields, defaults,
 * cadence validation and the mode-specific presentation stay within this
 * feature boundary.
 */
export function GoalLoopLauncher({
  mode,
  initialPrompt,
  busy,
  onCancel,
  onStart,
}: GoalLoopLauncherProps) {
  const [prompt, setPrompt] = useState(initialPrompt);
  const [second, setSecond] = useState(mode === "loop" ? "5m" : "");
  const intervalError =
    mode === "loop" ? scheduleFieldError("interval", second) : "";
  const [iterations, setIterations] = useState(
    mode === "goal" ? 10 : mode === "loop" ? 5 : 3,
  );
  const meta = {
    goal: {
      icon: <Target size={14} />,
      label: "Goal",
      hint: "iterate until the goal is met",
      start: "Start goal",
    },
    loop: {
      icon: <ArrowClockwise size={14} />,
      label: "Loop",
      hint: "repeat on a fixed cadence",
      start: "Start loop",
    },
    best: {
      icon: <ChartBar size={14} />,
      label: "Best of N",
      hint: "N isolated attempts, the verifier picks the best",
      start: "Start best-of-N",
    },
  }[mode];

  return (
    <div className="cx-launcher">
      <div className="cx-launcher-hd">
        {meta.icon}
        <b>{meta.label}</b>
        <span className="dim">{meta.hint}</span>
        <span className="cx-spacer" />
        <IconButton
          size="sm"
          variant="ghost"
          onClick={onCancel}
          aria-label="Close launcher"
        >
          <X size={13} />
        </IconButton>
      </div>
      <Textarea
        className="cx-launcher-prompt"
        rows={2}
        placeholder={
          mode === "goal"
            ? "What goal should the agent keep working toward?"
            : mode === "loop"
              ? "What should each iteration do?"
              : "What should each attempt try to do?"
        }
        value={prompt}
        onChange={(event) => setPrompt(event.target.value)}
      />
      <div className="cx-launcher-row">
        {mode === "loop" ? (
          <label
            className="cx-launcher-field"
            title="How often to run (Go duration, e.g. 30s, 5m, 1h)"
          >
            <span>Every</span>
            <Input
              placeholder="5m"
              value={second}
              onChange={(event) => setSecond(event.target.value)}
            />
          </label>
        ) : (
          <label
            className="cx-launcher-field"
            title={
              mode === "goal"
                ? "A shell command that must exit 0 for the goal to count as met. Optional — leave it empty and the agent self-certifies: it calls goal_complete when the goal is verifiably done (audited at the turn boundary)"
                : "A shell command that judges each attempt — exit 0 = pass (optional; without it the earliest attempt wins)"
            }
          >
            <span>
              {mode === "goal"
                ? "Done when (command)"
                : "Judge with (command)"}
            </span>
            <Input
              placeholder={
                mode === "goal"
                  ? "e.g. go test ./…  (empty = agent self-certifies)"
                  : "e.g. go test ./…  (optional)"
              }
              value={second}
              onChange={(event) => setSecond(event.target.value)}
            />
          </label>
        )}
        <label
          className="cx-launcher-field small"
          title={
            mode === "best"
              ? "How many isolated attempts to run"
              : "Safety cap on iterations"
          }
        >
          <span>{mode === "best" ? "Attempts" : "Max rounds"}</span>
          <Input
            type="number"
            min={mode === "best" ? 2 : 1}
            value={iterations}
            onChange={(event) =>
              setIterations(
                Math.max(
                  mode === "best" ? 2 : 1,
                  Number(event.target.value) || 1,
                ),
              )
            }
          />
        </label>
        <Button
          variant="solid"
          className="cx-launcher-go"
          loading={busy}
          disabled={
            !prompt.trim() ||
            (mode === "loop" && (!second.trim() || intervalError !== ""))
          }
          onClick={() =>
            onStart(prompt.trim(), second.trim(), iterations)
          }
        >
          {meta.start}
        </Button>
      </div>
      {intervalError !== "" && (
        <div className="mt-1 text-[12px] leading-5 text-red" role="alert">
          {intervalError}
        </div>
      )}
    </div>
  );
}

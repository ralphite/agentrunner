import {
  ArrowClockwise,
  ChartBar,
  X,
} from "@phosphor-icons/react";
import { useState } from "react";
import { scheduleFieldError } from "../../scheduleValidate";
import { Button } from "../../ui/Button";
import { Input, Textarea } from "../../ui/Field";
import { IconButton } from "../../ui/IconButton";

export interface GoalLoopLauncherProps {
  // G58② (QA-0724-goal) · "goal" is gone: the composer's Goal chip
  // (GoalOptions) is the goal form's only home — this panel serving a second
  // goal entry point invited the two to drift.
  mode: "loop" | "best";
  initialPrompt: string;
  busy: boolean;
  onCancel: () => void;
  onStart: (prompt: string, second: string, iterations: number) => void;
}

/**
 * Owns the complete launcher form interaction for Loop and Best-of-N.
 *
 * The parent only orchestrates the resulting command. Draft fields, defaults,
 * cadence validation and the mode-specific presentation stay within this
 * feature boundary. (Goal lives on the composer's Goal chip — GoalOptions —
 * not here; G58②.)
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
  const [iterations, setIterations] = useState(mode === "loop" ? 5 : 3);
  const meta = {
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
          mode === "loop"
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
            title="A shell command that judges each attempt — exit 0 = pass (optional; without it the earliest attempt wins)"
          >
            <span>Judge with (command)</span>
            <Input
              placeholder="e.g. go test ./…  (optional)"
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

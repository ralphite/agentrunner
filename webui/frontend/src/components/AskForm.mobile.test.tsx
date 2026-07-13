// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import { AskForm } from "./AskForm";

afterEach(cleanup);

describe("AskForm mobile option rows", () => {
  it("keeps label and description in one full-width, left-aligned text hierarchy", () => {
    render(
      <AskForm
        questions={[
          {
            question: "Which rollout strategy should we use?",
            options: [
              {
                label: "Gradual rollout",
                description: "Release to a small cohort first and expand after monitoring errors.",
              },
            ],
          },
        ]}
        onSubmit={() => {}}
        onSkip={() => {}}
      />,
    );

    const option = screen.getByRole("button", { name: /Gradual rollout/ });
    const label = screen.getByText("Gradual rollout");
    const description = screen.getByText("Release to a small cohort first and expand after monitoring errors.");
    const copy = label.parentElement;

    expect(option.classList.contains("w-full")).toBe(true);
    expect(option.classList.contains("items-start")).toBe(true);
    expect(option.classList.contains("text-left")).toBe(true);
    expect(option.querySelector("svg")?.classList.contains("shrink-0")).toBe(true);
    expect(copy?.classList.contains("min-w-0")).toBe(true);
    expect(copy?.classList.contains("flex-1")).toBe(true);
    expect(description.parentElement).toBe(copy);
    expect(label.classList.contains("block")).toBe(true);
    expect(label.classList.contains("font-medium")).toBe(true);
    expect(description.classList.contains("block")).toBe(true);
    expect(description.classList.contains("text-dim")).toBe(true);
  });

  it("selects the whole row when its secondary description is tapped", () => {
    const onSubmit = vi.fn();
    render(
      <AskForm
        questions={[
          {
            question: "Which signal should block release?",
            options: [{ label: "Tap failures", description: "Any failed approval or question-card action." }],
          },
        ]}
        onSubmit={onSubmit}
        onSkip={() => {}}
      />,
    );

    fireEvent.click(screen.getByText("Any failed approval or question-card action."));

    const option = screen.getByRole("button", { name: /Tap failures/ });
    expect(option.classList.contains("sel")).toBe(true);
    expect(option.classList.contains("border-blue")).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(onSubmit).toHaveBeenCalledWith(["1:1"]);
  });
});

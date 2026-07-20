import { describe, expect, it } from "vitest";
import { friendlyStatus, terminalNoticeFor } from "./components/pill";

describe("abnormal terminal notices", () => {
  it("offers a checkpoint continuation for a normal session that exhausted its budget", () => {
    expect(terminalNoticeFor("limit_exceeded")).toMatchObject({
      title: "Budget limit reached",
      action: "continue",
      actionLabel: "Continue in new session",
    });
  });

  it("keeps scheduled budget exhaustion review-first", () => {
    expect(terminalNoticeFor("limit_exceeded", true)).toMatchObject({
      action: "inspect",
      actionLabel: "Run details",
    });
  });

  it("maps stranded sessions to the real resume action", () => {
    expect(terminalNoticeFor("stranded")).toMatchObject({
      title: "Session needs recovery",
      action: "resume",
    });
  });

  it("does not add noise to normal completed sessions", () => {
    expect(terminalNoticeFor("completed")).toBeNull();
  });

  // A goal's ending is not a session ending: Quiescence keeps
  // "goal_budget_exhausted" as the durable status by design, but the session
  // is still waiting for input. The banner used to substring-match "budget"
  // and pin a false "Budget limit reached · Continue in new session" card —
  // and the advertised fork inherited the exhausted goal, reproducing the
  // dead end (QA v2sim, 2026-07-20).
  it("leaves goal endings to the goal banner instead of a false terminal card", () => {
    expect(terminalNoticeFor("goal_budget_exhausted")).toBeNull();
    expect(terminalNoticeFor("goal_satisfied")).toBeNull();
  });

  it("labels a goal's exhausted check budget as the goal's, not the session's", () => {
    expect(friendlyStatus("goal_budget_exhausted")).toMatchObject({
      text: "Goal stopped — check budget",
      cls: "stranded",
    });
    expect(friendlyStatus("goal_satisfied")).toMatchObject({ text: "Goal completed" });
    // The genuine session-budget ending keeps its wording.
    expect(friendlyStatus("limit_exceeded (budget)")).toMatchObject({
      text: "Budget limit reached",
    });
  });
});

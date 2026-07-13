import { describe, expect, it } from "vitest";
import { terminalNoticeFor } from "./components/pill";

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
});

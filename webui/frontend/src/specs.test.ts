import { describe, expect, it } from "vitest";
import {
  ACCESS_LEVELS,
  DEFAULT_WORKER,
  PERSONAS,
  buildSpec,
  replaceModel,
  runtimeModeTarget,
} from "./specs";

// The session mode pill (INC-54) is only as honest as this map: which access
// levels the daemon will accept as a mid-session switch (INC-42's
// ValidTransition = default↔acceptEdits), and which are launch-time only.
describe("runtimeModeTarget", () => {
  it("maps Ask to approve to the /mode default target", () => {
    expect(runtimeModeTarget("ask")).toBe("default");
  });

  it("maps Auto-accept edits to the /mode acceptEdits target", () => {
    expect(runtimeModeTarget("acceptEdits")).toBe("acceptEdits");
  });

  it("refuses Full access at runtime (launch-time spec posture, not a fold mode)", () => {
    expect(runtimeModeTarget("full")).toBeNull();
  });

  it("refuses Plan at runtime (exits via exit_plan_mode approval, not a switch)", () => {
    expect(runtimeModeTarget("plan")).toBeNull();
  });

  it("keeps the two clickable levels a strict subset of the offered rows", () => {
    const clickable = ACCESS_LEVELS.filter((a) => runtimeModeTarget(a.id) !== null).map((a) => a.id);
    expect(clickable).toEqual(["ask", "acceptEdits"]);
  });
});

describe("shipped agent YAML", () => {
  it("keeps every picker persona as a complete, distinct YAML definition", () => {
    expect(PERSONAS.map((persona) => persona.id)).toEqual(["dev", "lead", "auditor", "reviewer", "chat"]);

    for (const persona of PERSONAS) {
      expect(persona.spec).toContain(`name: ${persona.id}\n`);
      expect(persona.spec).toContain("model:");
      expect(persona.spec).toContain("system_prompt:");
      expect(persona.spec).toContain("tools:");
      expect(persona.spec).toContain("permissions:");
    }
    expect(DEFAULT_WORKER).toContain("name: worker\n");
    expect(DEFAULT_WORKER).toContain("max_generation_steps: 24");
  });

  it("overrides only model and permissions while preserving each YAML persona body", () => {
    const markers: Record<string, string> = {
      dev: "agents: [worker]",
      lead: "agents_dynamic: true",
      auditor: "Always open with \"[AUDITOR]\"",
      reviewer: "produce structured findings",
      chat: "tools: []",
    };

    for (const persona of PERSONAS) {
      const spec = buildSpec({
        provider: "anthropic",
        model: "claude-sonnet-5",
        access: "ask",
        persona: persona.id,
        effort: "high",
      });
      expect(spec).toContain(`name: ${persona.id}\n`);
      expect(spec).toContain("provider: anthropic");
      expect(spec).toContain("id: claude-sonnet-5");
      expect(spec).toContain("budget_tokens: 12288");
      expect(spec).toContain(markers[persona.id]);
      expect(spec).toContain("  - { tool: read_file, action: allow }");
      expect(spec).not.toContain("  - { action: allow }");
    }
  });

  it("can replace a readable multiline model block without touching the rest", () => {
    const original = `name: custom
model:
  provider: gemini
  id: old
  max_tokens: 99
system_prompt: Keep this exact prompt.
tools: []
`;
    const updated = replaceModel(original, "anthropic", "claude-sonnet-5", "light");
    expect(updated).toContain("model: { provider: anthropic, id: claude-sonnet-5");
    expect(updated).toContain("system_prompt: Keep this exact prompt.");
    expect(updated).toContain("tools: []");
    expect(updated).not.toContain("id: old");
  });
});

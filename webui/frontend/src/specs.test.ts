import { describe, expect, it } from "vitest";
import type { AgentCatalogEntry } from "./types";
import {
  ACCESS_LEVELS,
  agentById,
  buildSpec,
  legacyModelFromSpec,
  runtimeModeTarget,
  stripLegacyModel,
} from "./specs";

describe("runtimeModeTarget", () => {
  it("maps every posture to a runtime mode, with Ask and Full sharing default", () => {
    // Ask and Full both run under `default` — the permissions block in the
    // spec is what separates them, so a switch between the two is a spec
    // rewrite, not a mode command. Every posture is reachable at any time.
    expect(runtimeModeTarget("ask")).toBe("default");
    expect(runtimeModeTarget("full")).toBe("default");
    expect(runtimeModeTarget("acceptEdits")).toBe("acceptEdits");
    expect(runtimeModeTarget("plan")).toBe("plan");
    expect(ACCESS_LEVELS.every((a) => runtimeModeTarget(a.id) !== null)).toBe(true);
  });

  it("gives Full and Ask different permissions so the shared mode still gates differently", () => {
    const agent: AgentCatalogEntry = {
      name: "dev",
      description: "d",
      source: "shipped",
      yaml: "name: dev\nsystem_prompt: p\ntools: []\npermissions:\n  - { action: ask }\n",
    };
    expect(buildSpec({ agent, access: "full" })).toContain("{ action: allow }");
    expect(buildSpec({ agent, access: "ask" })).toContain("{ action: ask }");
  });
});

const catalog: AgentCatalogEntry[] = [
  {
    name: "dev",
    description: "Runtime-owned dev",
    source: "shipped",
    yaml: `name: dev
description: Runtime-owned dev
system_prompt: Keep this exact prompt.
tools: [read_file, bash]
agents: [worker]
permissions:
  - { action: allow }
`,
  },
  {
    name: "mine",
    description: "User override",
    source: "user",
    yaml: "name: mine\nsystem_prompt: User configured.\ntools: []\n",
  },
];

describe("shared Agent catalog projection", () => {
  it("selects runtime/user entries without frontend-owned YAML", () => {
    expect(agentById(catalog, "mine")).toBe(catalog[1]);
    expect(agentById(catalog, "missing")).toBe(catalog[0]);
  });

  it("only overlays permissions and never inserts model selection", () => {
    const spec = buildSpec({ agent: catalog[0], access: "ask" });
    expect(spec).toContain("system_prompt: Keep this exact prompt.");
    expect(spec).toContain("agents: [worker]");
    expect(spec).toContain("  - { tool: read_file, action: allow }");
    expect(spec).not.toContain("model:");
    expect(spec).not.toContain("  - { action: allow }");
  });


  it("migrates browser-local legacy Agent YAML without resubmitting model", () => {
    const legacy = `name: old
model: { provider: anthropic, id: claude-sonnet-5, max_tokens: 16384, thinking: { enabled: true, budget_tokens: 12288 } }
system_prompt: Preserve me.
tools: []
`;
    expect(legacyModelFromSpec(legacy)).toEqual({
      provider: "anthropic",
      model: "claude-sonnet-5",
      effort: "high",
    });
    const definition = stripLegacyModel(legacy);
    expect(definition).not.toContain("model:");
    expect(definition).toContain("system_prompt: Preserve me.");
  });
});

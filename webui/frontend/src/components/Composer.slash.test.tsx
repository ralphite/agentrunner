// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { dynamicSlash, SLASH, type SlashCatalog } from "../features/composer/slash";
import { SlashCommandMenu } from "../features/composer/ComposerParts";

// The "/" menu's dynamic surface: workspace commands and skills join the
// built-in table; both only complete "/name " (ingest expands server-side).
describe("dynamicSlash", () => {
  const catalog: SlashCatalog = {
    commands: [{ name: "ship", description: "Ship it" }],
    skills: [
      { name: "create-agent", description: "Create a custom agent", source: "shipped" },
      { name: "ship", description: "shadowed by the command", source: "workspace" },
      { name: "goal", description: "shadowed by a built-in", source: "workspace" },
    ],
  };

  it("maps commands and skills to needsArgs rows in both variants", () => {
    const rows = dynamicSlash(catalog);
    const byName = Object.fromEntries(rows.map((r) => [r.name, r]));
    expect(byName["ship"].group).toBe("command");
    expect(byName["create-agent"].group).toBe("skill");
    expect(byName["create-agent"].source).toBe("shipped");
    for (const r of rows) {
      expect(r.needsArgs).toBe(true);
      expect(r.variants).toEqual(["home", "session"]);
    }
  });

  it("drops names shadowed by built-ins and commands", () => {
    const rows = dynamicSlash(catalog);
    expect(rows.filter((r) => r.name === "ship")).toHaveLength(1); // command wins over skill
    expect(rows.find((r) => r.name === "goal")).toBeUndefined(); // built-in wins
    expect(SLASH.find((c) => c.name === "goal")).toBeTruthy();
  });

  it("degrades to empty on a missing catalog", () => {
    expect(dynamicSlash(null)).toEqual([]);
  });
});

describe("SlashCommandMenu grouping", () => {
  it("renders section headers per group and a source tag on skills", () => {
    const commands = [
      ...SLASH.filter((c) => c.name === "goal"),
      ...dynamicSlash({
        commands: [{ name: "ship", description: "Ship it" }],
        skills: [{ name: "create-agent", description: "Create a custom agent", source: "shipped" }],
      }),
    ];
    const { container } = render(
      <SlashCommandMenu
        commands={commands}
        activeIndex={0}
        onActiveIndexChange={vi.fn()}
        onSelect={vi.fn()}
      />,
    );
    const headers = [...container.querySelectorAll(".cx-slash-hd")].map((h) => h.textContent);
    expect(headers).toEqual(["Commands", "Workspace commands", "Skills"]);
    expect(screen.getByText("/create-agent")).toBeTruthy();
    expect(screen.getByText("shipped")).toBeTruthy();
    expect(screen.getByText("Create a custom agent")).toBeTruthy();
  });
});

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { buildAgentCatalog, buildHealth } from "../storybook/fixtures";
import { humanPause } from "../storybook/humanPlayback";
import { Home } from "./Home";

type StoryApi = AppServices["api"];

function failClosedApi(overrides: Partial<StoryApi> = {}): StoryApi {
  const allowed = {
    agents: async () => buildAgentCatalog(),
    ...overrides,
  } as StoryApi;
  return new Proxy(allowed, {
    get: (target, property, receiver) => {
      if (Reflect.has(target, property)) {
        return Reflect.get(target, property, receiver);
      }
      return () => {
        throw new Error(`Unexpected Storybook API call: ${String(property)}`);
      };
    },
  });
}

const noNetworkApi = failClosedApi();

const projectApi = failClosedApi({
    gitBranches: async () => ({
      isRepo: true,
      current: "main",
      branches: ["main"],
      dirty: 0,
      hasCommits: true,
    }),
});

const initialState = {
  health: buildHealth(),
  sessions: [],
  sessionsReady: true,
  newSessionProject: null,
} satisfies Partial<AppState>;

const meta = {
  title: "Pages/Home",
  component: Home,
  parameters: {
    fullHeight: true,
    options: { layout: { showPanel: false } },
  },
  render: () => (
    <StoryAppFrame initialState={initialState} services={{ api: noNetworkApi }}>
      <Home />
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("heading", { name: "What should we build?" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Explore and understand code" }),
    ).toBeVisible();
    await expect(canvas.getByPlaceholderText("Do anything")).toHaveFocus();
  },
} satisfies Meta<typeof Home>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const textarea = canvas.getByPlaceholderText("Do anything");
    await expect(textarea).toHaveFocus();
    await userEvent.type(textarea, "Review keyboard navigation");
    await expect(textarea).toHaveValue("Review keyboard navigation");

    await userEvent.tab();
    await expect(
      canvas.getByRole("button", { name: "Add and advanced options" }),
    ).toHaveFocus();
  },
};

export const StarterIntentFlow: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", {
        name: "Build a new feature, app, or tool",
      }),
    );

    const textarea = canvas.getByPlaceholderText("Do anything");
    await expect(textarea).toHaveValue("Build");
    await expect(canvas.getByLabelText("Build suggestions")).toBeVisible();
    await humanPause();

    await userEvent.click(
      canvas.getByRole("button", { name: "Build an internal tool" }),
    );
    await expect(textarea).toHaveValue("Build an internal tool");
    await expect(canvas.queryByLabelText("Build suggestions")).toBeNull();
  },
};

const longProject =
  "/workspace/platform/agentrunner-with-an-intentionally-long-project-name-for-headline-truncation";

export const ProjectAwareLongHeadline: Story = {
  render: () => (
    <StoryAppFrame
      initialState={initialState}
      services={{
        api: projectApi,
        local: { "arwebui.lastProject": longProject },
      }}
    >
      <Home />
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const heading = await canvas.findByRole("heading", {
      name: /What should we build in/,
    });
    const repo = heading.querySelector<HTMLElement>(".home-empty-repo")!;
    await expect(repo).toHaveTextContent(
      "agentrunner-with-an-intentionally-long-project-name-for-headline-truncation",
    );
    await expect(repo.scrollWidth).toBeGreaterThan(repo.clientWidth);
  },
};

export const DraftWithoutIntent: Story = {
  render: () => (
    <StoryAppFrame
      initialState={initialState}
      services={{
        api: noNetworkApi,
        session: {
          "arwebui.draft.~home":
            "Review the existing component states without choosing a starter",
        },
      }}
    >
      <Home />
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByPlaceholderText("Do anything")).toHaveValue(
      "Review the existing component states without choosing a starter",
    );
    await expect(
      canvas.queryByRole("button", { name: "Explore and understand code" }),
    ).not.toBeInTheDocument();
    await expect(
      canvas.queryByLabelText(/suggestions$/),
    ).not.toBeInTheDocument();
  },
};

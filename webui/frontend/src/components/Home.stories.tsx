import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { buildHealth } from "../storybook/fixtures";
import { Home } from "./Home";

type StoryApi = AppServices["api"];

const noNetworkApi = new Proxy({} as StoryApi, {
  get: (_target, property) => () => {
    throw new Error(`Unexpected Storybook API call: ${String(property)}`);
  },
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
  render: () => (
    <StoryAppFrame
      initialState={initialState}
      services={{ api: noNetworkApi }}
    >
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

    await userEvent.click(
      canvas.getByRole("button", { name: "Build an internal tool" }),
    );
    await expect(textarea).toHaveValue("Build an internal tool");
    await expect(canvas.queryByLabelText("Build suggestions")).toBeNull();
  },
};

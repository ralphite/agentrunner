import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { Toasts } from "./Toasts";

const notifications: AppState["toasts"] = [
  {
    id: 1,
    text: "Storybook coverage updated.",
    kind: "info",
  },
  {
    id: 2,
    text: "The browser check could not finish.",
    kind: "error",
    details: "playwright: locator('button').click: element was detached",
  },
];

function ToastFixture({ toasts = notifications }: { toasts?: AppState["toasts"] }) {
  return (
    <StoryAppFrame initialState={{ toasts }}>
      <Toasts />
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Feedback/Toasts",
  component: Toasts,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <ToastFixture />,
} satisfies Meta<typeof Toasts>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardDismiss: Story = {
  render: () => <ToastFixture toasts={[notifications[0]]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("status")).toHaveTextContent("Storybook coverage updated.");
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    const dismiss = canvas.getByRole("button", { name: "Dismiss notification" });
    await expect(dismiss).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.queryByRole("status")).not.toBeInTheDocument();
  },
};

export const DetailsExpanded: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText("Details", { selector: "summary" }));
    await expect(canvas.getByText(/element was detached/)).toBeVisible();
    await expect(canvas.getAllByRole("status")).toHaveLength(2);
  },
};

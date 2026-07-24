import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { StoryAppFrame } from "../../storybook/StoryAppFrame";
import { ComposerView } from "./ComposerView";

const onSubmit = fn();

function ComposerViewFixture() {
  const [text, setText] = useState("");

  return (
    <StoryAppFrame>
      <div className="mx-auto max-w-[760px] p-6">
        <ComposerView
          isSession
          dragging={false}
          cardEvents={{}}
          attachments={{ attachments: [], onRemove: fn() }}
          textarea={{
            "aria-label": "Message",
            value: text,
            placeholder: "Ask for follow-up changes",
            onChange: (event) => setText(event.target.value),
          }}
          addMenu={{
            page: "root",
            isSession: true,
            goalMode: false,
            planMode: false,
            kind: "chat",
            persona: "dev",
            onOpen: fn(),
            onPageChange: fn(),
            onPickFiles: fn(),
            onToggleGoal: fn(),
            onTogglePlan: fn(),
            onStartLoop: fn(),
            onStartBest: fn(),
            onToggleBackground: fn(),
            onSelectPersona: fn(),
            onEditSpec: fn(),
          }}
          accessPicker={{
            variant: "session",
            active: "ask",
            label: "Ask to approve",
            risk: "low",
            onSessionSelect: fn(),
          }}
          modelPicker={{
            provider: "gemini",
            model: "gemini-flash-latest",
            modelLabel: "Gemini Flash",
            effort: "medium",
            effortLabel: "Medium",
            budgetOverride: null,
            page: "root",
            onOpen: fn(),
            onPageChange: fn(),
            onSelectModel: fn(),
            onSelectEffort: fn(),
            onCustomModel: fn(),
            onCustomBudget: fn(),
          }}
          assistActions={{
            hasText: !!text.trim(),
            canUndo: false,
            optimizing: false,
            micVisible: false,
            micActive: false,
            dictationBusy: false,
            onOptimize: fn(),
            onUndo: fn(),
            onToggleMic: fn(),
          }}
          submitButton={{
            mode: "send",
            disabled: !text.trim(),
            onSubmit,
          }}
          fileInput={{}}
        />
      </div>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Input/ComposerView",
  component: ComposerViewFixture,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <ComposerViewFixture />,
} satisfies Meta<typeof ComposerViewFixture>;

export default meta;
type Story = StoryObj;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("textbox", { name: "Message" })).toHaveValue(
      "",
    );
    await expect(
      canvas.getByRole("button", { name: "Ask to approve" }),
    ).toBeVisible();
    await expect(canvas.getByTitle("Model & effort")).toHaveTextContent(
      "Gemini Flash",
    );
    await expect(
      canvas.getByRole("button", { name: "Send message" }),
    ).toBeDisabled();
  },
};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByRole("textbox", { name: "Message" });
    await userEvent.click(input);
    await userEvent.type(input, "Review the extracted composer view");
    await expect(input).toHaveValue("Review the extracted composer view");

    await userEvent.tab();
    const addButton = canvas.getByRole("button", {
      name: "Add and advanced options",
    });
    await expect(addButton).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(
      within(canvasElement.ownerDocument.body).getByRole("menuitem", {
        name: /Files and folders/,
      }),
    ).toBeVisible();
  },
};

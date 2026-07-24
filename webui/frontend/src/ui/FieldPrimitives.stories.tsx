import { useState } from "react";
import { MagnifyingGlass, SlidersHorizontal, X } from "@phosphor-icons/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { IconButton } from "./IconButton";
import { Field, Input, SearchField, Select, Textarea } from "./Field";

const meta = {
  title: "Foundations/Forms/Field primitives",
  component: Input,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof Input>;

export default meta;
type Story = StoryObj<typeof meta>;

export const InputStates: Story = {
  parameters: {
    pseudo: {
      focus: '[data-story-state="focus"]',
    },
  },
  render: () => (
    <div className="grid w-[420px] max-w-[calc(100vw-32px)] gap-5">
      <Field label="Empty">
        <Input aria-label="Empty input" />
      </Field>
      <Field label="Placeholder">
        <Input placeholder="Enter a value…" />
      </Field>
      <Field label="Value">
        <Input defaultValue="A saved value" />
      </Field>
      <Field label="Focus">
        <Input data-story-state="focus" defaultValue="Focused value" />
      </Field>
      <Field label="Required" required help="This value is required to continue.">
        <Input placeholder="Required value" />
      </Field>
      <Field label="Error" error="Use a valid workspace path.">
        <Input defaultValue="/missing/workspace" />
      </Field>
      <Field label="Disabled" disabled>
        <Input defaultValue="Unavailable value" />
      </Field>
      <Field label="Read only" help="This value is managed by Agent Runner.">
        <Input readOnly value="Managed value" />
      </Field>
      <Field label="A deliberately long field label that wraps without pushing the control outside its container">
        <Input placeholder="Long labels stay readable" />
      </Field>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("textbox", { name: /^Required/ }),
    ).toBeRequired();
    await expect(canvas.getByLabelText("Error")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await expect(canvas.getByLabelText("Disabled")).toBeDisabled();
    await expect(canvas.getByLabelText("Read only")).toHaveAttribute("readonly");
  },
};

export const TextareaStates: Story = {
  parameters: {
    pseudo: {
      focus: '[data-story-state="textarea-focus"]',
    },
  },
  render: () => (
    <div className="grid w-[520px] max-w-[calc(100vw-32px)] gap-5">
      <Field label="Empty">
        <Textarea placeholder="Write a response…" />
      </Field>
      <Field label="Focus">
        <Textarea data-story-state="textarea-focus" defaultValue="Focused multiline value" />
      </Field>
      <Field label="Long text" help="The control grows vertically, not horizontally.">
        <Textarea
          rows={5}
          defaultValue={
            "A long response can wrap across several lines while the field keeps a predictable width. Resize is available on the vertical axis for longer notes."
          }
        />
      </Field>
      <Field label="Code">
        <Textarea
          code
          spellCheck={false}
          defaultValue={'{\n  "sandbox": "workspace-write",\n  "approval": "on-request"\n}'}
        />
      </Field>
      <Field label="Error" error="The reason must be shorter than 500 characters.">
        <Textarea defaultValue="An invalid long-form value" />
      </Field>
      <Field label="Required" required help="A response is required.">
        <Textarea placeholder="Required response" />
      </Field>
      <Field label="Read only">
        <Textarea readOnly value="Managed multiline value" />
      </Field>
      <Field label="Disabled" disabled>
        <Textarea defaultValue="Editing is unavailable." />
      </Field>
    </div>
  ),
};

export const SelectStates: Story = {
  parameters: {
    pseudo: {
      focus: '[data-story-state="select-focus"]',
    },
  },
  render: () => (
    <div className="grid w-[420px] max-w-[calc(100vw-32px)] gap-5">
      <Field label="Empty">
        <Select defaultValue="">
          <option value="" disabled>
            Choose an environment…
          </option>
          <option value="local">Local</option>
        </Select>
      </Field>
      <Field label="Selected value">
        <Select defaultValue="workspace">
          <option value="workspace">
            Workspace write with approval on risky actions
          </option>
          <option value="readonly">Read only</option>
        </Select>
      </Field>
      <Field label="Focus">
        <Select data-story-state="select-focus" defaultValue="local">
          <option value="local">Local</option>
          <option value="workspace">Workspace</option>
        </Select>
      </Field>
      <Field label="Required" required help="Choose one runtime.">
        <Select defaultValue="" required>
          <option value="" disabled>Choose a runtime…</option>
          <option value="local">Local</option>
        </Select>
      </Field>
      <Field label="Error" error="Choose an available runtime.">
        <Select defaultValue="">
          <option value="" disabled>
            Unavailable runtime
          </option>
        </Select>
      </Field>
      <Field label="Disabled" disabled>
        <Select defaultValue="managed">
          <option value="managed">Managed by project</option>
        </Select>
      </Field>
    </div>
  ),
};

function SearchExamples() {
  const [query, setQuery] = useState("agent runner");
  return (
    <div className="grid w-[440px] max-w-[calc(100vw-32px)] gap-5">
      <Field label="Empty search">
        <SearchField
          aria-label="Empty search"
          icon={<MagnifyingGlass size={15} />}
          placeholder="Search sessions…"
        />
      </Field>
      <Field label="Focused search">
        <SearchField
          data-story-state="search-focus"
          aria-label="Focused search"
          icon={<MagnifyingGlass size={15} />}
          defaultValue="focused query"
        />
      </Field>
      <Field label="Search with value">
        <SearchField
          aria-label="Search with value"
          icon={<MagnifyingGlass size={15} />}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          endActions={
            <IconButton
              aria-label="Clear search"
              size="sm"
              variant="ghost"
              onClick={() => setQuery("")}
            >
              <X size={13} />
            </IconButton>
          }
        />
      </Field>
      <Field label="Search with end action" help="End actions remain separate, named controls.">
        <SearchField
          aria-label="Filtered search"
          icon={<MagnifyingGlass size={15} />}
          placeholder="Search settings…"
          endActions={
            <IconButton aria-label="Search filters" size="sm" variant="ghost">
              <SlidersHorizontal size={13} />
            </IconButton>
          }
        />
      </Field>
      <Field label="Invalid search" error="Search syntax is invalid.">
        <SearchField
          aria-label="Invalid search"
          aria-invalid="true"
          icon={<MagnifyingGlass size={15} />}
          defaultValue="status:"
        />
      </Field>
      <Field label="Disabled search" disabled>
        <SearchField
          aria-label="Disabled search"
          disabled
          icon={<MagnifyingGlass size={15} />}
          placeholder="Search unavailable"
        />
      </Field>
    </div>
  );
}

export const SearchStates: Story = {
  parameters: {
    pseudo: {
      focusWithin: '[data-story-state="search-focus"]',
    },
  },
  render: () => <SearchExamples />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const value = canvas.getByRole("searchbox", { name: "Search with value" });
    await expect(value).toHaveValue("agent runner");
    await userEvent.click(canvas.getByRole("button", { name: "Clear search" }));
    await expect(value).toHaveValue("");
    await expect(
      canvas.getByRole("button", { name: "Search filters" }),
    ).toHaveAttribute("title", "Search filters");
  },
};

export const ControlVariants: Story = {
  render: () => (
    <div className="grid w-[520px] max-w-[calc(100vw-32px)] gap-5">
      <Field label="Unstyled input inside custom chrome">
        <div className="rounded-full border border-line bg-panel px-4 py-2 focus-within:border-blue focus-within:ring-2 focus-within:ring-blue/30">
          <Input
            aria-label="Unstyled input inside custom chrome"
            variant="unstyled"
            placeholder="Unstyled input"
          />
        </div>
      </Field>
      <Field label="Unstyled textarea inside custom chrome">
        <div className="rounded-xl border border-line bg-panel p-3 focus-within:border-blue focus-within:ring-2 focus-within:ring-blue/30">
          <Textarea
            aria-label="Unstyled textarea inside custom chrome"
            variant="unstyled"
            rows={2}
            placeholder="Unstyled textarea"
          />
        </div>
      </Field>
      <Field label="Unstyled select inside custom chrome">
        <div className="rounded-lg border border-line bg-panel px-3 py-2 focus-within:border-blue focus-within:ring-2 focus-within:ring-blue/30">
          <Select
            aria-label="Unstyled select inside custom chrome"
            variant="unstyled"
            defaultValue="local"
          >
            <option value="local">Local</option>
            <option value="workspace">Workspace</option>
          </Select>
        </div>
      </Field>
      <Field label="Search variants">
        <div className="grid overflow-hidden rounded-xl border border-line">
          <SearchField aria-label="Default search" icon={<MagnifyingGlass size={15} />} placeholder="Default" />
          <SearchField aria-label="Flush search" variant="flush" icon={<MagnifyingGlass size={15} />} placeholder="Flush" />
          <div className="border-t border-line px-3 py-2">
            <SearchField aria-label="Unstyled search" variant="unstyled" icon={<MagnifyingGlass size={15} />} placeholder="Unstyled" />
          </div>
        </div>
      </Field>
    </div>
  ),
};

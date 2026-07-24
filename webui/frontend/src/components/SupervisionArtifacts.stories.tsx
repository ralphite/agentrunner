import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ArtifactItem, ArtifactsSection } from "./SupervisionParts";

const artifacts = [
  { stream: "reports/browser-qa.md", version: 3 },
  { stream: "screenshots/supervision-dark.png", version: 12 },
  { stream: "exports/component-inventory.json", version: 1 },
  {
    stream: "published/an-intentionally-long-artifact-name-that-must-truncate-without-hiding-open.pdf",
    version: 28,
  },
  { stream: "README", version: 2 },
];

const onOpen = fn();

function SectionSurface({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="supervision-panel session-side"
      style={{ position: "relative", inset: "auto", margin: "24px auto", width: 344 }}
    >
      {children}
    </div>
  );
}

const meta = {
  title: "Components/Supervision/Artifacts",
  component: ArtifactsSection,
  args: { artifacts, onOpen },
  render: (args) => <SectionSurface><ArtifactsSection {...args} /></SectionSurface>,
} satisfies Meta<typeof ArtifactsSection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const FileTypesAndOverflow: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Document · MD · v3")).toBeVisible();
    await expect(canvas.getByText("Image · PNG · v12")).toBeVisible();
    await expect(canvas.getByText("File · JSON · v1")).toBeVisible();
    await expect(canvas.getByText("Artifact · v2")).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: /Open reports\/browser-qa\.md version 3/ }));
    await expect(args.onOpen).toHaveBeenCalledWith("reports/browser-qa.md", 3);
  },
};

export const SingleArtifactItem: Story = {
  render: () => (
    <SectionSurface>
      <section className="supervision-section">
        <div className="artifact-list">
          <ArtifactItem artifact={artifacts[0]} onOpen={onOpen} />
        </div>
      </section>
    </SectionSurface>
  ),
};

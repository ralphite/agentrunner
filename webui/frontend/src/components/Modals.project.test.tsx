// @vitest-environment jsdom
// INC-104 · Create/Edit project dialog: prefill, folder add/remove through the
// stacked PromptModal, inline server errors that keep the dialog open, and the
// in-footer two-step Remove (the confirm modal shares the slot, so opening it
// would destroy unsaved edits).
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  createProject: vi.fn(async (b: { name: string; folders: string[] }) => ({
    overlays: {},
    projects: [{ id: "p-new", name: b.name, folders: b.folders }],
    created: { id: "p-new", name: b.name, folders: b.folders },
  })),
  saveProject: vi.fn(async () => ({ overlays: {}, projects: [] })),
  deleteProject: vi.fn(async () => ({ overlays: {}, projects: [] })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    createProject: mocks.createProject,
    saveProject: mocks.saveProject,
    deleteProject: mocks.deleteProject,
  },
}));

import { useStore } from "../store";
import { Modals } from "./Modals";

beforeEach(() => {
  mocks.createProject.mockClear();
  mocks.saveProject.mockClear();
  mocks.deleteProject.mockClear();
  useStore.setState({
    sessions: [
      { id: "s1", status: "idle", turns: 1, workspace: "/repo/known" },
    ] as any,
    projectDefs: [],
    modal: null,
    prompt: null,
  });
});

afterEach(() => {
  cleanup();
  useStore.setState({ modal: null, prompt: null });
});

const addFolderThroughPrompt = async (path: string) => {
  fireEvent.click(screen.getByRole("button", { name: /Add folder/ }));
  const input = await screen.findByPlaceholderText("/path/to/folder");
  fireEvent.change(input, { target: { value: path } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
};

describe("ProjectModal (INC-104)", () => {
  it("creates a project from typed folders and closes on success", async () => {
    useStore.setState({ modal: { kind: "project", mode: "create" } });
    render(<Modals />);

    const submit = screen.getByRole("button", { name: "Create project" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true); // no name, no folders yet

    fireEvent.change(screen.getByPlaceholderText("Project name"), { target: { value: "Orca" } });
    await addFolderThroughPrompt("/repo/app/");

    // The trailing slash is trimmed and the folder shows as a row.
    expect(screen.getByTitle("/repo/app")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create project" }));

    await waitFor(() => expect(mocks.createProject).toHaveBeenCalledWith({ name: "Orca", folders: ["/repo/app"] }));
    await waitFor(() => expect(useStore.getState().modal).toBeNull());
  });

  it("offers known workspaces as datalist suggestions in the Add-folder prompt", async () => {
    useStore.setState({ modal: { kind: "project", mode: "create" } });
    const { baseElement } = render(<Modals />);
    fireEvent.click(screen.getByRole("button", { name: /Add folder/ }));
    await screen.findByPlaceholderText("/path/to/folder");
    const options = [...baseElement.querySelectorAll("datalist option")].map((o) => o.getAttribute("value"));
    expect(options).toContain("/repo/known");
  });

  it("keeps the dialog open and shows the server sentence inline on a validation error", async () => {
    mocks.createProject.mockRejectedValueOnce(new Error('That folder is already in "Other".'));
    useStore.setState({ modal: { kind: "project", mode: "create" } });
    render(<Modals />);

    fireEvent.change(screen.getByPlaceholderText("Project name"), { target: { value: "Orca" } });
    await addFolderThroughPrompt("/repo/app");
    fireEvent.click(screen.getByRole("button", { name: "Create project" }));

    expect((await screen.findByRole("alert")).textContent).toContain('That folder is already in "Other".');
    expect(useStore.getState().modal).toMatchObject({ kind: "project" });
  });

  it("prefills edit mode, removes a folder row, and saves the full replacement list", async () => {
    useStore.setState({
      modal: {
        kind: "project",
        mode: "edit",
        id: "p-1",
        initialName: "Orca",
        initialFolders: ["/repo/app", "/repo/docs"],
      },
    });
    render(<Modals />);

    expect((screen.getByPlaceholderText("Project name") as HTMLInputElement).value).toBe("Orca");
    fireEvent.click(screen.getByRole("button", { name: "Remove folder /repo/docs" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(mocks.saveProject).toHaveBeenCalledWith({ id: "p-1", name: "Orca", folders: ["/repo/app"] }));
    await waitFor(() => expect(useStore.getState().modal).toBeNull());
  });

  it("deletes only after the in-footer two-step confirmation", async () => {
    useStore.setState({
      modal: { kind: "project", mode: "edit", id: "p-1", initialName: "Orca", initialFolders: ["/repo/app"] },
    });
    render(<Modals />);

    fireEvent.click(screen.getByRole("button", { name: "Remove project" }));
    expect(mocks.deleteProject).not.toHaveBeenCalled();
    expect(screen.getByText("Remove permanently?")).toBeTruthy();

    // Backing out restores the pill without deleting.
    fireEvent.click(screen.getByRole("button", { name: "Keep" }));
    expect(screen.getByRole("button", { name: "Remove project" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Remove project" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(mocks.deleteProject).toHaveBeenCalledWith("p-1"));
    await waitFor(() => expect(useStore.getState().modal).toBeNull());
  });
});

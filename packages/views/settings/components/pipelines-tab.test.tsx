// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Pipeline } from "@multica/core/types";
import type { ReactElement } from "react";

const mockListPipelines = vi.hoisted(() => vi.fn());
const mockCreatePipeline = vi.hoisted(() => vi.fn());
const mockUpdatePipeline = vi.hoisted(() => vi.fn());
const mockDeletePipeline = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Alpha Workspace" }),
}));

vi.mock("@multica/core/api", () => ({
  api: {
    listPipelines: (...args: unknown[]) => mockListPipelines(...args),
    createPipeline: (...args: unknown[]) => mockCreatePipeline(...args),
    updatePipeline: (...args: unknown[]) => mockUpdatePipeline(...args),
    deletePipeline: (...args: unknown[]) => mockDeletePipeline(...args),
    getPipeline: vi.fn(),
  },
}));

vi.mock("./account-tab", () => ({ AccountTab: () => <div /> }));
vi.mock("./appearance-tab", () => ({ AppearanceTab: () => <div /> }));
vi.mock("./tokens-tab", () => ({ TokensTab: () => <div /> }));
vi.mock("./workspace-tab", () => ({ WorkspaceTab: () => <div /> }));
vi.mock("./members-tab", () => ({ MembersTab: () => <div /> }));
vi.mock("./repositories-tab", () => ({ RepositoriesTab: () => <div /> }));

import { SettingsPage } from "./settings-page";
import { PipelinesTab } from "./pipelines-tab";

const pipelines: Pipeline[] = [
  {
    id: "pipeline-1",
    workspace_id: "ws-1",
    name: "Launch Flow",
    description: "Moves work through the release process",
    stages: [
      {
        id: "stage-1",
        pipeline_id: "pipeline-1",
        name: "Triage",
        status: "todo",
        agent_id: "agent-1",
        stage_instructions: "Check the issue details",
        position: 1,
      },
    ],
  },
];

function renderWithQueryClient(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

describe("SettingsPage pipelines tab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListPipelines.mockResolvedValue({ pipelines, total: pipelines.length });
    mockCreatePipeline.mockResolvedValue(pipelines[0]);
    mockUpdatePipeline.mockResolvedValue(pipelines[0]);
    mockDeletePipeline.mockResolvedValue(undefined);
  });

  it("shows a Pipelines tab in workspace settings", async () => {
    renderWithQueryClient(<SettingsPage />);

    expect(await screen.findByRole("tab", { name: /pipelines/i })).toBeInTheDocument();
  });

  it("shows an error state when pipelines fail to load", async () => {
    mockListPipelines.mockRejectedValueOnce(new Error("boom"));
    renderWithQueryClient(<PipelinesTab />);

    expect(await screen.findByText(/failed to load pipelines/i)).toBeInTheDocument();
  });

  it("prevents creating a pipeline when a required stage field is blank", async () => {
    renderWithQueryClient(<PipelinesTab />);

    expect(await screen.findByText("Launch Flow")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("New pipeline name"), { target: { value: "New Pipeline" } });
    fireEvent.click(screen.getByRole("button", { name: /create pipeline/i }));

    await waitFor(() => {
      expect(mockCreatePipeline).not.toHaveBeenCalled();
    });
  });

  it("creates a new pipeline through the mutation", async () => {
    renderWithQueryClient(<PipelinesTab />);

    expect(await screen.findByText("Launch Flow")).toBeInTheDocument();
    expect(screen.getByText("Moves work through the release process")).toBeInTheDocument();
    expect(screen.getByText((_, el) => el?.tagName === "P" && el.textContent?.startsWith("1 pipeline") === true)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("New pipeline name"), { target: { value: "New Pipeline" } });
    fireEvent.change(screen.getByLabelText("New pipeline description"), { target: { value: "New description" } });
    fireEvent.change(screen.getByLabelText(/new stage 1 name/i), { target: { value: "Triage" } });
    fireEvent.change(screen.getByLabelText(/new stage 1 status/i), { target: { value: "todo" } });
    fireEvent.change(screen.getByLabelText(/new stage 1 agent/i), { target: { value: "agent-1" } });
    fireEvent.change(screen.getByLabelText(/new stage 1 instructions/i), { target: { value: "Check the issue details" } });
    fireEvent.change(screen.getByLabelText(/new stage 1 position/i), { target: { value: "1" } });
    fireEvent.click(screen.getByRole("button", { name: /create pipeline/i }));

    await waitFor(() => {
      expect(mockCreatePipeline).toHaveBeenCalledWith({
        name: "New Pipeline",
        description: "New description",
        stages: [
          {
            name: "Triage",
            status: "todo",
            agent_id: "agent-1",
            stage_instructions: "Check the issue details",
            position: 1,
          },
        ],
      });
    });
  });

  it("updates an existing pipeline through the mutation", async () => {
    renderWithQueryClient(<PipelinesTab />);

    expect(await screen.findByText("Launch Flow")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("pipeline-1 pipeline name"), {
      target: { value: "Launch Flow v2" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(mockUpdatePipeline).toHaveBeenCalledWith("pipeline-1", {
        name: "Launch Flow v2",
        description: "Moves work through the release process",
        stages: [
          {
            name: "Triage",
            status: "todo",
            agent_id: "agent-1",
            stage_instructions: "Check the issue details",
            position: 1,
          },
        ],
      });
    });
  });
});

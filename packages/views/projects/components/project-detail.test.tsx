// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { Project, ProjectControl, Pipeline } from "@multica/core/types";
import { ProjectDetail } from "./project-detail";

const mockUpdateProjectControl = vi.fn();
const mockReconcileProjectControl = vi.fn();
const mockReconcileProjectOverseer = vi.fn();

const project: Project = {
  id: "project-1",
  workspace_id: "ws-1",
  title: "Automation Project",
  description: null,
  icon: "📁",
  status: "planned",
  priority: "medium",
  lead_type: null,
  lead_id: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  issue_count: 0,
  done_count: 0,
};

const projectControl: ProjectControl = {
  project_id: "project-1",
  overseer_agent_id: "agent-1",
  overseer_autopilot_id: "autopilot-9",
  overseer_autonomy_status: "active",
  overseer_autonomy_trigger_id: "trigger-1",
  overseer_autonomy_next_run_at: "2026-01-02T06:00:00Z",
  overseer_autonomy: {
    autopilot_id: "autopilot-9",
    status: "active",
    trigger_id: "trigger-1",
    next_run_at: "2026-01-02T06:00:00Z",
  },
  default_pipeline_id: "pipeline-1",
  automation_mode: "assisted",
  auto_triage_enabled: true,
  auto_route_enabled: true,
  auto_escalate_blocked: true,
  stale_after_minutes: 120,
  review_policy: null,
  quality_policy: null,
  overseer_config: {
    scan_interval_hours: 12,
    scan_focus: ["security", "documentation"],
    product_context: "Protect customer-facing releases and checkout quality.",
    quality_bar: ["Zero regressions", "Clear owner"],
    priority_weights: {
      impact: 5,
      urgency: 3,
    },
    max_issues_per_run: 4,
    require_approval: true,
  },
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const pipelines: Pipeline[] = [
  {
    id: "pipeline-1",
    workspace_id: "ws-1",
    name: "Default Delivery",
    description: null,
    stages: [],
  },
];

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector?: (state: { user: { id: string } | null }) => unknown) => {
    const state = { user: { id: "user-1" } };
    return selector ? selector(state) : state;
  },
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Alpha Workspace", slug: "alpha" }),
  useWorkspacePaths: () => ({
    projects: () => "/alpha/projects",
    autopilotDetail: (id: string) => `/alpha/autopilots/${id}`,
  }),
}));

vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({
    getActorName: (_type: string, id: string) => (id === "agent-1" ? "Overseer Agent" : "Unknown"),
  }),
}));

vi.mock("@multica/core/projects/mutations", () => ({
  useUpdateProject: () => ({ mutate: vi.fn() }),
  useDeleteProject: () => ({ mutate: vi.fn() }),
}));

vi.mock("@multica/core/projects/queries", () => ({
  projectDetailOptions: () => ({
    queryKey: ["projects", "ws-1", "detail", "project-1"],
    queryFn: async () => project,
  }),
}));

vi.mock("@multica/core/project-control", () => ({
  projectControlOptions: () => ({
    queryKey: ["project-control", "ws-1", "detail", "project-1"],
    queryFn: async () => projectControl,
  }),
  useUpdateProjectControl: () => ({
    mutate: mockUpdateProjectControl,
    isPending: false,
  }),
  useReconcileProjectControl: () => ({
    mutate: mockReconcileProjectControl,
    isPending: false,
  }),
  useReconcileProjectOverseer: () => ({
    mutate: mockReconcileProjectOverseer,
    isPending: false,
  }),
}));

vi.mock("@multica/core/pipelines", () => ({
  pipelineListOptions: () => ({
    queryKey: ["pipelines", "ws-1", "list"],
    queryFn: async () => pipelines,
  }),
}));

vi.mock("@multica/core/issues/queries", () => ({
  issueListOptions: () => ({ queryKey: ["issues", "ws-1"], queryFn: async () => [] }),
  childIssueProgressOptions: () => ({ queryKey: ["issues", "ws-1", "child-progress"], queryFn: async () => new Map() }),
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members", "ws-1"], queryFn: async () => [] }),
  agentListOptions: () => ({
    queryKey: ["agents", "ws-1"],
    queryFn: async () => [
      { id: "agent-1", name: "Overseer Agent", archived_at: null },
      { id: "agent-2", name: "Operator Agent", archived_at: null },
    ],
  }),
}));

vi.mock("@multica/core/pins", () => ({
  pinListOptions: () => ({ queryKey: ["pins", "user-1"], queryFn: async () => [] }),
  useCreatePin: () => ({ mutate: vi.fn() }),
  useDeletePin: () => ({ mutate: vi.fn() }),
}));

vi.mock("@multica/core/issues/mutations", () => ({
  useUpdateIssue: () => ({ mutate: vi.fn() }),
}));

vi.mock("@multica/ui/hooks/use-mobile", () => ({
  useIsMobile: () => false,
}));

vi.mock("@multica/ui/components/ui/resizable", () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizablePanel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizableHandle: () => <div />,
}));

vi.mock("react-resizable-panels", () => ({
  useDefaultLayout: () => ({ defaultLayout: undefined, onLayoutChanged: vi.fn() }),
  usePanelRef: () => ({ current: { collapse: vi.fn(), expand: vi.fn(), isCollapsed: () => false } }),
}));

vi.mock("../../navigation", () => ({
  AppLink: ({ children, href, ...props }: { children: React.ReactNode; href: string }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
  useNavigation: () => ({ push: vi.fn() }),
}));

vi.mock("../../editor", () => ({
  TitleEditor: ({ defaultValue }: { defaultValue: string }) => <div>{defaultValue}</div>,
  ContentEditor: () => <div data-testid="content-editor" />,
}));

vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: ({ actorId }: { actorId: string }) => <span>{actorId}</span>,
}));

vi.mock("../../issues/components/issues-header", () => ({
  IssuesHeader: () => <div />,
}));

vi.mock("../../issues/components/board-view", () => ({
  BoardView: () => <div />,
}));

vi.mock("../../issues/components/list-view", () => ({
  ListView: () => <div />,
}));

vi.mock("../../issues/components/batch-action-toolbar", () => ({
  BatchActionToolbar: () => <div />,
}));

vi.mock("../../layout/page-header", () => ({
  PageHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("@multica/core/issues/stores/view-store", () => ({
  createIssueViewStore: () => ({
    getState: () => ({
      sortBy: "position",
      sortDirection: "asc",
      setSortBy: vi.fn(),
      setSortDirection: vi.fn(),
    }),
  }),
}));

vi.mock("@multica/core/issues/stores/view-store-context", () => ({
  ViewStoreProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  useViewStore: (
    selector: (state: {
      viewMode: "board";
      statusFilters: string[];
      priorityFilters: string[];
      assigneeFilters: string[];
      includeNoAssignee: boolean;
      creatorFilters: string[];
    }) => unknown,
  ) =>
    selector({
      viewMode: "board",
      statusFilters: [],
      priorityFilters: [],
      assigneeFilters: [],
      includeNoAssignee: false,
      creatorFilters: [],
    }),
}));

function renderProjectDetail() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectDetail projectId="project-1" />
    </QueryClientProvider>,
  );
}

describe("ProjectDetail automation controls", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders strategic overseer config values and linked autonomy status", async () => {
    renderProjectDetail();

    expect(await screen.findByText("Automation")).toBeInTheDocument();
    expect(screen.getByText("Overseer")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Overseer" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Default pipeline" })).toBeInTheDocument();
    expect(screen.getByRole("spinbutton", { name: "Scan interval hours" })).toHaveValue(12);
    expect(screen.getByRole("checkbox", { name: "Security" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Documentation" })).toBeChecked();
    expect(screen.getByRole("textbox", { name: "Product context" })).toHaveValue(
      "Protect customer-facing releases and checkout quality.",
    );
    expect(screen.getByRole("textbox", { name: "Quality bar" })).toHaveValue("Zero regressions\nClear owner");
    expect(screen.getByRole("textbox", { name: "Priority weights" })).toHaveValue(`{
  "impact": 5,
  "urgency": 3
}`);
    expect(screen.getByRole("spinbutton", { name: "Max issues per run" })).toHaveValue(4);
    expect(screen.getByRole("switch", { name: "Require approval" })).toBeChecked();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("2026-01-02T06:00:00Z")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open linked autopilot" })).toHaveAttribute(
      "href",
      "/alpha/autopilots/autopilot-9",
    );
  });

  it("saves overseer config field changes through the mutation", async () => {
    renderProjectDetail();

    fireEvent.change(await screen.findByRole("spinbutton", { name: "Scan interval hours" }), {
      target: { value: "24" },
    });
    fireEvent.blur(screen.getByRole("spinbutton", { name: "Scan interval hours" }));

    fireEvent.click(screen.getByRole("checkbox", { name: "Documentation" }));

    fireEvent.change(screen.getByRole("textbox", { name: "Quality bar" }), {
      target: { value: "Zero regressions\nFast rollback" },
    });
    fireEvent.blur(screen.getByRole("textbox", { name: "Quality bar" }));

    fireEvent.change(screen.getByRole("textbox", { name: "Priority weights" }), {
      target: { value: '{"impact":7,"confidence":2}' },
    });
    fireEvent.blur(screen.getByRole("textbox", { name: "Priority weights" }));

    fireEvent.change(screen.getByRole("spinbutton", { name: "Max issues per run" }), {
      target: { value: "6" },
    });
    fireEvent.blur(screen.getByRole("spinbutton", { name: "Max issues per run" }));

    fireEvent.click(screen.getByRole("switch", { name: "Require approval" }));
    fireEvent.click(screen.getByRole("switch", { name: /auto-escalate blocked/i }));

    await waitFor(() => {
      expect(mockUpdateProjectControl).toHaveBeenCalledWith({
        id: "project-1",
        data: {
          overseer_config: {
            scan_interval_hours: 24,
            scan_focus: ["security", "documentation"],
            product_context: "Protect customer-facing releases and checkout quality.",
            quality_bar: ["Zero regressions", "Clear owner"],
            priority_weights: {
              impact: 5,
              urgency: 3,
            },
            max_issues_per_run: 4,
            require_approval: true,
          },
        },
      });
    });

    expect(mockUpdateProjectControl).toHaveBeenCalledWith({
      id: "project-1",
      data: {
        overseer_config: {
          scan_interval_hours: 12,
          scan_focus: ["security"],
          product_context: "Protect customer-facing releases and checkout quality.",
          quality_bar: ["Zero regressions", "Clear owner"],
          priority_weights: {
            impact: 5,
            urgency: 3,
          },
          max_issues_per_run: 4,
          require_approval: true,
        },
      },
    });

    expect(mockUpdateProjectControl).toHaveBeenCalledWith({
      id: "project-1",
      data: {
        overseer_config: {
          scan_interval_hours: 12,
          scan_focus: ["security", "documentation"],
          product_context: "Protect customer-facing releases and checkout quality.",
          quality_bar: ["Zero regressions", "Fast rollback"],
          priority_weights: {
            impact: 5,
            urgency: 3,
          },
          max_issues_per_run: 4,
          require_approval: true,
        },
      },
    });

    expect(mockUpdateProjectControl).toHaveBeenCalledWith({
      id: "project-1",
      data: {
        overseer_config: {
          scan_interval_hours: 12,
          scan_focus: ["security", "documentation"],
          product_context: "Protect customer-facing releases and checkout quality.",
          quality_bar: ["Zero regressions", "Clear owner"],
          priority_weights: {
            impact: 7,
            confidence: 2,
          },
          max_issues_per_run: 4,
          require_approval: true,
        },
      },
    });

    expect(mockUpdateProjectControl).toHaveBeenCalledWith({
      id: "project-1",
      data: {
        overseer_config: {
          scan_interval_hours: 12,
          scan_focus: ["security", "documentation"],
          product_context: "Protect customer-facing releases and checkout quality.",
          quality_bar: ["Zero regressions", "Clear owner"],
          priority_weights: {
            impact: 5,
            urgency: 3,
          },
          max_issues_per_run: 6,
          require_approval: true,
        },
      },
    });

    expect(mockUpdateProjectControl).toHaveBeenCalledWith({
      id: "project-1",
      data: {
        overseer_config: {
          scan_interval_hours: 12,
          scan_focus: ["security", "documentation"],
          product_context: "Protect customer-facing releases and checkout quality.",
          quality_bar: ["Zero regressions", "Clear owner"],
          priority_weights: {
            impact: 5,
            urgency: 3,
          },
          max_issues_per_run: 4,
          require_approval: false,
        },
      },
    });

    expect(mockUpdateProjectControl).toHaveBeenCalledWith({
      id: "project-1",
      data: { auto_escalate_blocked: false },
    });
  });

  it("triggers the strategic overseer reconcile action", async () => {
    renderProjectDetail();

    fireEvent.click(await screen.findByRole("button", { name: "Reconcile strategic overseer" }));

    expect(mockReconcileProjectOverseer).toHaveBeenCalledWith("project-1");
    expect(mockReconcileProjectControl).not.toHaveBeenCalled();
  });
});

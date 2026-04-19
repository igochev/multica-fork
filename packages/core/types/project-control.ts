export type OverseerScanFocus =
  | "security"
  | "test_coverage"
  | "code_quality"
  | "documentation"
  | "architecture";

export type OverseerPriorityWeights = Record<string, number>;

export interface OverseerConfig {
  scan_interval_hours?: number;
  scan_focus?: OverseerScanFocus[];
  product_context?: string;
  quality_bar?: string[];
  priority_weights?: OverseerPriorityWeights;
  max_issues_per_run?: number;
  require_approval?: boolean;
}

export interface ProjectOverseerAutonomy {
  autopilot_id: string;
  status: string;
  trigger_id: string | null;
  next_run_at: string | null;
}

export interface ProjectControl {
  project_id: string;
  overseer_agent_id: string | null;
  overseer_autopilot_id: string | null;
  overseer_autonomy_status: string | null;
  overseer_autonomy_trigger_id: string | null;
  overseer_autonomy_next_run_at: string | null;
  overseer_autonomy: ProjectOverseerAutonomy | null;
  default_pipeline_id: string | null;
  automation_mode: string;
  auto_triage_enabled: boolean;
  auto_route_enabled: boolean;
  auto_escalate_blocked: boolean;
  stale_after_minutes: number;
  review_policy: string | null;
  quality_policy: string | null;
  overseer_config: OverseerConfig;
  created_at: string;
  updated_at: string;
}

export interface UpdateProjectControlRequest {
  overseer_agent_id?: string | null;
  default_pipeline_id?: string | null;
  automation_mode?: string;
  auto_triage_enabled?: boolean;
  auto_route_enabled?: boolean;
  auto_escalate_blocked?: boolean;
  stale_after_minutes?: number;
  review_policy?: string | null;
  quality_policy?: string | null;
  overseer_config?: OverseerConfig;
}

export interface ReconcileProjectControlResponse {
  project_id: string;
  escalated_issue_ids: string[];
  escalated_count: number;
}

export interface ReconcileProjectOverseerResponse {
  project_id: string;
  autonomy: ProjectOverseerAutonomy | null;
  created_autopilot: boolean;
  updated_autopilot: boolean;
  created_trigger: boolean;
  updated_trigger: boolean;
  updated_project_link: boolean;
}

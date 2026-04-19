export interface ProjectControl {
  project_id: string;
  overseer_agent_id: string | null;
  default_pipeline_id: string | null;
  automation_mode: string;
  auto_triage_enabled: boolean;
  auto_route_enabled: boolean;
  auto_escalate_blocked: boolean;
  stale_after_minutes: number;
  review_policy: string | null;
  quality_policy: string | null;
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
}

export interface ReconcileProjectControlResponse {
  project_id: string;
  escalated_issue_ids: string[];
  escalated_count: number;
}

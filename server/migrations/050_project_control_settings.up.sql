CREATE TABLE project_control_settings (
    project_id UUID PRIMARY KEY REFERENCES project(id) ON DELETE CASCADE,
    overseer_agent_id UUID REFERENCES agent(id) ON DELETE SET NULL,
    default_pipeline_id UUID REFERENCES pipelines(id) ON DELETE SET NULL,
    automation_mode TEXT NOT NULL DEFAULT 'manual'
        CHECK (automation_mode IN ('manual', 'assisted', 'autonomous')),
    auto_triage_enabled BOOLEAN NOT NULL DEFAULT false,
    auto_route_enabled BOOLEAN NOT NULL DEFAULT false,
    auto_escalate_blocked BOOLEAN NOT NULL DEFAULT false,
    stale_after_minutes INT NOT NULL DEFAULT 60 CHECK (stale_after_minutes > 0),
    review_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    quality_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION ensure_project_control_settings_workspace_match()
RETURNS trigger AS $$
BEGIN
    IF NEW.overseer_agent_id IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM project p
        JOIN agent a ON a.id = NEW.overseer_agent_id
        WHERE p.id = NEW.project_id
          AND p.workspace_id = a.workspace_id
    ) THEN
        RAISE EXCEPTION 'project control overseer agent workspace must match project workspace'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'project_control_settings_overseer_workspace_match';
    END IF;

    IF NEW.default_pipeline_id IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM project p
        JOIN pipelines pl ON pl.id = NEW.default_pipeline_id
        WHERE p.id = NEW.project_id
          AND p.workspace_id = pl.workspace_id
    ) THEN
        RAISE EXCEPTION 'project control default pipeline workspace must match project workspace'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'project_control_settings_pipeline_workspace_match';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_project_control_settings_workspace_match
    BEFORE INSERT OR UPDATE OF project_id, overseer_agent_id, default_pipeline_id
    ON project_control_settings
    FOR EACH ROW
    EXECUTE FUNCTION ensure_project_control_settings_workspace_match();

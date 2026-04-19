DROP TRIGGER IF EXISTS trg_project_control_settings_workspace_match ON project_control_settings;
DROP FUNCTION IF EXISTS ensure_project_control_settings_workspace_match();

ALTER TABLE project_control_settings
DROP COLUMN IF EXISTS overseer_autopilot_id;

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
        JOIN pipelines pipeline ON pipeline.id = NEW.default_pipeline_id
        WHERE p.id = NEW.project_id
          AND p.workspace_id = pipeline.workspace_id
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

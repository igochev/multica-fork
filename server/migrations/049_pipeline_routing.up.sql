CREATE TABLE pipelines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pipelines_workspace_name_unique UNIQUE (workspace_id, name)
);

CREATE TABLE pipeline_stages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL
        CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'blocked', 'cancelled')),
    agent_id UUID NOT NULL REFERENCES agent(id),
    stage_instructions TEXT,
    position INT NOT NULL CHECK (position > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pipeline_stages_pipeline_position_unique UNIQUE (pipeline_id, position),
    CONSTRAINT pipeline_stages_pipeline_status_unique UNIQUE (pipeline_id, status),
    CONSTRAINT pipeline_stages_pipeline_id_id_unique UNIQUE (pipeline_id, id)
);

CREATE INDEX idx_pipeline_stages_agent ON pipeline_stages(agent_id);

CREATE OR REPLACE FUNCTION ensure_pipeline_stage_agent_workspace_match()
RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pipelines p
        JOIN agent a ON a.id = NEW.agent_id
        WHERE p.id = NEW.pipeline_id
          AND p.workspace_id = a.workspace_id
    ) THEN
        RAISE EXCEPTION 'pipeline stage agent workspace must match pipeline workspace'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'pipeline_stages_agent_workspace_match';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_pipeline_stages_agent_workspace_match
    BEFORE INSERT OR UPDATE OF pipeline_id, agent_id
    ON pipeline_stages
    FOR EACH ROW
    EXECUTE FUNCTION ensure_pipeline_stage_agent_workspace_match();

CREATE TABLE issue_pipelines (
    issue_id UUID PRIMARY KEY REFERENCES issue(id) ON DELETE CASCADE,
    pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    current_stage_id UUID NOT NULL,
    stage_sequence INT NOT NULL DEFAULT 0 CHECK (stage_sequence >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT issue_pipelines_pipeline_stage_fk
        FOREIGN KEY (pipeline_id, current_stage_id)
        REFERENCES pipeline_stages(pipeline_id, id)
);

CREATE INDEX idx_issue_pipelines_pipeline ON issue_pipelines(pipeline_id);
CREATE INDEX idx_issue_pipelines_current_stage ON issue_pipelines(current_stage_id);

CREATE OR REPLACE FUNCTION ensure_issue_pipeline_workspace_match()
RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM issue i
        JOIN pipelines p ON p.id = NEW.pipeline_id
        WHERE i.id = NEW.issue_id
          AND i.workspace_id = p.workspace_id
    ) THEN
        RAISE EXCEPTION 'issue pipeline workspace must match issue workspace'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'issue_pipelines_workspace_match';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_issue_pipelines_workspace_match
    BEFORE INSERT OR UPDATE OF issue_id, pipeline_id
    ON issue_pipelines
    FOR EACH ROW
    EXECUTE FUNCTION ensure_issue_pipeline_workspace_match();

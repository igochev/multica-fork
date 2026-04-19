ALTER TABLE project_control_settings
ADD COLUMN overseer_config JSONB NOT NULL DEFAULT '{}'::jsonb;

export interface PipelineStage {
  id: string;
  pipeline_id: string;
  name: string;
  status: string;
  agent_id: string;
  stage_instructions: string | null;
  position: number;
}

export interface Pipeline {
  id: string;
  workspace_id: string;
  name: string;
  description: string | null;
  stages: PipelineStage[];
}

export interface PipelineStageRequest {
  name: string;
  status: string;
  agent_id: string;
  stage_instructions?: string;
  position: number;
}

export interface CreatePipelineRequest {
  name: string;
  description?: string;
  stages: PipelineStageRequest[];
}

export interface UpdatePipelineRequest {
  name?: string;
  description?: string | null;
  stages?: PipelineStageRequest[];
}

export interface ListPipelinesResponse {
  pipelines: Pipeline[];
  total: number;
}

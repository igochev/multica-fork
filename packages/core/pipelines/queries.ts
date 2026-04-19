import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const pipelineKeys = {
  all: (wsId: string) => ["pipelines", wsId] as const,
  list: (wsId: string) => [...pipelineKeys.all(wsId), "list"] as const,
  detail: (wsId: string, id: string) => [...pipelineKeys.all(wsId), "detail", id] as const,
};

export function pipelineListOptions(wsId: string) {
  return queryOptions({
    queryKey: pipelineKeys.list(wsId),
    queryFn: () => api.listPipelines(),
    select: (data) => data.pipelines,
  });
}

export function pipelineDetailOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: pipelineKeys.detail(wsId, id),
    queryFn: () => api.getPipeline(id),
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import type { CreatePipelineRequest, Pipeline, UpdatePipelineRequest } from "../types";
import { pipelineKeys } from "./queries";

function syncPipelineList(list: Pipeline[] | undefined, pipeline: Pipeline) {
  if (!list) return list;
  return list.some((item) => item.id === pipeline.id)
    ? list.map((item) => (item.id === pipeline.id ? pipeline : item))
    : [...list, pipeline];
}

export function useCreatePipeline() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (data: CreatePipelineRequest) => api.createPipeline(data),
    onSuccess: (pipeline) => {
      qc.setQueryData<Pipeline[]>(pipelineKeys.list(wsId), (old) => syncPipelineList(old, pipeline));
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: pipelineKeys.list(wsId) });
    },
  });
}

export function useUpdatePipeline() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & UpdatePipelineRequest) =>
      api.updatePipeline(id, data),
    onSuccess: (pipeline) => {
      qc.setQueryData<Pipeline[]>(pipelineKeys.list(wsId), (old) => syncPipelineList(old, pipeline));
      qc.setQueryData<Pipeline>(pipelineKeys.detail(wsId, pipeline.id), pipeline);
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: pipelineKeys.detail(wsId, vars.id) });
      qc.invalidateQueries({ queryKey: pipelineKeys.list(wsId) });
    },
  });
}

export function useDeletePipeline() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (id: string) => api.deletePipeline(id),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: pipelineKeys.list(wsId) });
      const prevList = qc.getQueryData<Pipeline[]>(pipelineKeys.list(wsId));
      qc.setQueryData<Pipeline[]>(pipelineKeys.list(wsId), (old) =>
        old?.filter((pipeline) => pipeline.id !== id),
      );
      qc.removeQueries({ queryKey: pipelineKeys.detail(wsId, id) });
      return { prevList };
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prevList) qc.setQueryData(pipelineKeys.list(wsId), ctx.prevList);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: pipelineKeys.list(wsId) });
    },
  });
}

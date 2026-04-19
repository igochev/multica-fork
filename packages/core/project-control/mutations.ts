import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import type { ProjectControl, UpdateProjectControlRequest } from "../types";
import { projectControlKeys } from "./queries";

export function useUpdateProjectControl() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateProjectControlRequest }) =>
      api.updateProjectControl(id, data),
    onMutate: async ({ id, data }) => {
      await qc.cancelQueries({ queryKey: projectControlKeys.detail(wsId, id) });
      const previousControl = qc.getQueryData<ProjectControl>(projectControlKeys.detail(wsId, id));
      qc.setQueryData<ProjectControl>(projectControlKeys.detail(wsId, id), (current) =>
        current ? { ...current, ...data } : current,
      );
      return { id, previousControl };
    },
    onError: (_error, _variables, context) => {
      if (context?.previousControl) {
        qc.setQueryData(projectControlKeys.detail(wsId, context.id), context.previousControl);
      }
    },
    onSuccess: (control) => {
      qc.setQueryData(projectControlKeys.detail(wsId, control.project_id), control);
    },
    onSettled: (_data, _error, variables) => {
      qc.invalidateQueries({ queryKey: projectControlKeys.detail(wsId, variables.id) });
    },
  });
}

export function useReconcileProjectControl() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: (id: string) => api.reconcileProjectControl(id),
    onSettled: (_data, _error, id) => {
      qc.invalidateQueries({ queryKey: projectControlKeys.detail(wsId, id) });
    },
  });
}

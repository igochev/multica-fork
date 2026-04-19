import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const projectControlKeys = {
  all: (wsId: string) => ["project-control", wsId] as const,
  detail: (wsId: string, id: string) => [...projectControlKeys.all(wsId), "detail", id] as const,
};

export function projectControlOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: projectControlKeys.detail(wsId, id),
    queryFn: () => api.getProjectControl(id),
  });
}
